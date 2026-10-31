package repository

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/SonyPradana/simpus-antrian-server/internal/model"
)

type PatientRepository struct {
	db DBTX
}

func NewPatientRepository(db DBTX) *PatientRepository {
	return &PatientRepository{db: db}
}

const patientQuery = `
SELECT
	p.id,
	COALESCE(p.nama, ''),
	COALESCE(p.nama_kk, ''),
	COALESCE(p.alamat, ''),
	COALESCE(o.dusun, ''),
	COALESCE(v.desa, ''),
	COALESCE(DATE_FORMAT(NULLIF(p.tgl_lahir, '0000-00-00'), '%Y-%m-%d'), ''),
	COALESCE(p.no_kartu, ''),
	COALESCE(p.nik, ''),
	p.sex_id,
	COALESCE(p.work_id, ''),
	p.orchard_id,
	p.village_id,
	p.typepatient_id,
	COALESCE(p.education_id, ''),
	COALESCE(t.jenis, ''),
	COALESCE(t.bpjs, 0),
	COALESCE(t.gakin, 0),
	COALESCE(t.bayar, 0),
	COALESCE(p.prolanis, ''),
	COALESCE(p.prb, '')
FROM patients p
LEFT JOIN typepatients t ON p.typepatient_id = t.id
LEFT JOIN orchards o ON p.orchard_id = o.id
LEFT JOIN villages v ON p.village_id = v.id
WHERE {{WHERE}}
LIMIT 1`

func (r *PatientRepository) FindByNIK(ctx context.Context, nik string) (model.Patient, error) {
	return r.findOne(ctx, "p.nik = ?", strings.TrimSpace(nik))
}

func (r *PatientRepository) FindByNoKartu(ctx context.Context, noKartu string) (model.Patient, error) {
	return r.findOne(ctx, "p.no_kartu = ?", strings.TrimSpace(noKartu))
}

func (r *PatientRepository) FindByID(ctx context.Context, id string) (model.Patient, error) {
	return r.findOne(ctx, "p.id = ?", strings.TrimSpace(id))
}

func (r *PatientRepository) findOne(ctx context.Context, condition string, value string) (model.Patient, error) {
	if value == "" {
		return model.Patient{}, ErrNotFound
	}

	query := strings.Replace(patientQuery, "{{WHERE}}", condition, 1)
	var (
		patient            model.Patient
		tglLahir           string
		isBPJS, gakin, pay int
	)

	err := r.db.QueryRowContext(ctx, query, value).Scan(
		&patient.ID,
		&patient.Nama,
		&patient.NamaKK,
		&patient.Alamat,
		&patient.Dusun,
		&patient.Desa,
		&tglLahir,
		&patient.NoKartu,
		&patient.NIK,
		&patient.SexID,
		&patient.WorkID,
		&patient.OrchardID,
		&patient.VillageID,
		&patient.TypepatientID,
		&patient.EducationID,
		&patient.JenisPasien,
		&isBPJS,
		&gakin,
		&pay,
		&patient.Prolanis,
		&patient.PRB,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Patient{}, ErrNotFound
	}
	if err != nil {
		return model.Patient{}, err
	}

	patient.IsBPJS = isBPJS == 1
	patient.Gakin = gakin == 1
	patient.Bayar = pay == 1
	patient.TglLahir = tglLahir
	if tglLahir != "" {
		parsed, parseErr := time.Parse("2006-01-02", tglLahir)
		if parseErr == nil {
			patient.TglLahirTime = parsed
		}
	}
	return patient, nil
}
