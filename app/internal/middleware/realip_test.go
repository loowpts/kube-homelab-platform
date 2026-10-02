package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRealIP(t *testing.T) {
	tests := []struct {
		name    string
		headers map[string]string
		want    string
	}{
		{
			name:    "X-Real-IP from ingress replaces RemoteAddr",
			headers: map[string]string{"X-Real-IP": "203.0.113.7"},
			want:    "203.0.113.7",
		},
		{
			name:    "client-controlled headers are ignored",
			headers: map[string]string{"True-Client-IP": "1.1.1.1", "X-Forwarded-For": "2.2.2.2"},
			want:    "10.0.0.1",
		},
		{
			name:    "invalid X-Real-IP is ignored",
			headers: map[string]string{"X-Real-IP": "not-an-ip"},
			want:    "10.0.0.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = "10.0.0.1:12345"
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			var got string
			RealIP(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got = clientIP(r)
			})).ServeHTTP(httptest.NewRecorder(), req)

			if got != tt.want {
				t.Errorf("client IP = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestClientIPIgnoresXForwardedFor(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "1.2.3.4")

	if got := clientIP(req); got != "10.0.0.1" {
		t.Errorf("clientIP = %q, want %q", got, "10.0.0.1")
	}
}
