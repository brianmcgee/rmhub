// Package devices keeps the devices that paired with an account.
package devices

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/brianmcgee/rmhub/internal/ids"
	"github.com/brianmcgee/rmhub/internal/state"
)

// CacheTTL is the time for which the registry keeps the result of a
// device check.
const CacheTTL = 30 * time.Second

// MaxIDLen is the maximum length of a device ID.
const MaxIDLen = 128

// ErrNotFound means that the device is not in the registry.
var ErrNotFound = errors.New("device not found")

// ValidID tells if id is a valid device ID: 1 to 128 characters from
// A-Z, a-z, 0-9 and "-".
func ValidID(id string) bool {
	if id == "" || len(id) > MaxIDLen {
		return false
	}
	for i := range len(id) {
		c := id[i]
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '-' {
			return false
		}
	}
	return true
}

// Device is a paired device.
type Device struct {
	DeviceID string    `json:"deviceId"`
	Desc     string    `json:"desc"`
	PairedAt time.Time `json:"pairedAt"`
}

// record is the stored form of a device.
type record struct {
	Device
	// Pairing identifies one pairing of the device. A record from an
	// earlier version of rmhub has no pairing ID.
	Pairing string `json:"pairing,omitempty"`
}

type cacheEntry struct {
	exists  bool
	pairing string
	expires time.Time
}

// Registry keeps the devices in a state store. Its device check has a
// cache in memory.
type Registry struct {
	st  state.Store
	now func() time.Time

	mu    sync.Mutex
	cache map[string]cacheEntry
}

// New returns a registry.
func New(st state.Store) *Registry {
	return &Registry{st: st, now: time.Now, cache: map[string]cacheEntry{}}
}

// SetClock sets the clock. Tests use it.
func (r *Registry) SetClock(now func() time.Time) { r.now = now }

// Key returns the state key of a device. A device ID can contain
// uppercase letters, which a state key cannot. So the key holds a hash.
func Key(account, deviceID string) string {
	h := sha256.Sum256([]byte(deviceID))
	return "accounts/" + account + "/devices/" + hex.EncodeToString(h[:])
}

// Register adds the device, or replaces the record of a device with the
// same ID. It returns the ID of the new pairing. The tokens of an
// earlier pairing of the device do not pass Exists and Check.
func (r *Registry) Register(ctx context.Context, account string, d Device) (string, error) {
	if !ValidID(d.DeviceID) {
		return "", fmt.Errorf("invalid device ID %q", d.DeviceID)
	}
	pairing := ids.Random(16)
	b, err := json.Marshal(record{Device: d, Pairing: pairing})
	if err != nil {
		return "", err
	}
	key := Key(account, d.DeviceID)
	for range 5 {
		_, ver, err := r.st.Get(ctx, key)
		switch {
		case errors.Is(err, state.ErrNotFound):
			_, err = r.st.PutIfAbsent(ctx, key, b)
		case err == nil:
			_, err = r.st.PutIfMatch(ctx, key, b, ver)
		}
		if errors.Is(err, state.ErrConflict) {
			continue
		}
		if err != nil {
			return "", err
		}
		r.forget(account, d.DeviceID)
		return pairing, nil
	}
	return "", errors.New("device record changed five times during the write")
}

// List returns the devices of the account, ordered by device ID.
func (r *Registry) List(ctx context.Context, account string) ([]Device, error) {
	keys, err := r.st.List(ctx, "accounts/"+account+"/devices/", "", 0)
	if err != nil {
		return nil, err
	}
	out := []Device{}
	for _, k := range keys {
		b, _, err := r.st.Get(ctx, k)
		if errors.Is(err, state.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		var rec record
		if err := json.Unmarshal(b, &rec); err != nil {
			return nil, fmt.Errorf("device record %s: %w", k, err)
		}
		out = append(out, rec.Device)
	}
	slices.SortFunc(out, func(a, b Device) int { return strings.Compare(a.DeviceID, b.DeviceID) })
	return out, nil
}

// Remove deletes the device. Other processes see the removal within
// CacheTTL.
func (r *Registry) Remove(ctx context.Context, account, deviceID string) error {
	if !ValidID(deviceID) {
		return ErrNotFound
	}
	key := Key(account, deviceID)
	if _, _, err := r.st.Get(ctx, key); errors.Is(err, state.ErrNotFound) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	if err := r.st.Delete(ctx, key); err != nil {
		return err
	}
	r.forget(account, deviceID)
	return nil
}

// lookup reads the record of the device without the cache.
func (r *Registry) lookup(ctx context.Context, account, deviceID string) (cacheEntry, error) {
	if !ValidID(deviceID) {
		return cacheEntry{}, nil
	}
	b, _, err := r.st.Get(ctx, Key(account, deviceID))
	if errors.Is(err, state.ErrNotFound) {
		return cacheEntry{}, nil
	}
	if err != nil {
		return cacheEntry{}, err
	}
	var rec record
	if err := json.Unmarshal(b, &rec); err != nil {
		return cacheEntry{}, fmt.Errorf("device record of %s: %w", deviceID, err)
	}
	return cacheEntry{exists: true, pairing: rec.Pairing}, nil
}

// match tells if the entry is a paired device with the pairing ID.
func (e cacheEntry) match(pairing string) bool {
	return e.exists && subtle.ConstantTimeCompare([]byte(e.pairing), []byte(pairing)) == 1
}

// Exists tells if the device is in the registry with the pairing ID. It
// reads the registry without the cache.
func (r *Registry) Exists(ctx context.Context, account, deviceID, pairing string) (bool, error) {
	e, err := r.lookup(ctx, account, deviceID)
	return e.match(pairing), err
}

// Check tells if the device is in the registry with the pairing ID. It
// keeps each record for CacheTTL.
func (r *Registry) Check(ctx context.Context, account, deviceID, pairing string) (bool, error) {
	ck := account + "/" + deviceID
	now := r.now()
	r.mu.Lock()
	e, ok := r.cache[ck]
	r.mu.Unlock()
	if ok && now.Before(e.expires) {
		return e.match(pairing), nil
	}
	e, err := r.lookup(ctx, account, deviceID)
	if err != nil {
		return false, err
	}
	e.expires = now.Add(CacheTTL)
	r.mu.Lock()
	defer r.mu.Unlock()
	// Remove expired entries so that the map does not grow without limit.
	for k, v := range r.cache {
		if !now.Before(v.expires) {
			delete(r.cache, k)
		}
	}
	r.cache[ck] = e
	return e.match(pairing), nil
}

func (r *Registry) forget(account, deviceID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.cache, account+"/"+deviceID)
}
