package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDeprecatedHeaders(t *testing.T) {
	next := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}

	tests := []struct {
		name       string
		successor  string
		wantLink   string
		wantNoLink bool
	}{
		{name: "with successor", successor: "/api/v1/apm/poli", wantLink: `</api/v1/apm/poli>; rel="successor-version"`},
		{name: "without successor", wantNoLink: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := &Server{}
			req := httptest.NewRequest(http.MethodGet, "/api/v1/poli", nil)
			rec := httptest.NewRecorder()

			server.deprecated(tt.successor, next).ServeHTTP(rec, req)

			if rec.Code != http.StatusNoContent {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
			}
			if got := rec.Header().Get("Deprecation"); got != "true" {
				t.Fatalf("Deprecation = %q, want true", got)
			}
			if tt.wantNoLink {
				if got := rec.Header().Get("Link"); got != "" {
					t.Fatalf("Link = %q, want empty", got)
				}
				return
			}
			if got := rec.Header().Get("Link"); got != tt.wantLink {
				t.Fatalf("Link = %q, want %q", got, tt.wantLink)
			}
		})
	}
}

func TestAPMQueueRejectsFreeTextLokasi(t *testing.T) {
	server := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apm/queue", strings.NewReader(`{"mode":"blank","poli_id":"01","lokasi":"X"}`))
	rec := httptest.NewRecorder()

	server.apmQueue(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rec.Body.String(), "INVALID_INPUT") {
		t.Fatalf("body = %q, want INVALID_INPUT", rec.Body.String())
	}
}

func TestWithAPIKey(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	tests := []struct {
		name       string
		apiKey     string
		header     string
		wantStatus int
	}{
		{name: "missing key", apiKey: "secret", wantStatus: http.StatusUnauthorized},
		{name: "wrong key", apiKey: "secret", header: "wrong", wantStatus: http.StatusUnauthorized},
		{name: "valid key", apiKey: "secret", header: "secret", wantStatus: http.StatusNoContent},
		{name: "not configured", apiKey: "", header: "secret", wantStatus: http.StatusServiceUnavailable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := &Server{apiKey: tt.apiKey}
			req := httptest.NewRequest(http.MethodPost, "/api/v1/apm/lookup", nil)
			if tt.header != "" {
				req.Header.Set("X-API-Key", tt.header)
			}
			rec := httptest.NewRecorder()

			server.withAPIKey(next).ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}

func TestWithCORS(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	tests := []struct {
		name   string
		origin string
		want   string
	}{
		{name: "disabled by default", want: ""},
		{name: "configured origin", origin: "http://localhost:3000", want: "http://localhost:3000"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := &Server{corsOrigin: tt.origin}
			req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
			rec := httptest.NewRecorder()

			server.withCORS(next).ServeHTTP(rec, req)

			if got := rec.Header().Get("Access-Control-Allow-Origin"); got != tt.want {
				t.Fatalf("Access-Control-Allow-Origin = %q, want %q", got, tt.want)
			}
		})
	}
}
