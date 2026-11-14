package rootlog

import (
	"sync"
	"sync/atomic"
)

// BufferSize is the number of events that a listener can hold.
const BufferSize = 64

// Event tells that a commit succeeded.
type Event struct {
	Account    string
	Generation uint64
	Hash       string
	Source     Source
	Broadcast  bool
}

// Listener receives the commit events of one account, or of all
// accounts.
type Listener struct {
	C       <-chan Event
	c       chan Event
	account string
	lost    atomic.Bool
	broker  *Broker
	once    sync.Once
}

// Lost tells if the broker dropped an event because the buffer was full,
// and clears the flag. A listener that sees true reads the current root.
func (l *Listener) Lost() bool { return l.lost.Swap(false) }

// Close stops the events. It does not close C.
func (l *Listener) Close() {
	l.once.Do(func() { l.broker.unsubscribe(l) })
}

// Broker sends commit events to local listeners. It never blocks.
type Broker struct {
	mu        sync.RWMutex
	listeners map[string]map[*Listener]struct{}
}

// NewBroker returns an empty broker.
func NewBroker() *Broker {
	return &Broker{listeners: map[string]map[*Listener]struct{}{}}
}

// Subscribe adds a listener for the account. The account "" gives the
// events of all accounts.
func (b *Broker) Subscribe(account string) *Listener {
	c := make(chan Event, BufferSize)
	l := &Listener{C: c, c: c, account: account, broker: b}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.listeners[account] == nil {
		b.listeners[account] = map[*Listener]struct{}{}
	}
	b.listeners[account][l] = struct{}{}
	return l
}

func (b *Broker) unsubscribe(l *Listener) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.listeners[l.account], l)
	if len(b.listeners[l.account]) == 0 {
		delete(b.listeners, l.account)
	}
}

// Publish sends the event to each listener of its account and to each
// listener of all accounts. A full buffer drops the event and sets the
// lost-events flag of that listener.
func (b *Broker) Publish(e Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, set := range []map[*Listener]struct{}{b.listeners[e.Account], b.listeners[""]} {
		for l := range set {
			select {
			case l.c <- e:
			default:
				l.lost.Store(true)
			}
		}
	}
}
