package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/SonyPradana/simpus-antrian-server/internal/config"
	"github.com/SonyPradana/simpus-antrian-server/internal/model"
	"github.com/SonyPradana/simpus-antrian-server/internal/pcare"
	"github.com/SonyPradana/simpus-antrian-server/internal/repository"
)

var (
	ErrInvalidInput       = errors.New("invalid input")
	ErrPatientNotFound    = errors.New("patient not found")
	ErrPoliNotFound       = errors.New("poli not found")
	ErrDoctorNotFound     = errors.New("doctor not found")
	ErrDoctorRequired     = errors.New("doctor must be selected")
	ErrQuotaExceeded      = errors.New("doctor quota is full")
	ErrBPJSIncomplete     = errors.New("patient BPJS data is incomplete")
	ErrBPJSPoliMissing    = errors.New("BPJS poli code is missing")
	ErrBPJSInactive       = errors.New("BPJS membership is not active")
	ErrBPJSDOBMismatch    = errors.New("BPJS date of birth does not match")
	ErrIdentifierMismatch = errors.New("patient identifiers do not match")
	ErrPCareUnavailable   = errors.New("BPJS verification is unavailable")
	ErrQueueBusy          = errors.New("queue is busy")
	ErrPetugasNotFound    = errors.New("registration worker not found")
	ErrPoscheckNotFound   = errors.New("registration location not found")
)

type AlreadyRegisteredError struct {
	Result model.RegistrationResult
}

func (e *AlreadyRegisteredError) Error() string {
	return "patient is already registered"
}

type Service struct {
	db           *sql.DB
	cfg          config.Config
	loc          *time.Location
	log          *slog.Logger
	patient      *repository.PatientRepository
	poli         *repository.PoliRepository
	doctor       *repository.DoctorRepository
	queue        *repository.QueueRepository
	registration *repository.RegistrationRepository
	pcare        *pcare.Client
}

type LookupRequest struct {
	NIK    string `json:"nik,omitempty"`
	NoBPJS string `json:"no_bpjs,omitempty"`
	BPJS   string `json:"bpjs,omitempty"`
	NoRM   string `json:"no_rm,omitempty"`
	PCare  *bool  `json:"pcare,omitempty"`
}

type LookupResult struct {
	Patient model.Patient    `json:"patient"`
	BPJS    model.BPJSStatus `json:"bpjs"`
}

type DoctorList struct {
	Poli         model.Poli     `json:"poli"`
	Doctors      []model.Doctor `json:"dokter"`
	AutoSelected bool           `json:"otomatis"`
	Selected     *model.Doctor  `json:"dipilih,omitempty"`
}

type RegisterRequest struct {
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

type QueueOnlyRequest struct {
	PoliID     string `json:"poli_id"`
	DoctorCode string `json:"kode_dokter,omitempty"`
	PoscheckID string `json:"poscheck_id,omitempty"`
	DryRun     bool   `json:"dry_run,omitempty"`
}

func New(db *sql.DB, cfg config.Config, logger *slog.Logger) (*Service, error) {
	settings, err := pcare.LoadSettings(context.Background(), db, cfg.PcareBaseURL, cfg.PcareMode, cfg.PcareKodeAplikasi, cfg.UserAgent, cfg.PcareCacheTTL, cfg.HTTPTimeout)
	if err != nil {
		logger.Warn("pcare settings unavailable; pcare verification disabled", "error", err)
		settings = pcare.Settings{
			BaseURL:      cfg.PcareBaseURL,
			Mode:         cfg.PcareMode,
			KodeAplikasi: cfg.PcareKodeAplikasi,
			UserAgent:    cfg.UserAgent,
			CacheTTL:     cfg.PcareCacheTTL,
			HTTPTimeout:  cfg.HTTPTimeout,
		}
	}
	svc := &Service{
		db:           db,
		cfg:          cfg,
		loc:          cfg.LocationPtr(),
		log:          logger,
		patient:      repository.NewPatientRepository(db),
		poli:         repository.NewPoliRepository(db),
		doctor:       repository.NewDoctorRepository(db),
		queue:        repository.NewQueueRepository(db),
		registration: repository.NewRegistrationRepository(db),
		pcare:        pcare.New(settings),
	}
	if !cfg.LocationExplicit {
		location, err := svc.registration.Location(context.Background(), cfg.PoscheckID)
		switch {
		case err == nil && location != "":
			svc.cfg.Location = location
		case err != nil && !errors.Is(err, repository.ErrNotFound):
			logger.Warn("registration location unavailable", "poscheck_id", cfg.PoscheckID, "error", err)
		}
	}
	return svc, nil
}

func (s *Service) ListPoli(ctx context.Context) ([]model.Poli, error) {
	return s.poli.ListActive(ctx)
}

func (s *Service) ListDoctors(ctx context.Context, poliID string) (DoctorList, error) {
	poli, err := s.resolvePoli(ctx, poliID)
	if err != nil {
		return DoctorList{}, err
	}
	today, err := s.queue.ServerDate(ctx)
	if err != nil {
		return DoctorList{}, err
	}
	doctors, err := s.doctor.ListByPoliAndDate(ctx, poli.BPJSCode, today)
	if err != nil {
		return DoctorList{}, err
	}
	result := DoctorList{Poli: poli, Doctors: doctors}
	if len(doctors) == 1 {
		result.AutoSelected = true
		result.Selected = &doctors[0]
	}
	return result, nil
}

func (s *Service) RegistrationOptions(ctx context.Context) (model.RegistrationOptions, error) {
	options, err := s.registration.ListOptions(ctx)
	if err != nil {
		return model.RegistrationOptions{}, err
	}
	options.Selected = model.RegistrationSelection{
		PetugasID:        s.cfg.WorkerID,
		JenisPasienID:    s.cfg.GroupvisitID,
		JenisPuskesmasID: s.cfg.PoscheckID,
		JenisTindakanID:  s.cfg.ActionID,
		JenisPengirimID:  s.cfg.ConsignorID,
		JenisKunjunganID: s.cfg.JenisKunjungan,
	}
	options.Location = s.cfg.Location
	options.ConfigFile = s.cfg.ConfigFile
	return options, nil
}

func (s *Service) LookupPatient(ctx context.Context, req LookupRequest) (LookupResult, error) {
	nik, noBPJS, noRM, err := singleIdentifier(req.NIK, req.NoBPJS, req.BPJS, req.NoRM)
	if err != nil {
		return LookupResult{}, err
	}
	patient, err := s.resolvePatient(ctx, nik, noBPJS, noRM)
	if err != nil {
		return LookupResult{}, err
	}
	status, err := s.bpjsStatus(ctx, patient, nil, false, verifyPCare(req.PCare))
	if err != nil {
		status = localBPJSStatus(patient, err.Error())
	}
	return LookupResult{Patient: patient, BPJS: status}, nil
}

func (s *Service) Register(ctx context.Context, req RegisterRequest) (model.RegistrationResult, error) {
	nik, noBPJS, noRM, err := singleIdentifier(req.NIK, req.NoBPJS, req.BPJS, req.NoRM)
	if err != nil {
		return model.RegistrationResult{}, err
	}
	patient, err := s.resolvePatient(ctx, nik, noBPJS, noRM)
	if err != nil {
		return model.RegistrationResult{}, err
	}
	if strings.TrimSpace(req.PoliID) == "" {
		return model.RegistrationResult{}, fmt.Errorf("%w: poli_id is required", ErrInvalidInput)
	}

	poli, err := s.resolvePoli(ctx, req.PoliID)
	if err != nil {
		return model.RegistrationResult{}, err
	}
	regCtx, err := s.resolveRegistrationContext(ctx, req.PetugasID, req.PoscheckID)
	if err != nil {
		return model.RegistrationResult{}, err
	}
	today, err := s.queue.ServerDate(ctx)
	if err != nil {
		return model.RegistrationResult{}, err
	}
	doctors, err := s.doctor.ListByPoliAndDate(ctx, poli.BPJSCode, today)
	if err != nil {
		return model.RegistrationResult{}, err
	}
	if len(doctors) == 0 {
		return model.RegistrationResult{}, fmt.Errorf("%w: no doctor is scheduled for this poli today", ErrDoctorNotFound)
	}

	doctor, autoSelected, err := selectDoctor(doctors, req.DoctorCode)
	if err != nil {
		return model.RegistrationResult{}, err
	}

	bpjsStatus, err := s.bpjsStatus(ctx, patient, &poli, true, verifyPCare(req.PCare))
	if err != nil {
		return model.RegistrationResult{}, err
	}

	result := model.RegistrationResult{
		Patient:    patient,
		BPJS:       bpjsStatus,
		Poli:       poli,
		Doctor:     doctor,
		PetugasID:  regCtx.PetugasID,
		PoscheckID: regCtx.PoscheckID,
		Location:   regCtx.Location,
		DryRun:     req.DryRun,
	}
	if autoSelected {
		s.log.Info("doctor selected automatically", "poli_id", poli.ID, "kode_dokter", doctor.Code)
	}

	conn, err := s.db.Conn(ctx)
	if err != nil {
		return model.RegistrationResult{}, err
	}
	defer conn.Close()

	lockName := "apm:queue:" + today
	if err := s.queue.AcquireLock(ctx, conn, lockName, 5); err != nil {
		return model.RegistrationResult{}, fmt.Errorf("%w: %v", ErrQueueBusy, err)
	}
	defer s.releaseQueueLock(conn, lockName)

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return model.RegistrationResult{}, err
	}
	defer tx.Rollback()

	duplicateVisits, err := s.queue.CountDuplicateVisits(ctx, tx, today, patient.ID, poli.UnitID, patient.IsBPJS)
	if err != nil {
		return model.RegistrationResult{}, err
	}
	duplicateQueues, err := s.queue.CountDuplicateQueue(ctx, tx, today, patient.ID, poli.Name)
	if err != nil {
		return model.RegistrationResult{}, err
	}
	if duplicateVisits > 0 || duplicateQueues > 0 {
		existing, found, err := s.queue.FindExistingQueue(ctx, tx, today, patient.ID, poli.Name)
		if err != nil {
			return model.RegistrationResult{}, err
		}
		if found {
			result.Queue = existing
			result.Existing = true
			if result.Doctor.Code == "" {
				result.Doctor.Code = existing.DoctorCode
				result.Doctor.Name = existing.DoctorName
			}
		}
		return result, &AlreadyRegisteredError{Result: result}
	}

	globalCounter, poliCounter, err := s.queue.CurrentCounters(ctx, tx, today, poli.Name)
	if err != nil {
		return model.RegistrationResult{}, err
	}
	globalNumber := globalCounter + 1
	poliNumber := poliCounter + 1
	if doctor.Capacity > 0 && poliNumber > doctor.Capacity && !s.cfg.AllowOverQuota {
		return model.RegistrationResult{}, fmt.Errorf("%w: kapasitas %d, nomor berikutnya %d", ErrQuotaExceeded, doctor.Capacity, poliNumber)
	}
	waitingAhead, err := s.queue.WaitingAhead(ctx, tx, today, poli.Name, poliNumber)
	if err != nil {
		return model.RegistrationResult{}, err
	}

	queueCode := repository.NextQueueCode(poli.Prefix, poliNumber)
	remainingQuota := 0
	if doctor.Capacity > 0 {
		remainingQuota = doctor.Capacity - poliNumber
	}
	result.Queue = model.Queue{
		Code:           queueCode,
		Number:         poliNumber,
		GlobalNumber:   globalNumber,
		Date:           today,
		Poli:           poli.Name,
		DoctorName:     doctor.Name,
		DoctorCode:     doctor.Code,
		WaitingAhead:   waitingAhead,
		RemainingQuota: remainingQuota,
	}

	if req.DryRun {
		queueTime, err := s.queue.ServerDateTime(ctx)
		if err != nil {
			return model.RegistrationResult{}, err
		}
		result.Queue.Waktu = queueTime
		return result, nil
	}

	queueTime, err := s.queue.ServerDateTimeTx(ctx, tx)
	if err != nil {
		return model.RegistrationResult{}, err
	}
	result.Queue.Waktu = queueTime

	dob := patient.TglLahirTime
	if dob.IsZero() {
		return model.RegistrationResult{}, fmt.Errorf("%w: patient date of birth is invalid", ErrInvalidInput)
	}
	visitDate, err := time.ParseInLocation("2006-01-02", today, s.loc)
	if err != nil {
		return model.RegistrationResult{}, err
	}
	age := ageOn(dob, visitDate)
	groupage := groupageID(dob, visitDate)
	_, week := visitDate.ISOWeek()

	visitArgs := []any{
		patient.ID,
		today,
		regCtx.PoscheckID,
		regCtx.PetugasID,
		patient.NamaKK,
		patient.Nama,
		patient.WorkID,
		patient.Alamat,
		patient.OrchardID,
		patient.VillageID,
		strconv.Itoa(dob.Day()),
		strconv.Itoa(int(dob.Month())),
		strconv.Itoa(age),
		strconv.Itoa(groupage),
		s.cfg.GroupvisitID,
		patient.TypepatientID,
		s.cfg.ActionID,
		s.cfg.ConsignorID,
		patient.SexID,
		poli.UnitID,
		patient.NoKartu,
		patient.NIK,
		strconv.Itoa(week),
		strconv.Itoa(int(visitDate.Month())),
		strconv.Itoa(visitDate.Year()),
		s.cfg.JenisKunjungan,
		patient.EducationID,
		boolDigit(patient.Gakin),
		poli.BPJSCode,
		"0",
		boolDigit(patient.Bayar),
		queueTime,
		patient.Prolanis,
		patient.PRB,
	}

	if err := s.queue.CallSPapmsimpus(ctx, tx, visitArgs...); err != nil {
		_ = s.queue.Compensate(ctx, tx, patient.ID, queueTime, queueCode, 0)
		return model.RegistrationResult{}, fmt.Errorf("create visit: %w", err)
	}
	visitID, err := s.queue.LatestVisitID(ctx, tx, patient.ID, today)
	if err != nil {
		_ = s.queue.Compensate(ctx, tx, patient.ID, queueTime, queueCode, 0)
		return model.RegistrationResult{}, fmt.Errorf("read visit id: %w", err)
	}

	sisa := "0"
	if doctor.Capacity > 0 {
		sisa = strconv.Itoa(doctor.Capacity - poliNumber)
	}
	queueArgs := []any{
		queueCode,
		queueTime,
		patient.ID,
		patient.NoKartu,
		patient.Nama,
		strconv.Itoa(age),
		patient.JenisPasien,
		strconv.Itoa(globalNumber),
		strconv.Itoa(poliNumber),
		poli.Name,
		doctor.Name,
		doctor.Code,
		doctor.Schedule,
		strconv.Itoa(doctor.Capacity),
		sisa,
		poli.PoliHFIS,
		"Tidak",
		regCtx.Location,
		bpjsStatus.ProviderCode,
		bpjsStatus.ProviderName,
	}
	if err := s.queue.CallSPinsertFingerUnit(ctx, tx, queueArgs...); err != nil {
		_ = s.queue.Compensate(ctx, tx, patient.ID, queueTime, queueCode, visitID)
		return model.RegistrationResult{}, fmt.Errorf("create queue: %w", err)
	}
	if err := s.queue.InsertFingerReport(ctx, tx, queueCode, patient.ID, poli.BPJSCode); err != nil {
		_ = s.queue.Compensate(ctx, tx, patient.ID, queueTime, queueCode, visitID)
		return model.RegistrationResult{}, fmt.Errorf("create queue report: %w", err)
	}
	if err := tx.Commit(); err != nil {
		_ = s.queue.Compensate(ctx, tx, patient.ID, queueTime, queueCode, visitID)
		return model.RegistrationResult{}, fmt.Errorf("commit registration: %w", err)
	}

	result.VisitID = visitID
	return result, nil
}

func (s *Service) CreateEmptyQueue(ctx context.Context, req QueueOnlyRequest) (model.QueueOnlyResult, error) {
	return s.createQueueOnly(ctx, req, "empty")
}

func (s *Service) CreateNewQueue(ctx context.Context, req QueueOnlyRequest) (model.QueueOnlyResult, error) {
	return s.createQueueOnly(ctx, req, "new")
}

func (s *Service) createQueueOnly(ctx context.Context, req QueueOnlyRequest, kind string) (model.QueueOnlyResult, error) {
	if strings.TrimSpace(req.PoliID) == "" {
		return model.QueueOnlyResult{}, fmt.Errorf("%w: poli_id is required", ErrInvalidInput)
	}
	poli, err := s.resolvePoli(ctx, req.PoliID)
	if err != nil {
		return model.QueueOnlyResult{}, err
	}
	regCtx, err := s.resolveRegistrationContext(ctx, "", req.PoscheckID)
	if err != nil {
		return model.QueueOnlyResult{}, err
	}
	today, err := s.queue.ServerDate(ctx)
	if err != nil {
		return model.QueueOnlyResult{}, err
	}
	doctors, err := s.doctor.ListByPoliAndDate(ctx, poli.BPJSCode, today)
	if err != nil {
		return model.QueueOnlyResult{}, err
	}
	if len(doctors) == 0 {
		return model.QueueOnlyResult{}, fmt.Errorf("%w: no doctor is scheduled for this poli today", ErrDoctorNotFound)
	}
	doctor, autoSelected, err := selectDoctor(doctors, req.DoctorCode)
	if err != nil {
		return model.QueueOnlyResult{}, err
	}
	if autoSelected {
		s.log.Info("doctor selected automatically", "poli_id", poli.ID, "kode_dokter", doctor.Code)
	}

	result := model.QueueOnlyResult{
		Type:       kind,
		Poli:       poli,
		Doctor:     doctor,
		PoscheckID: regCtx.PoscheckID,
		Location:   regCtx.Location,
		DryRun:     req.DryRun,
	}

	conn, err := s.db.Conn(ctx)
	if err != nil {
		return model.QueueOnlyResult{}, err
	}
	defer conn.Close()

	lockName := "apm:queue:" + today
	if err := s.queue.AcquireLock(ctx, conn, lockName, 5); err != nil {
		return model.QueueOnlyResult{}, fmt.Errorf("%w: %v", ErrQueueBusy, err)
	}
	defer s.releaseQueueLock(conn, lockName)

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return model.QueueOnlyResult{}, err
	}
	defer tx.Rollback()

	globalCounter, poliCounter, err := s.queue.CurrentCounters(ctx, tx, today, poli.Name)
	if err != nil {
		return model.QueueOnlyResult{}, err
	}
	globalNumber := globalCounter + 1
	poliNumber := poliCounter + 1
	if doctor.Capacity > 0 && poliNumber > doctor.Capacity && !s.cfg.AllowOverQuota {
		return model.QueueOnlyResult{}, fmt.Errorf("%w: kapasitas %d, nomor berikutnya %d", ErrQuotaExceeded, doctor.Capacity, poliNumber)
	}
	waitingAhead, err := s.queue.WaitingAhead(ctx, tx, today, poli.Name, poliNumber)
	if err != nil {
		return model.QueueOnlyResult{}, err
	}
	queueCode := repository.NextQueueCode(poli.Prefix, poliNumber)
	remainingQuota := 0
	if doctor.Capacity > 0 {
		remainingQuota = doctor.Capacity - poliNumber
	}
	result.Queue = model.Queue{
		Code:           queueCode,
		Number:         poliNumber,
		GlobalNumber:   globalNumber,
		Date:           today,
		Poli:           poli.Name,
		DoctorName:     doctor.Name,
		DoctorCode:     doctor.Code,
		WaitingAhead:   waitingAhead,
		RemainingQuota: remainingQuota,
	}
	if req.DryRun {
		queueTime, err := s.queue.ServerDateTime(ctx)
		if err != nil {
			return model.QueueOnlyResult{}, err
		}
		result.Queue.Waktu = queueTime
		return result, nil
	}

	queueTime, err := s.queue.ServerDateTimeTx(ctx, tx)
	if err != nil {
		return model.QueueOnlyResult{}, err
	}
	result.Queue.Waktu = queueTime
	sisa := "0"
	if doctor.Capacity > 0 {
		sisa = strconv.Itoa(doctor.Capacity - poliNumber)
	}
	queueArgs := []any{
		queueCode,
		queueTime,
		"",
		"",
		"",
		"",
		"",
		strconv.Itoa(globalNumber),
		strconv.Itoa(poliNumber),
		poli.Name,
		doctor.Name,
		doctor.Code,
		doctor.Schedule,
		strconv.Itoa(doctor.Capacity),
		sisa,
		poli.PoliHFIS,
		"Tidak",
		regCtx.Location,
		"",
		"",
	}
	if err := s.queue.CallSPinsertFingerUnit(ctx, tx, queueArgs...); err != nil {
		_ = s.queue.CompensateQueueOnly(ctx, tx, queueCode, queueTime)
		return model.QueueOnlyResult{}, fmt.Errorf("create queue: %w", err)
	}
	if err := s.queue.InsertEmptyFingerReport(ctx, tx, queueCode, queueTime, "-"); err != nil {
		_ = s.queue.CompensateQueueOnly(ctx, tx, queueCode, queueTime)
		return model.QueueOnlyResult{}, fmt.Errorf("create queue report: %w", err)
	}
	if err := tx.Commit(); err != nil {
		_ = tx.Rollback()
		_ = s.queue.CompensateQueueOnly(ctx, conn, queueCode, queueTime)
		return model.QueueOnlyResult{}, fmt.Errorf("commit queue: %w", err)
	}
	return result, nil
}

func (s *Service) releaseQueueLock(conn *sql.Conn, name string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	s.queue.ReleaseLock(ctx, conn, name)
}

func singleIdentifier(nik, noBPJS, bpjs, noRM string) (string, string, string, error) {
	nik = strings.TrimSpace(nik)
	noBPJS = strings.TrimSpace(noBPJS)
	bpjs = strings.TrimSpace(bpjs)
	noRM = strings.TrimSpace(noRM)
	if noBPJS != "" && bpjs != "" {
		return "", "", "", fmt.Errorf("%w: isi salah satu dari no_bpjs atau bpjs", ErrInvalidInput)
	}
	if noBPJS == "" {
		noBPJS = bpjs
	}
	count := 0
	if nik != "" {
		count++
	}
	if noBPJS != "" {
		count++
	}
	if noRM != "" {
		count++
	}
	if count != 1 {
		return "", "", "", fmt.Errorf("%w: isi tepat salah satu dari nik, no_bpjs/bpjs, atau no_rm", ErrInvalidInput)
	}
	return nik, noBPJS, noRM, nil
}

func (s *Service) resolvePatient(ctx context.Context, nik, noBPJS, noRM string) (model.Patient, error) {
	nik = strings.TrimSpace(nik)
	noBPJS = strings.TrimSpace(noBPJS)
	noRM = strings.TrimSpace(noRM)
	if nik == "" && noBPJS == "" && noRM == "" {
		return model.Patient{}, fmt.Errorf("%w: nik, no_bpjs, or no_rm is required", ErrInvalidInput)
	}

	var (
		patient model.Patient
		err     error
	)
	switch {
	case nik != "":
		patient, err = s.patient.FindByNIK(ctx, nik)
	case noBPJS != "":
		patient, err = s.patient.FindByNoKartu(ctx, noBPJS)
	case noRM != "":
		patient, err = s.patient.FindByID(ctx, noRM)
	}
	if errors.Is(err, repository.ErrNotFound) {
		return model.Patient{}, ErrPatientNotFound
	}
	if err != nil {
		return model.Patient{}, err
	}

	check := func(value string, find func(context.Context, string) (model.Patient, error)) error {
		if value == "" {
			return nil
		}
		other, err := find(ctx, value)
		if errors.Is(err, repository.ErrNotFound) {
			return ErrIdentifierMismatch
		}
		if err != nil {
			return err
		}
		if other.ID != patient.ID {
			return ErrIdentifierMismatch
		}
		return nil
	}
	if err := check(nik, s.patient.FindByNIK); err != nil {
		return model.Patient{}, err
	}
	if err := check(noBPJS, s.patient.FindByNoKartu); err != nil {
		return model.Patient{}, err
	}
	if err := check(noRM, s.patient.FindByID); err != nil {
		return model.Patient{}, err
	}
	return patient, nil
}

type registrationContext struct {
	PetugasID  string
	PoscheckID string
	Location   string
}

func (s *Service) resolveRegistrationContext(ctx context.Context, petugasID, poscheckID string) (registrationContext, error) {
	result := registrationContext{
		PetugasID:  s.cfg.WorkerID,
		PoscheckID: s.cfg.PoscheckID,
		Location:   s.cfg.Location,
	}

	if petugasID = strings.TrimSpace(petugasID); petugasID != "" {
		active, err := s.registration.WorkerActive(ctx, petugasID)
		if err != nil {
			return registrationContext{}, err
		}
		if !active {
			return registrationContext{}, ErrPetugasNotFound
		}
		result.PetugasID = petugasID
	}

	if poscheckID = strings.TrimSpace(poscheckID); poscheckID != "" {
		location, err := s.registration.Location(ctx, poscheckID)
		if errors.Is(err, repository.ErrNotFound) {
			return registrationContext{}, ErrPoscheckNotFound
		}
		if err != nil {
			return registrationContext{}, err
		}
		result.PoscheckID = poscheckID
		result.Location = location
	}

	return result, nil
}

func (s *Service) resolvePoli(ctx context.Context, id string) (model.Poli, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return model.Poli{}, fmt.Errorf("%w: poli_id is required", ErrInvalidInput)
	}
	poli, err := s.poli.FindActiveByID(ctx, id)
	if errors.Is(err, repository.ErrNotFound) {
		return model.Poli{}, ErrPoliNotFound
	}
	return poli, err
}

func selectDoctor(doctors []model.Doctor, code string) (model.Doctor, bool, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		if len(doctors) == 1 {
			return doctors[0], true, nil
		}
		return model.Doctor{}, false, ErrDoctorRequired
	}
	for _, doctor := range doctors {
		if doctor.Code == code {
			return doctor, false, nil
		}
	}
	return model.Doctor{}, false, ErrDoctorNotFound
}

func (s *Service) bpjsStatus(ctx context.Context, patient model.Patient, poli *model.Poli, forRegistration, verify bool) (model.BPJSStatus, error) {
	status := localBPJSStatus(patient, "")
	if !verify {
		if patient.IsBPJS {
			status.Message = "verifikasi BPJS dilewati"
		}
		return status, nil
	}
	if !patient.IsBPJS {
		return status, nil
	}
	if len(digitsOnly(patient.NoKartu)) != 13 {
		if forRegistration {
			return status, ErrBPJSIncomplete
		}
		status.Message = "nomor kartu BPJS tidak lengkap"
		return status, nil
	}
	if forRegistration && (poli == nil || strings.TrimSpace(poli.BPJSCode) == "") {
		return status, ErrBPJSPoliMissing
	}
	if !s.pcare.Enabled() {
		status.Message = "verifikasi BPJS tidak aktif"
		if forRegistration && s.cfg.PcareMode == "strict" {
			return status, ErrPCareUnavailable
		}
		return status, nil
	}

	participant, err := s.pcare.Verify(ctx, patient.NoKartu)
	if err != nil {
		status.Message = err.Error()
		if forRegistration && s.cfg.PcareMode == "strict" {
			return status, fmt.Errorf("%w: %v", ErrPCareUnavailable, err)
		}
		return status, nil
	}

	if !sameDate(participant.TglLahir, patient.TglLahir) {
		return status, ErrBPJSDOBMismatch
	}
	active := participant.Aktif
	status = model.BPJSStatus{
		IsBPJS:          true,
		NoKartu:         patient.NoKartu,
		Source:          "pcare",
		Verified:        true,
		Active:          &active,
		Status:          participant.KetAktif,
		JenisPeserta:    participant.JnsPeserta.Nama,
		Kelas:           participant.JnsKelas.Nama,
		ProviderCode:    participant.KdProviderPst.KdProvider,
		ProviderName:    participant.KdProviderPst.NmProvider,
		TglMulaiAktif:   participant.TglMulaiAktif,
		TglAkhirBerlaku: participant.TglAkhirBerlaku,
	}
	if !participant.Aktif {
		if forRegistration {
			return status, ErrBPJSInactive
		}
		status.Message = participant.KetAktif
	}
	return status, nil
}

func localBPJSStatus(patient model.Patient, message string) model.BPJSStatus {
	status := model.BPJSStatus{
		IsBPJS:  patient.IsBPJS,
		NoKartu: patient.NoKartu,
		Source:  "local",
		Message: message,
	}
	if patient.IsBPJS {
		status.Status = patient.JenisPasien
	}
	return status
}

func sameDate(pcareDate, patientDate string) bool {
	if strings.TrimSpace(pcareDate) == "" || strings.TrimSpace(patientDate) == "" {
		return true
	}
	parsed, err := time.Parse("02-01-2006", pcareDate)
	if err != nil {
		return false
	}
	return parsed.Format("2006-01-02") == patientDate
}

func ageOn(dob, on time.Time) int {
	years := on.Year() - dob.Year()
	if on.Month() < dob.Month() || (on.Month() == dob.Month() && on.Day() < dob.Day()) {
		years--
	}
	if years < 0 {
		return 0
	}
	return years
}

func groupageID(dob, on time.Time) int {
	days := int(on.Sub(dob).Hours() / 24)
	years := ageOn(dob, on)
	switch {
	case days <= 7:
		return 1
	case days <= 30:
		return 2
	case years < 1:
		return 3
	case years <= 4:
		return 4
	case years <= 9:
		return 5
	case years <= 14:
		return 6
	case years <= 19:
		return 7
	case years <= 44:
		return 8
	case years <= 54:
		return 9
	case years <= 59:
		return 10
	case years <= 69:
		return 11
	default:
		return 12
	}
}

func boolDigit(value bool) string {
	if value {
		return "1"
	}
	return "0"
}

func digitsOnly(value string) string {
	var builder strings.Builder
	for _, char := range value {
		if char >= '0' && char <= '9' {
			builder.WriteRune(char)
		}
	}
	return builder.String()
}

func verifyPCare(value *bool) bool {
	return value == nil || *value
}
