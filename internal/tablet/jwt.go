package tablet

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/brianmcgee/rmhub/internal/accounts"
	"github.com/brianmcgee/rmhub/internal/ids"
	"github.com/brianmcgee/rmhub/internal/state"
)

const (
	// KeyName is the state key of the JWT signing key.
	KeyName = "server/jwt-key"
	// UserTokenLifetime is the lifetime of a user token.
	UserTokenLifetime = 3 * time.Hour
	// Scopes is the scope claim of a user token. sync:tortoise selects
	// the sync protocol 1.5.
	Scopes = "intgr screenshare docedit sync:tortoise"
	keyID  = "1"
)

// LoadKey returns the JWT signing key. The first server writes a new key
// with put-if-absent, so all servers on one state store use one key.
func LoadKey(ctx context.Context, st state.Store) ([]byte, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return nil, err
	}
	enc := base64.StdEncoding.EncodeToString(buf)
	_, err := st.PutIfAbsent(ctx, KeyName, []byte(enc))
	if err == nil {
		return buf, nil
	}
	if !errors.Is(err, state.ErrConflict) {
		return nil, err
	}
	b, _, err := st.Get(ctx, KeyName)
	if err != nil {
		return nil, err
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(b)))
	if err != nil || len(key) < 32 {
		return nil, fmt.Errorf("%s: the stored key is not valid", KeyName)
	}
	return key, nil
}

// Issuer signs and checks the tablet tokens.
type Issuer struct {
	key []byte
	now func() time.Time
}

// NewIssuer returns an issuer with the key.
func NewIssuer(key []byte) *Issuer { return &Issuer{key: key, now: time.Now} }

// SetClock sets the clock. Tests use it.
func (i *Issuer) SetClock(now func() time.Time) { i.now = now }

func (i *Issuer) sign(c jwt.MapClaims) (string, error) {
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, c)
	t.Header["kid"] = keyID
	return t.SignedString(i.key)
}

// pairingClaim is the claim with the pairing ID of the device. The
// tablet does not read it.
const pairingClaim = "rmhub-pairing"

// DeviceToken returns a device token for one pairing of the device. It
// has no expiry time.
func (i *Issuer) DeviceToken(account, desc, deviceID, pairing string) (string, error) {
	return i.sign(jwt.MapClaims{
		"auth0-userid": "auth0|" + account,
		"device-desc":  desc,
		"device-id":    deviceID,
		pairingClaim:   pairing,
		"aud":          "api",
	})
}

// UserToken returns a user token for one pairing of the device of the
// account.
func (i *Issuer) UserToken(a accounts.Account, desc, deviceID, pairing string) (string, error) {
	now := i.now().UTC().Truncate(time.Second)
	name := a.Name
	if name == "" {
		name = a.Email
	}
	created := a.CreatedAt.UTC().Format(time.RFC3339)
	return i.sign(jwt.MapClaims{
		"auth0-profile": map[string]any{
			"UserID":        "auth0|" + a.ID,
			"Email":         a.Email,
			"Name":          name,
			"EmailVerified": true,
			"CreatedAt":     created,
			"UpdatedAt":     created,
		},
		"device-desc": desc,
		"device-id":   deviceID,
		pairingClaim:  pairing,
		"scopes":      Scopes,
		"level":       "connect",
		"tectonic":    "eu",
		"version":     10,
		"iss":         "rM WebApp",
		"sub":         "rM User Token",
		"jti":         ids.Random(16),
		"iat":         now.Unix(),
		"nbf":         now.Unix(),
		"exp":         now.Add(UserTokenLifetime).Unix(),
	})
}

// Device is the content of a valid device token. Pairing is empty in a
// token from an earlier version of rmhub.
type Device struct {
	Account  string
	DeviceID string
	Desc     string
	Pairing  string
}

// User is the content of a valid user token. Pairing is empty in a token
// from an earlier version of rmhub.
type User struct {
	Account  string
	DeviceID string
	Desc     string
	Pairing  string
}

var errToken = errors.New("tablet token is not valid")

func (i *Issuer) parse(text string, opts ...jwt.ParserOption) (jwt.MapClaims, error) {
	opts = append(opts, jwt.WithValidMethods([]string{"HS256"}), jwt.WithTimeFunc(i.now))
	claims := jwt.MapClaims{}
	_, err := jwt.ParseWithClaims(text, claims, func(*jwt.Token) (any, error) { return i.key, nil }, opts...)
	if err != nil {
		return nil, errToken
	}
	return claims, nil
}

func accountOf(userID any) (string, bool) {
	s, _ := userID.(string)
	acct, ok := strings.CutPrefix(s, "auth0|")
	return acct, ok && ids.IsAccountID(acct)
}

// ParseDevice checks a device token.
func (i *Issuer) ParseDevice(text string) (Device, error) {
	c, err := i.parse(text, jwt.WithAudience("api"))
	if err != nil {
		return Device{}, err
	}
	acct, ok := accountOf(c["auth0-userid"])
	id, _ := c["device-id"].(string)
	desc, _ := c["device-desc"].(string)
	pairing, _ := c[pairingClaim].(string)
	if !ok || id == "" {
		return Device{}, errToken
	}
	return Device{Account: acct, DeviceID: id, Desc: desc, Pairing: pairing}, nil
}

// ParseUser checks a user token: signature, expiry, version 10 and a
// UserID in auth0-profile.
func (i *Issuer) ParseUser(text string) (User, error) {
	c, err := i.parse(text, jwt.WithExpirationRequired())
	if err != nil {
		return User{}, err
	}
	if v, _ := c["version"].(float64); v != 10 {
		return User{}, errToken
	}
	profile, _ := c["auth0-profile"].(map[string]any)
	acct, ok := accountOf(profile["UserID"])
	id, _ := c["device-id"].(string)
	desc, _ := c["device-desc"].(string)
	pairing, _ := c[pairingClaim].(string)
	if !ok || id == "" {
		return User{}, errToken
	}
	return User{Account: acct, DeviceID: id, Desc: desc, Pairing: pairing}, nil
}
