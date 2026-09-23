package ipinfo

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
)

func TestOrg(t *testing.T) {
	tests := []struct {
		name    string
		addr    string
		status  int
		body    string
		want    string
		wantErr bool
	}{
		{
			name:   "IPv4",
			addr:   "8.8.8.8",
			status: http.StatusOK,
			body:   `{"ip": "8.8.8.8", "org": "AS15169 Google LLC"}`,
			want:   "AS15169 Google LLC",
		},
		{
			name:   "IPv6",
			addr:   "2001:4860:4860::8888",
			status: http.StatusOK,
			body:   `{"ip": "2001:4860:4860::8888", "org": "AS15169 Google LLC"}`,
			want:   "AS15169 Google LLC",
		},
		{
			name:   "no organization on record",
			addr:   "192.0.2.1",
			status: http.StatusOK,
			body:   `{"ip": "192.0.2.1", "bogon": true}`,
			want:   "",
		},
		{
			name:    "rate limited",
			addr:    "8.8.8.8",
			status:  http.StatusTooManyRequests,
			body:    `{"status": 429, "error": {"title": "Rate limit exceeded"}}`,
			wantErr: true,
		},
		{
			name:    "malformed response",
			addr:    "8.8.8.8",
			status:  http.StatusOK,
			body:    `{"org": `,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if want := "/" + tt.addr + "/json"; r.URL.Path != want {
					t.Errorf("request path = %q, want %q", r.URL.Path, want)
				}
				w.WriteHeader(tt.status)
				fmt.Fprint(w, tt.body)
			}))
			defer srv.Close()

			c := &Client{HTTPClient: srv.Client(), BaseURL: srv.URL}
			got, err := c.Org(t.Context(), netip.MustParseAddr(tt.addr))
			if (err != nil) != tt.wantErr {
				t.Fatalf("Org() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("Org() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestOrgSkipsNonPublicAddresses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request for %s", r.URL.Path)
	}))
	defer srv.Close()

	c := &Client{HTTPClient: srv.Client(), BaseURL: srv.URL}
	for _, addr := range []string{
		"0.0.0.0",
		"10.1.2.3",
		"127.0.0.1",
		"169.254.1.1",
		"172.16.0.1",
		"192.168.1.1",
		"224.0.0.1",
		"::1",
		"fd00::1",
		"fe80::1%eth0",
	} {
		org, err := c.Org(t.Context(), netip.MustParseAddr(addr))
		if org != "" || err != nil {
			t.Errorf("Org(%s) = %q, %v, want \"\", nil", addr, org, err)
		}
	}
}
