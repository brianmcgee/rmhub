package ratelimit

import (
	"net/http"
	"net/netip"
	"testing"
)

func TestClientKey(t *testing.T) {
	local := []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32"), netip.MustParsePrefix("10.0.0.0/8")}
	for _, tc := range []struct {
		name    string
		peer    string
		xff     []string
		trusted []netip.Prefix
		want    string
	}{
		{"no proxy", "192.0.2.1:4000", nil, nil, "192.0.2.1"},
		{"header from an untrusted peer", "192.0.2.1:4000", []string{"203.0.113.9"}, nil, "192.0.2.1"},
		{"header from a peer that is not in the list", "192.0.2.1:4000", []string{"203.0.113.9"}, local, "192.0.2.1"},
		{"trusted proxy", "127.0.0.1:4000", []string{"198.51.100.7, 203.0.113.9"}, local, "203.0.113.9"},
		{"two trusted proxies", "127.0.0.1:4000", []string{"198.51.100.7, 203.0.113.9, 10.2.3.4"}, local, "203.0.113.9"},
		{"two header lines", "127.0.0.1:4000", []string{"198.51.100.7", "203.0.113.9"}, local, "203.0.113.9"},
		{"trusted proxy without a header", "127.0.0.1:4000", nil, local, "127.0.0.1"},
		{"only trusted addresses", "127.0.0.1:4000", []string{"10.2.3.4"}, local, "127.0.0.1"},
		{"bad header", "127.0.0.1:4000", []string{"198.51.100.7, unknown"}, local, "127.0.0.1"},
		{"IPv4 in IPv6", "[::ffff:192.0.2.1]:4000", nil, nil, "192.0.2.1"},
		{"IPv6 network", "[2001:db8:1:2:3:4:5:6]:4000", nil, nil, "2001:db8:1:2::/64"},
		{"IPv6 same network", "[2001:db8:1:2::1]:4000", nil, nil, "2001:db8:1:2::/64"},
		{"IPv6 from the header", "127.0.0.1:4000", []string{"2001:db8:9:8:7:6:5:4"}, local, "2001:db8:9:8::/64"},
		{"no address", "@", nil, nil, "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, err := http.NewRequest(http.MethodPost, "http://rmhub.example/token/json/2/device/new", nil)
			if err != nil {
				t.Fatal(err)
			}
			r.RemoteAddr = tc.peer
			for _, v := range tc.xff {
				r.Header.Add("X-Forwarded-For", v)
			}
			if got := ClientKey(r, tc.trusted); got != tc.want {
				t.Errorf("ClientKey = %q, want %q", got, tc.want)
			}
		})
	}
}
