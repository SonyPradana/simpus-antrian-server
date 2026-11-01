package httpapi

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLegacyEndpointsRemoved(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := New(nil, logger, "secret", "").Handler()

	requests := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/poli"},
		{http.MethodGet, "/api/v1/dokter"},
		{http.MethodGet, "/api/v1/registration/options"},
		{http.MethodGet, "/api/v1/patients/lookup"},
		{http.MethodPost, "/api/v1/registration"},
		{http.MethodPost, "/api/v1/regestration"},
		{http.MethodPost, "/api/v1/queue/empty"},
		{http.MethodPost, "/api/v1/queue/new"},
	}

	for _, request := range requests {
		t.Run(request.method+" "+request.path, func(t *testing.T) {
			req := httptest.NewRequest(request.method, request.path, nil)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
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
