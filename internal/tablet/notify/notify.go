// Package notify sends SyncComplete messages to the notification
// websockets of the tablets.
package notify

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/brianmcgee/rmhub/internal/ids"
	"github.com/brianmcgee/rmhub/internal/rootlog"
	"github.com/brianmcgee/rmhub/internal/tablet"
)

// SendBuffer is the number of unsent messages after which the hub
// closes a websocket.
const SendBuffer = 64

// DeviceCheck tells if the device is still paired with the account.
type DeviceCheck func(ctx context.Context, account, deviceID string) (bool, error)

// Hub keeps the open websockets and sends the notifications.
type Hub struct {
	log   *slog.Logger
	check DeviceCheck

	// PingInterval is the time between two pings. At each ping, the hub
	// also checks that the device is still paired.
	PingInterval time.Duration
	// PongWait is the time after the last pong after which the hub
	// closes a websocket.
	PongWait time.Duration

	upgrader websocket.Upgrader

	mu     sync.Mutex
	conns  map[string]map[*conn]struct{} // by account
	closed bool
}

// New returns a hub. check finds removed devices.
func New(log *slog.Logger, check DeviceCheck) *Hub {
	return &Hub{
		log:          log,
		check:        check,
		PingInterval: 30 * time.Second,
		PongWait:     90 * time.Second,
		// The tablet sends no browser origin.
		upgrader: websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }},
		conns:    map[string]map[*conn]struct{}{},
	}
}

type conn struct {
	ws       *websocket.Conn
	account  string
	deviceID string
	send     chan []byte
	once     sync.Once
	done     chan struct{}
}

func (c *conn) close() {
	c.once.Do(func() {
		close(c.done)
		_ = c.ws.Close()
	})
}

// Message is the JSON form of a notification.
type Message struct {
	Message MessageBody `json:"message"`
}

// MessageBody is the body of a notification.
type MessageBody struct {
	Attributes Attributes `json:"attributes"`
	MessageID  string     `json:"messageid"`
}

// Attributes are the attributes of a notification.
type Attributes struct {
	Auth0UserID      string `json:"auth0UserID"`
	Event            string `json:"event"`
	SourceDeviceDesc string `json:"sourceDeviceDesc"`
	SourceDeviceID   string `json:"sourceDeviceID"`
}

// Run sends a notification for each root log event with broadcast,
// until ctx ends. Then it closes all websockets.
func (h *Hub) Run(ctx context.Context, roots *rootlog.Log) {
	l := roots.Subscribe("")
	defer l.Close()
	defer h.closeAll()
	for {
		select {
		case <-ctx.Done():
			return
		case e := <-l.C:
			if l.Lost() {
				h.log.Warn("notification hub lost root log events; some devices miss a notification")
			}
			if e.Broadcast {
				h.Notify(e.Account, e.Source)
			}
		}
	}
}

func (h *Hub) closeAll() {
	h.mu.Lock()
	h.closed = true
	var all []*conn
	for _, cs := range h.conns {
		for c := range cs {
			all = append(all, c)
		}
	}
	h.mu.Unlock()
	for _, c := range all {
		c.close()
	}
}

// Notify sends a SyncComplete message to each websocket of the account,
// except the websockets of the source device. It returns the message ID.
func (h *Hub) Notify(account string, src rootlog.Source) string {
	id := ids.RandomHex(16)
	b, err := json.Marshal(Message{Message: MessageBody{
		Attributes: Attributes{
			Auth0UserID:      account,
			Event:            "SyncComplete",
			SourceDeviceDesc: src.Desc,
			SourceDeviceID:   src.ID,
		},
		MessageID: id,
	}})
	if err != nil {
		panic(err)
	}
	h.mu.Lock()
	var full []*conn
	for c := range h.conns[account] {
		if src.Type == rootlog.SourceDevice && c.deviceID == src.ID {
			continue
		}
		select {
		case c.send <- b:
		default:
			full = append(full, c)
		}
	}
	h.mu.Unlock()
	for _, c := range full {
		h.log.Warn("notification websocket has too many unsent messages; closing it", "account", c.account, "device", c.deviceID)
		c.close()
	}
	return id
}

// ServeHTTP upgrades a request with a valid user token to a websocket.
// Use it behind tablet.Service.RequireUser.
func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	u := tablet.UserFrom(r.Context())
	ws, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		// The upgrader wrote the error response.
		return
	}
	c := &conn{ws: ws, account: u.Account, deviceID: u.DeviceID, send: make(chan []byte, SendBuffer), done: make(chan struct{})}
	if !h.add(c) {
		c.close()
		return
	}
	h.log.Info("notification websocket open", "account", u.Account, "device", u.DeviceID)
	go h.read(c)
	h.write(c)
	h.remove(c)
	h.log.Info("notification websocket closed", "account", u.Account, "device", u.DeviceID)
}

func (h *Hub) add(c *conn) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return false
	}
	if h.conns[c.account] == nil {
		h.conns[c.account] = map[*conn]struct{}{}
	}
	h.conns[c.account][c] = struct{}{}
	return true
}

func (h *Hub) remove(c *conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.conns[c.account], c)
	if len(h.conns[c.account]) == 0 {
		delete(h.conns, c.account)
	}
}

// read discards the messages of the tablet and extends the read
// deadline at each pong.
func (h *Hub) read(c *conn) {
	defer c.close()
	c.ws.SetReadLimit(64 << 10)
	_ = c.ws.SetReadDeadline(time.Now().Add(h.PongWait))
	c.ws.SetPongHandler(func(string) error {
		return c.ws.SetReadDeadline(time.Now().Add(h.PongWait))
	})
	for {
		if _, _, err := c.ws.ReadMessage(); err != nil {
			return
		}
	}
}

const writeWait = 10 * time.Second

func (h *Hub) write(c *conn) {
	defer c.close()
	ticker := time.NewTicker(h.PingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-c.done:
			return
		case b := <-c.send:
			_ = c.ws.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.ws.WriteMessage(websocket.TextMessage, b); err != nil {
				return
			}
		case <-ticker.C:
			ok, err := h.check(context.Background(), c.account, c.deviceID)
			if err == nil && !ok {
				h.log.Info("device removed; closing its notification websocket", "account", c.account, "device", c.deviceID)
				_ = c.ws.WriteControl(websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "device removed"), time.Now().Add(writeWait))
				return
			}
			if err := c.ws.WriteControl(websocket.PingMessage, nil, time.Now().Add(writeWait)); err != nil {
				return
			}
		}
	}
}
