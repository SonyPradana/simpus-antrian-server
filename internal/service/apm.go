package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/SonyPradana/simpus-antrian-server/internal/model"
)

const (
	APMModeExistingActive      = "existing_active"
	APMModeExistingInactive    = "existing_inactive"
	APMModeExistingBPJSUnknown = "existing_bpjs_unknown"
	APMModeNew                 = "new"

	APMQueueModeRegistration = "registration"
	APMQueueModeBlank        = "blank"

	APMFallbackBlank = "blank"
)

type APMQueueRequest struct {
	Mode       string `json:"mode"`
	NIK        string `json:"nik,omitempty"`
	NoBPJS     string `json:"no_bpjs,omitempty"`
	BPJS       string `json:"bpjs,omitempty"`
	NoRM       string `json:"no_rm,omitempty"`
	PoliID     string `json:"poli_id"`
	DoctorCode string `json:"kode_dokter,omitempty"`
	PetugasID  string `json:"petugas_id,omitempty"`
	PoscheckID string `json:"poscheck_id,omitempty"`
	DryRun     bool   `json:"dry_run,omitempty"`
	PCare      *bool  `json:"pcare,omitempty"`
}

func (s *Service) APMOptions(ctx context.Context) (model.APMOptionsResult, error) {
	options, err := s.registration.ListOptions(ctx)
	if err != nil {
		return model.APMOptionsResult{}, err
	}
	return model.APMOptionsResult{
		Petugas:  options.Petugas,
		Poscheck: options.JenisPuskesmas,
		Terpilih: model.APMOptionSelection{
			PetugasID:  s.cfg.WorkerID,
			PoscheckID: s.cfg.PoscheckID,
			Location:   s.cfg.Location,
		},
	}, nil
}

func (s *Service) LookupAPM(ctx context.Context, req LookupRequest) (model.APMLookupResult, error) {
	nik, noBPJS, noRM, err := singleIdentifier(req.NIK, req.NoBPJS, req.BPJS, req.NoRM)
	if err != nil {
		return model.APMLookupResult{}, err
	}

	patient, err := s.resolvePatient(ctx, nik, noBPJS, noRM)
	if errors.Is(err, ErrPatientNotFound) {
		return model.APMLookupResult{
			Mode:     APMModeNew,
			Fallback: APMFallbackBlank,
			Reason:   "patient_not_found",
		}, nil
	}
	if err != nil {
		return model.APMLookupResult{}, err
	}

	verify := verifyPCare(req.PCare)
	status, statusErr := s.bpjsStatus(ctx, patient, nil, false, verify)
	if statusErr != nil {
		status = localBPJSStatus(patient, statusErr.Error())
	}

	mode, canRegister, fallback, reason := classifyAPMLookup(patient, status, statusErr, verify, s.cfg.PcareMode)
	return model.APMLookupResult{
		Mode:        mode,
		CanRegister: canRegister,
		Fallback:    fallback,
		Reason:      reason,
		Patient:     &patient,
		BPJS:        &status,
	}, nil
}

func (s *Service) TakeAPMQueue(ctx context.Context, req APMQueueRequest) (model.APMQueueResult, error) {
	mode := strings.ToLower(strings.TrimSpace(req.Mode))
	switch mode {
	case APMQueueModeRegistration:
		result, err := s.Register(ctx, RegisterRequest{
			NIK:        req.NIK,
			NoBPJS:     req.NoBPJS,
			BPJS:       req.BPJS,
			NoRM:       req.NoRM,
			PoliID:     req.PoliID,
			DoctorCode: req.DoctorCode,
			PetugasID:  req.PetugasID,
			PoscheckID: req.PoscheckID,
			DryRun:     req.DryRun,
			PCare:      req.PCare,
		})
		if err != nil {
			var already *AlreadyRegisteredError
			if errors.As(err, &already) {
				return apmResultFromRegistration(already.Result), err
			}
			return model.APMQueueResult{}, err
		}
		return apmResultFromRegistration(result), nil

	case APMQueueModeBlank:
		if strings.TrimSpace(req.NIK) != "" ||
			strings.TrimSpace(req.NoBPJS) != "" ||
			strings.TrimSpace(req.BPJS) != "" ||
			strings.TrimSpace(req.NoRM) != "" {
			return model.APMQueueResult{}, fmt.Errorf("%w: identitas tidak boleh diisi untuk mode blank", ErrInvalidInput)
		}
		if strings.TrimSpace(req.PetugasID) != "" {
			return model.APMQueueResult{}, fmt.Errorf("%w: petugas_id tidak dipakai untuk mode blank", ErrInvalidInput)
		}
		result, err := s.CreateNewQueue(ctx, QueueOnlyRequest{
			PoliID:     req.PoliID,
			DoctorCode: req.DoctorCode,
			PoscheckID: req.PoscheckID,
			DryRun:     req.DryRun,
		})
		if err != nil {
			return model.APMQueueResult{}, err
		}
		return model.APMQueueResult{
			Mode:              APMQueueModeBlank,
			NeedsRegistration: true,
			Poli:              result.Poli,
			Doctor:            result.Doctor,
			Queue:             result.Queue,
			PoscheckID:        result.PoscheckID,
			Location:          result.Location,
			DryRun:            result.DryRun,
		}, nil

	default:
		return model.APMQueueResult{}, fmt.Errorf("%w: mode harus registration atau blank", ErrInvalidInput)
	}
}

func classifyAPMLookup(patient model.Patient, status model.BPJSStatus, statusErr error, verify bool, pcareMode string) (string, bool, string, string) {
	if !patient.IsBPJS {
		return APMModeExistingActive, true, "", ""
	}

	if statusErr != nil {
		switch {
		case errors.Is(statusErr, ErrBPJSDOBMismatch):
			return APMModeExistingBPJSUnknown, false, APMFallbackBlank, "bpjs_dob_mismatch"
		case errors.Is(statusErr, ErrBPJSIncomplete):
			return APMModeExistingBPJSUnknown, false, APMFallbackBlank, "bpjs_incomplete"
		case errors.Is(statusErr, ErrPCareUnavailable):
			return APMModeExistingBPJSUnknown, false, APMFallbackBlank, "bpjs_unverified"
		default:
			return APMModeExistingBPJSUnknown, false, APMFallbackBlank, "bpjs_unverified"
		}
	}

	if len(digitsOnly(patient.NoKartu)) != 13 {
		return APMModeExistingBPJSUnknown, false, APMFallbackBlank, "bpjs_incomplete"
	}

	if status.Active != nil {
		if *status.Active {
			return APMModeExistingActive, true, "", ""
		}
		return APMModeExistingInactive, false, APMFallbackBlank, "bpjs_inactive"
	}

	if pcareMode == "strict" && verify {
		return APMModeExistingBPJSUnknown, false, APMFallbackBlank, "bpjs_unverified"
	}
	return APMModeExistingBPJSUnknown, true, "", "bpjs_unverified"
}

func apmResultFromRegistration(result model.RegistrationResult) model.APMQueueResult {
	patient := result.Patient
	bpjs := result.BPJS
	return model.APMQueueResult{
		Mode:              APMQueueModeRegistration,
		NeedsRegistration: false,
		Patient:           &patient,
		BPJS:              &bpjs,
		Poli:              result.Poli,
		Doctor:            result.Doctor,
		Queue:             result.Queue,
		PetugasID:         result.PetugasID,
		PoscheckID:        result.PoscheckID,
		Location:          result.Location,
		VisitID:           result.VisitID,
		DryRun:            result.DryRun,
		Existing:          result.Existing,
	}
}
