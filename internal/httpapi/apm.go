package httpapi

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/SonyPradana/simpus-antrian-server/internal/service"
)

func (s *Server) apmLookup(w http.ResponseWriter, r *http.Request) {
	var req service.LookupRequest
	if err := decodeJSON(w, r, &req); err != nil {
		s.writeServiceError(w, err, nil)
		return
	}
	if pcare, err := queryOptionalBool(r, "pcare"); err != nil {
		s.writeServiceError(w, err, nil)
		return
	} else if pcare != nil {
		req.PCare = pcare
	}

	result, err := s.service.LookupAPM(r.Context(), req)
	if err != nil {
		s.writeServiceError(w, err, nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": result})
}

func (s *Server) apmOptions(w http.ResponseWriter, r *http.Request) {
	result, err := s.service.APMOptions(r.Context())
	if err != nil {
		s.writeServiceError(w, err, nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": result})
}

func (s *Server) apmListPoli(w http.ResponseWriter, r *http.Request) {
	items, err := s.service.ListPoli(r.Context())
	if err != nil {
		s.writeServiceError(w, err, nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items})
}

func (s *Server) apmListDoctors(w http.ResponseWriter, r *http.Request) {
	poliID := strings.TrimSpace(r.URL.Query().Get("poli_id"))
	if poliID == "" {
		poliID = strings.TrimSpace(r.URL.Query().Get("poli"))
	}
	result, err := s.service.ListDoctors(r.Context(), poliID)
	if err != nil {
		s.writeServiceError(w, err, nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": result})
}

func (s *Server) apmQueue(w http.ResponseWriter, r *http.Request) {
	var req service.APMQueueRequest
	if err := decodeJSON(w, r, &req); err != nil {
		s.writeServiceError(w, err, nil)
		return
	}
	if value := r.URL.Query().Get("dry_run"); value != "" {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			s.writeServiceError(w, service.ErrInvalidInput, nil)
			return
		}
		req.DryRun = parsed
	}

	result, err := s.service.TakeAPMQueue(r.Context(), req)
	if err != nil {
		var already *service.AlreadyRegisteredError
		if errors.As(err, &already) {
			s.writeServiceError(w, err, result)
			return
		}
		s.writeServiceError(w, err, nil)
		return
	}

	status := http.StatusCreated
	if result.DryRun {
		status = http.StatusOK
	}
	writeJSON(w, status, map[string]any{"data": result})
}

func (s *Server) withAPIKey(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.apiKey == "" {
			writeAPIError(w, http.StatusServiceUnavailable, "APM_NOT_CONFIGURED", "APM API key belum dikonfigurasi")
			return
		}
		provided := strings.TrimSpace(r.Header.Get("X-API-Key"))
		if subtle.ConstantTimeCompare([]byte(provided), []byte(s.apiKey)) != 1 {
			writeAPIError(w, http.StatusUnauthorized, "UNAUTHORIZED", "API key tidak valid")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeAPIError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]any{
			"code":    code,
			"message": message,
		},
	})
}
