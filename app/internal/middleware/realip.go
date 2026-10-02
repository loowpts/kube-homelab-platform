package middleware

import (
	"net"
	"net/http"
)

// RealIP sets RemoteAddr from X-Real-IP, which ingress-nginx always overwrites
// with the address of the connecting client. Unlike chi's RealIP it ignores
// True-Client-IP and X-Forwarded-For: ingress-nginx passes those through from
// the client, so trusting them would let anyone spoof their address.
func RealIP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ip := net.ParseIP(r.Header.Get("X-Real-IP")); ip != nil {
			r.RemoteAddr = net.JoinHostPort(ip.String(), "0")
		}
		next.ServeHTTP(w, r)
	})
}
