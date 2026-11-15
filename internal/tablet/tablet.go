// Package tablet serves the reMarkable protocol on the tablet listener.
package tablet

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/brianmcgee/rmhub/internal/accounts"
	"github.com/brianmcgee/rmhub/internal/devices"
	"github.com/brianmcgee/rmhub/internal/pairing"
	"github.com/brianmcgee/rmhub/internal/ratelimit"
)

// maxBody is the maximum size of a JSON request body.
const maxBody = 1 << 20

// Service holds the dependencies of the tablet endpoints.
type Service struct {
	Accounts *accounts.Store
	Devices  *devices.Registry
	Pairing  *pairing.Store
	Issuer   *Issuer
	Log      *slog.Logger
	// PairLimit limits the pairing attempts of each client address. Nil
	// means no limit.
	PairLimit *ratelimit.Limiter
	// Proxies are the trusted proxies of the tablet listener.
	Proxies []netip.Prefix
	// Now is the clock. Tests set it.
	Now func() time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Register adds the token endpoints to the mux.
func (s *Service) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /token/json/2/device/new", s.deviceNew)
	mux.HandleFunc("POST /token/json/2/user/new", s.userNew)
	mux.HandleFunc("POST /token/json/2/device/delete", s.deviceDelete)
	mux.HandleFunc("POST /token/json/3/device/delete", s.deviceDelete)
}

func (s *Service) fail(w http.ResponseWriter, r *http.Request, err error) {
	s.Log.Error("tablet request failed", "method", r.Method, "path", r.URL.Path, "err", err)
	http.Error(w, "Internal Server Error", http.StatusInternalServerError)
}

func writeText(w http.ResponseWriter, text string) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, text)
}

func unauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = io.WriteString(w, "Unauthorized")
}

func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if len(h) < 7 || !strings.EqualFold(h[:7], "bearer ") {
		return ""
	}
	return strings.TrimSpace(h[7:])
}

// maxDeviceDesc is the maximum length in bytes of a device description.
// The description goes into the device record and into each token of
// the device.
const maxDeviceDesc = 256

type deviceNewRequest struct {
	Code       string `json:"code"`
	DeviceDesc string `json:"deviceDesc"`
	DeviceID   string `json:"deviceID"`
}

func (s *Service) deviceNew(w http.ResponseWriter, r *http.Request) {
	// Each attempt counts, with a valid code or not: the limit makes a
	// search for a code too slow to succeed.
	if ok, wait := s.PairLimit.Allow(ratelimit.ClientKey(r, s.Proxies)); !ok {
		w.Header().Set("Retry-After", ratelimit.RetryAfter(wait))
		w.WriteHeader(http.StatusTooManyRequests)
		return
	}
	var req deviceNewRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, maxBody)).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	// Check the device ID before the code use, so that a bad request does
	// not consume the code.
	if !devices.ValidID(req.DeviceID) || len(req.DeviceDesc) > maxDeviceDesc {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	acct, err := s.Pairing.Use(r.Context(), req.Code, req.DeviceID)
	if errors.Is(err, pairing.ErrInvalidCode) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	d := devices.Device{DeviceID: req.DeviceID, Desc: req.DeviceDesc, PairedAt: s.now().UTC().Truncate(time.Second)}
	pairingID, err := s.Devices.Register(r.Context(), acct, d)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	tok, err := s.Issuer.DeviceToken(acct, req.DeviceDesc, req.DeviceID, pairingID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.Log.Info("device paired", "account", acct, "device", req.DeviceID, "desc", req.DeviceDesc)
	writeText(w, tok)
}

// device checks the device token of the request and that the device is
// in the registry.
func (s *Service) device(w http.ResponseWriter, r *http.Request) (Device, bool) {
	d, err := s.Issuer.ParseDevice(bearer(r))
	if err != nil {
		unauthorized(w)
		return Device{}, false
	}
	ok, err := s.Devices.Exists(r.Context(), d.Account, d.DeviceID, d.Pairing)
	if err != nil {
		s.fail(w, r, err)
		return Device{}, false
	}
	if !ok {
		unauthorized(w)
		return Device{}, false
	}
	return d, true
}

func (s *Service) userNew(w http.ResponseWriter, r *http.Request) {
	d, ok := s.device(w, r)
	if !ok {
		return
	}
	a, err := s.Accounts.Get(r.Context(), d.Account)
	if errors.Is(err, accounts.ErrNotFound) {
		unauthorized(w)
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	tok, err := s.Issuer.UserToken(a, d.Desc, d.DeviceID, d.Pairing)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeText(w, tok)
}

func (s *Service) deviceDelete(w http.ResponseWriter, r *http.Request) {
	d, ok := s.device(w, r)
	if !ok {
		return
	}
	err := s.Devices.Remove(r.Context(), d.Account, d.DeviceID)
	if err != nil && !errors.Is(err, devices.ErrNotFound) {
		s.fail(w, r, err)
		return
	}
	s.Log.Info("device removed by the device", "account", d.Account, "device", d.DeviceID)
	w.WriteHeader(http.StatusNoContent)
}

type userKey struct{}

// UserFrom returns the user of a request that passed RequireUser.
func UserFrom(ctx context.Context) User {
	u, _ := ctx.Value(userKey{}).(User)
	return u
}

// RequireUser checks the user token of each request and that its device
// is in the registry. A failed check gets status 401 with the body
// "Unauthorized".
func (s *Service) RequireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, err := s.Issuer.ParseUser(bearer(r))
		if err != nil {
			unauthorized(w)
			return
		}
		ok, err := s.Devices.Check(r.Context(), u.Account, u.DeviceID, u.Pairing)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if !ok {
			unauthorized(w)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey{}, u)))
	})
}
