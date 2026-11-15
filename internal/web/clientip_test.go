package web

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
)

func TestClientIP(t *testing.T) {
	testCases := []struct {
		name      string
		peer      string
		forwarded []string
		want      string
	}{
		{name: "direct from the internet", peer: "203.0.113.5:40000", want: "203.0.113.5"},
		{name: "header forged by a client is ignored", peer: "203.0.113.5:40000", forwarded: []string{"198.51.100.9"}, want: "203.0.113.5"},
		{name: "through the proxy", peer: "172.18.0.2:51000", forwarded: []string{"203.0.113.5"}, want: "203.0.113.5"},
		{name: "through the proxy on this machine", peer: "127.0.0.1:51000", forwarded: []string{"203.0.113.5"}, want: "203.0.113.5"},
		{name: "through the proxy over IPv6", peer: "[::1]:51000", forwarded: []string{"2001:db8::7"}, want: "2001:db8::7"},
		{name: "last entry of a list", peer: "172.18.0.2:51000", forwarded: []string{"198.51.100.9, 203.0.113.5"}, want: "203.0.113.5"},
		{name: "last of several headers", peer: "172.18.0.2:51000", forwarded: []string{"198.51.100.9", "192.0.2.44, 203.0.113.5"}, want: "203.0.113.5"},
		{name: "spaces around the entry", peer: "172.18.0.2:51000", forwarded: []string{"198.51.100.9 ,  203.0.113.5 "}, want: "203.0.113.5"},
		{name: "proxy sent no header", peer: "172.18.0.1:51000", want: "172.18.0.1"},
		{name: "header is not an address", peer: "172.18.0.2:51000", forwarded: []string{"<script>"}, want: "172.18.0.2"},
		{name: "header is empty", peer: "172.18.0.2:51000", forwarded: []string{""}, want: "172.18.0.2"},
		{name: "IPv4 written as IPv6", peer: "[::ffff:203.0.113.5]:40000", want: "203.0.113.5"},
		{name: "peer cannot be read", peer: "not an address", forwarded: []string{"203.0.113.5"}, want: "invalid IP"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
			request.RemoteAddr = testCase.peer
			for _, value := range testCase.forwarded {
				request.Header.Add("X-Forwarded-For", value)
			}

			if got := clientIP(request).String(); got != testCase.want {
				t.Errorf("client IP = %s, want %s", got, testCase.want)
			}
		})
	}
}

func TestLimiterKey(t *testing.T) {
	testCases := []struct {
		address string
		want    string
	}{
		{address: "203.0.113.5", want: "203.0.113.5"},
		{address: "2001:db8:1:2:3:4:5:6", want: "2001:db8:1:2::/64"},
		{address: "2001:db8:1:2::1", want: "2001:db8:1:2::/64"},
		{address: "2001:db8:1:3::1", want: "2001:db8:1:3::/64"},
	}

	for _, testCase := range testCases {
		if got := limiterKey(netip.MustParseAddr(testCase.address)); got != testCase.want {
			t.Errorf("key for %s = %s, want %s", testCase.address, got, testCase.want)
		}
	}
	if got := limiterKey(netip.Addr{}); got != "invalid IP" {
		t.Errorf("key for an unknown address = %s, want one shared key", got)
	}
}
