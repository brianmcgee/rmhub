// Package devices keeps the devices that paired with an account.
package devices

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

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

type cacheEntry struct {
	exists  bool
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
// same ID.
func (r *Registry) Register(ctx context.Context, account string, d Device) error {
	if !ValidID(d.DeviceID) {
		return fmt.Errorf("invalid device ID %q", d.DeviceID)
	}
	b, err := json.Marshal(d)
	if err != nil {
		return err
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
		if err == nil {
			r.forget(account, d.DeviceID)
		}
		return err
	}
	return errors.New("device record changed five times during the write")
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
		var d Device
		if err := json.Unmarshal(b, &d); err != nil {
			return nil, fmt.Errorf("device record %s: %w", k, err)
		}
		out = append(out, d)
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

// Exists reads the registry without the cache.
func (r *Registry) Exists(ctx context.Context, account, deviceID string) (bool, error) {
	if !ValidID(deviceID) {
		return false, nil
	}
	_, _, err := r.st.Get(ctx, Key(account, deviceID))
	if errors.Is(err, state.ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}

// Check tells if the device is in the registry. It keeps each result for
// CacheTTL.
func (r *Registry) Check(ctx context.Context, account, deviceID string) (bool, error) {
	ck := account + "/" + deviceID
	now := r.now()
	r.mu.Lock()
	e, ok := r.cache[ck]
	r.mu.Unlock()
	if ok && now.Before(e.expires) {
		return e.exists, nil
	}
	exists, err := r.Exists(ctx, account, deviceID)
	if err != nil {
		return false, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	// Remove expired entries so that the map does not grow without limit.
	for k, v := range r.cache {
		if !now.Before(v.expires) {
			delete(r.cache, k)
		}
	}
	r.cache[ck] = cacheEntry{exists: exists, expires: now.Add(CacheTTL)}
	return exists, nil
}

func (r *Registry) forget(account, deviceID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.cache, account+"/"+deviceID)
}
