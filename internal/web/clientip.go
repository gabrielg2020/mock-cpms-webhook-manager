package web

import (
	"context"
	"net/http"
	"net/netip"
	"strings"
)

const clientIPKey contextKey = "clientIP"

// clientIPFrom returns the address the request came from. It is the zero Addr if that is not known.
func clientIPFrom(ctx context.Context) netip.Addr {
	address, _ := ctx.Value(clientIPKey).(netip.Addr)
	return address
}

func rememberClientIP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), clientIPKey, clientIP(r))))
	})
}

// clientIP works out who sent the request. Behind the proxy every connection comes from the
// proxy, which puts the real address in the X-Forwarded-For. Anyone can set that header, so
// it is believed only when the connection itself comes from a private address, which on the
// sever means the proxy. The last entry is used because it is the one the nearest proxy added.
func clientIP(r *http.Request) netip.Addr {
	peer, err := netip.ParseAddrPort(r.RemoteAddr)
	if err != nil {
		return netip.Addr{}
	}
	address := peer.Addr().Unmap()
	if !address.IsLoopback() && !address.IsPrivate() {
		return address
	}

	headers := r.Header.Values("X-Forwarded-For")
	if len(headers) == 0 {
		return address
	}
	last := headers[len(headers)-1]
	if comma := strings.LastIndex(last, ","); comma >= 0 {
		last = last[comma+1:]
	}
	forwarded, err := netip.ParseAddr(strings.TrimSpace(last))
	if err != nil {
		return address
	}
	return forwarded.Unmap()
}

// limiterKey is what a client's attempts are counted under. An IPv6 customer usally has a
// whole /64 network to choose addresses from, so those are counted together.
func limiterKey(address netip.Addr) string {
	if address.Is6() {
		if network, err := address.Prefix(64); err == nil {
			return network.String()
		}
	}
	return address.String()
}
