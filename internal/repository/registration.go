package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/SonyPradana/simpus-antrian-server/internal/model"
)

type RegistrationRepository struct {
	db DBTX
}

func NewRegistrationRepository(db DBTX) *RegistrationRepository {
	return &RegistrationRepository{db: db}
}

func (r *RegistrationRepository) ListOptions(ctx context.Context) (model.RegistrationOptions, error) {
	var result model.RegistrationOptions

	workers, err := r.list(ctx, `
SELECT id, COALESCE(nama, '')
FROM workers
WHERE aktif = 1
ORDER BY id`)
	if err != nil {
		return result, err
	}
	groupvisits, err := r.list(ctx, `
SELECT id, COALESCE(kelompok_kunjungan, '')
FROM groupvisits
ORDER BY id`)
	if err != nil {
		return result, err
	}
	poschecks, err := r.list(ctx, `
SELECT id, COALESCE(nama, '')
FROM poschecks
ORDER BY id`)
	if err != nil {
		return result, err
	}
	actions, err := r.list(ctx, `
SELECT id, COALESCE(tindak_lanjut, '')
FROM actions
ORDER BY id`)
	if err != nil {
		return result, err
	}
	consignors, err := r.list(ctx, `
SELECT id, COALESCE(pengirim, '')
FROM consignors
ORDER BY id`)
	if err != nil {
		return result, err
	}

	result.Petugas = workers
	result.JenisPasien = groupvisits
	result.JenisPuskesmas = poschecks
	result.JenisTindakan = actions
	result.JenisPengirim = consignors
	result.JenisKunjungan = []model.RegistrationOption{
		{ID: "0", Name: "Sehat"},
		{ID: "1", Name: "Sakit"},
	}
	return result, nil
}

func (r *RegistrationRepository) WorkerActive(ctx context.Context, id string) (bool, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM workers
WHERE id = ?
	AND aktif = 1`, id).Scan(&count)
	return count > 0, err
}

func (r *RegistrationRepository) Location(ctx context.Context, poscheckID string) (string, error) {
	var name string
	err := r.db.QueryRowContext(ctx, `
SELECT COALESCE(nama, '')
FROM poschecks
WHERE id = ?
LIMIT 1`, poscheckID).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return name, err
}

func (r *RegistrationRepository) list(ctx context.Context, query string) ([]model.RegistrationOption, error) {
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []model.RegistrationOption
	for rows.Next() {
		var item model.RegistrationOption
		if err := rows.Scan(&item.ID, &item.Name); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}
