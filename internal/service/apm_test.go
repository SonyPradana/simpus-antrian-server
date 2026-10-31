package service

import (
	"context"
	"errors"
	"testing"

	"github.com/SonyPradana/simpus-antrian-server/internal/config"
	"github.com/SonyPradana/simpus-antrian-server/internal/model"
)

func TestClassifyAPMLookup(t *testing.T) {
	active := true
	inactive := false

	tests := []struct {
		name         string
		patient      model.Patient
		status       model.BPJSStatus
		statusErr    error
		verify       bool
		pcareMode    string
		wantMode     string
		wantRegister bool
		wantFallback string
		wantReason   string
	}{
		{
			name:         "non bpjs can register",
			patient:      model.Patient{ID: "1"},
			wantMode:     APMModeExistingActive,
			wantRegister: true,
		},
		{
			name:         "bpjs active can register",
			patient:      model.Patient{ID: "1", IsBPJS: true, NoKartu: "0000000000001"},
			status:       model.BPJSStatus{Active: &active},
			verify:       true,
			pcareMode:    "warn",
			wantMode:     APMModeExistingActive,
			wantRegister: true,
		},
		{
			name:         "bpjs inactive falls back to blank",
			patient:      model.Patient{ID: "1", IsBPJS: true, NoKartu: "0000000000001"},
			status:       model.BPJSStatus{Active: &inactive},
			verify:       true,
			pcareMode:    "warn",
			wantMode:     APMModeExistingInactive,
			wantFallback: APMFallbackBlank,
			wantReason:   "bpjs_inactive",
		},
		{
			name:         "bpjs unknown warn can register",
			patient:      model.Patient{ID: "1", IsBPJS: true, NoKartu: "0000000000001"},
			verify:       true,
			pcareMode:    "warn",
			wantMode:     APMModeExistingBPJSUnknown,
			wantRegister: true,
			wantReason:   "bpjs_unverified",
		},
		{
			name:         "bpjs unknown strict cannot register",
			patient:      model.Patient{ID: "1", IsBPJS: true, NoKartu: "0000000000001"},
			verify:       true,
			pcareMode:    "strict",
			wantMode:     APMModeExistingBPJSUnknown,
			wantFallback: APMFallbackBlank,
			wantReason:   "bpjs_unverified",
		},
		{
			name:         "bpjs unknown strict can register when verification skipped",
			patient:      model.Patient{ID: "1", IsBPJS: true, NoKartu: "0000000000001"},
			verify:       false,
			pcareMode:    "strict",
			wantMode:     APMModeExistingBPJSUnknown,
			wantRegister: true,
			wantReason:   "bpjs_unverified",
		},
		{
			name:         "incomplete bpjs card falls back to blank",
			patient:      model.Patient{ID: "1", IsBPJS: true, NoKartu: "123"},
			wantMode:     APMModeExistingBPJSUnknown,
			wantFallback: APMFallbackBlank,
			wantReason:   "bpjs_incomplete",
		},
		{
			name:         "bpjs date of birth mismatch falls back to blank",
			patient:      model.Patient{ID: "1", IsBPJS: true, NoKartu: "0000000000001"},
			statusErr:    ErrBPJSDOBMismatch,
			wantMode:     APMModeExistingBPJSUnknown,
			wantFallback: APMFallbackBlank,
			wantReason:   "bpjs_dob_mismatch",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mode, canRegister, fallback, reason := classifyAPMLookup(tt.patient, tt.status, tt.statusErr, tt.verify, tt.pcareMode)
			if mode != tt.wantMode {
				t.Fatalf("mode = %q, want %q", mode, tt.wantMode)
			}
			if canRegister != tt.wantRegister {
				t.Fatalf("canRegister = %v, want %v", canRegister, tt.wantRegister)
			}
			if fallback != tt.wantFallback {
				t.Fatalf("fallback = %q, want %q", fallback, tt.wantFallback)
			}
			if reason != tt.wantReason {
				t.Fatalf("reason = %q, want %q", reason, tt.wantReason)
			}
		})
	}
}

func TestResolveRegistrationContextDefaults(t *testing.T) {
	svc := &Service{cfg: config.Config{
		WorkerID:   "1",
		PoscheckID: "1",
		Location:   "Puskesmas Contoh",
	}}

	got, err := svc.resolveRegistrationContext(context.Background(), "", "")
	if err != nil {
		t.Fatal(err)
	}
	if got.PetugasID != "1" || got.PoscheckID != "1" || got.Location != "Puskesmas Contoh" {
		t.Fatalf("context = %+v, want configured defaults", got)
	}
}

func TestTakeAPMQueueRejectsInvalidRequests(t *testing.T) {
	svc := &Service{}
	tests := []struct {
		name string
		req  APMQueueRequest
	}{
		{name: "unknown mode", req: APMQueueRequest{Mode: "other", PoliID: "05"}},
		{name: "blank with identity", req: APMQueueRequest{Mode: APMQueueModeBlank, NoRM: "000001", PoliID: "01"}},
		{name: "blank with petugas", req: APMQueueRequest{Mode: APMQueueModeBlank, PetugasID: "1", PoliID: "01"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := svc.TakeAPMQueue(t.Context(), tt.req); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("err = %v, want ErrInvalidInput", err)
			}
		})
	}
}

func TestClassifyAPMLookupPCareUnavailable(t *testing.T) {
	mode, canRegister, fallback, reason := classifyAPMLookup(
		model.Patient{ID: "1", IsBPJS: true, NoKartu: "0000000000001"},
		model.BPJSStatus{},
		errors.New("pcare down"),
		true,
		"strict",
	)
	if mode != APMModeExistingBPJSUnknown || canRegister || fallback != APMFallbackBlank || reason != "bpjs_unverified" {
		t.Fatalf("unexpected classification: %s %v %s %s", mode, canRegister, fallback, reason)
	}
}
