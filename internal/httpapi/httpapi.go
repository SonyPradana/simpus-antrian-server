package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/SonyPradana/simpus-antrian-server/internal/service"
)

type Server struct {
	service    *service.Service
	log        *slog.Logger
	apiKey     string
	corsOrigin string
}

func New(svc *service.Service, logger *slog.Logger, apiKey, corsOrigin string) *Server {
	return &Server{service: svc, log: logger, apiKey: apiKey, corsOrigin: corsOrigin}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.Handle("POST /api/v1/apm/lookup", s.withAPIKey(http.HandlerFunc(s.apmLookup)))
	mux.Handle("GET /api/v1/apm/options", s.withAPIKey(http.HandlerFunc(s.apmOptions)))
	mux.Handle("GET /api/v1/apm/poli", s.withAPIKey(http.HandlerFunc(s.apmListPoli)))
	mux.Handle("GET /api/v1/apm/dokter", s.withAPIKey(http.HandlerFunc(s.apmListDoctors)))
	mux.Handle("POST /api/v1/apm/queue", s.withAPIKey(http.HandlerFunc(s.apmQueue)))
	return s.withLogging(s.withCORS(mux))
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (s *Server) writeServiceError(w http.ResponseWriter, err error, data any) {
	status, code, message := mapError(err)
	payload := map[string]any{
		"error": map[string]any{
			"code":    code,
			"message": message,
		},
	}
	if data != nil {
		payload["data"] = data
	}
	writeJSON(w, status, payload)
}

func mapError(err error) (int, string, string) {
	switch {
	case errors.Is(err, service.ErrInvalidInput):
		return http.StatusBadRequest, "INVALID_INPUT", cleanMessage(err)
	case errors.Is(err, service.ErrIdentifierMismatch):
		return http.StatusBadRequest, "IDENTIFIER_MISMATCH", cleanMessage(err)
	case errors.Is(err, service.ErrPatientNotFound):
		return http.StatusNotFound, "PATIENT_NOT_FOUND", "pasien tidak ditemukan"
	case errors.Is(err, service.ErrPoliNotFound):
		return http.StatusNotFound, "POLI_NOT_FOUND", "poli tidak ditemukan atau tidak aktif"
	case errors.Is(err, service.ErrDoctorNotFound):
		return http.StatusNotFound, "DOCTOR_NOT_FOUND", "dokter tidak tersedia"
	case errors.Is(err, service.ErrDoctorRequired):
		return http.StatusBadRequest, "DOCTOR_REQUIRED", "dokter harus dipilih karena tersedia lebih dari satu"
	case errors.Is(err, service.ErrPetugasNotFound):
		return http.StatusNotFound, "PETUGAS_NOT_FOUND", "petugas pendaftaran tidak ditemukan atau tidak aktif"
	case errors.Is(err, service.ErrPoscheckNotFound):
		return http.StatusNotFound, "POSCHECK_NOT_FOUND", "lokasi pendaftaran tidak ditemukan"
	case errors.Is(err, service.ErrQuotaExceeded):
		return http.StatusUnprocessableEntity, "QUOTA_EXCEEDED", cleanMessage(err)
	case errors.Is(err, service.ErrBPJSIncomplete):
		return http.StatusUnprocessableEntity, "BPJS_INCOMPLETE", "data BPJS pasien belum lengkap"
	case errors.Is(err, service.ErrBPJSPoliMissing):
		return http.StatusUnprocessableEntity, "BPJS_POLI_MISSING", "kode poli BPJS belum terisi"
	case errors.Is(err, service.ErrBPJSInactive):
		return http.StatusUnprocessableEntity, "BPJS_INACTIVE", "kepersertaan BPJS pasien tidak aktif"
	case errors.Is(err, service.ErrBPJSDOBMismatch):
		return http.StatusUnprocessableEntity, "BPJS_DOB_MISMATCH", "tanggal lahir pasien tidak sesuai dengan data BPJS"
	case errors.Is(err, service.ErrPCareUnavailable):
		return http.StatusBadGateway, "PCARE_UNAVAILABLE", "verifikasi BPJS gagal"
	case errors.Is(err, service.ErrQueueBusy):
		return http.StatusServiceUnavailable, "QUEUE_BUSY", "antrian sedang sibuk, coba lagi"
	default:
		var already *service.AlreadyRegisteredError
		if errors.As(err, &already) {
			return http.StatusConflict, "ALREADY_REGISTERED", "pasien sudah terdaftar hari ini"
		}
		return http.StatusInternalServerError, "INTERNAL_ERROR", "terjadi kesalahan pada server"
	}
}

func cleanMessage(err error) string {
	message := err.Error()
	if index := strings.Index(message, ": "); index >= 0 {
		return message[index+2:]
	}
	return message
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return service.ErrInvalidInput
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return service.ErrInvalidInput
	}
	return nil
}

func queryOptionalBool(r *http.Request, key string) (*bool, error) {
	value := strings.TrimSpace(r.URL.Query().Get(key))
	if value == "" {
		return nil, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return nil, service.ErrInvalidInput
	}
	return &parsed, nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(payload)
}

func (s *Server) withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.corsOrigin != "" {
			w.Header().Set("Access-Control-Allow-Origin", s.corsOrigin)
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Add("Vary", "Origin")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		s.log.Info("http request", "method", r.Method, "path", r.URL.Path, "duration", time.Since(start))
	})
}

func (s *Server) Shutdown(ctx context.Context) error {
	return nil
}
