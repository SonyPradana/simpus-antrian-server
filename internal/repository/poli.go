package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/SonyPradana/simpus-antrian-server/internal/model"
)

type PoliRepository struct {
	db DBTX
}

func NewPoliRepository(db DBTX) *PoliRepository {
	return &PoliRepository{db: db}
}

const poliSelect = `
SELECT
	LPAD(b.id, 2, '0'),
	COALESCE(b.kdPoliAntrian, ''),
	COALESCE(u.id, LPAD(b.id, 2, '0')),
	b.nmPoli,
	b.kdPoli_bpjs,
	COALESCE(b.poliSakit, 0),
	COALESCE(b.nomerurut, ''),
	COALESCE(NULLIF(b.kodepolihfis, ''), b.kdPoli_bpjs),
	COALESCE(ua.unit, ''),
	COALESCE(u.unit, ''),
	COALESCE((
		SELECT COUNT(*)
		FROM finger_unit f
		WHERE f.Poli = b.nmPoli
			AND f.UnitID = 'Pendaftaran'
			AND SUBSTRING(f.TanggalFingger, 1, 10) = CURDATE()
	), 0)
FROM bpjs_polyclinics_puskesmas b
LEFT JOIN units_antrian ua ON b.kdPoliAntrian = ua.id
LEFT JOIN units u ON u.id = LPAD(b.id, 2, '0')
WHERE b.Aktif = 1`

func (r *PoliRepository) ListActive(ctx context.Context) ([]model.Poli, error) {
	query := poliSelect + " ORDER BY b.id"
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []model.Poli
	for rows.Next() {
		item, err := scanPoli(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *PoliRepository) FindActiveByID(ctx context.Context, id string) (model.Poli, error) {
	query := poliSelect + `
		AND (
			LPAD(b.id, 2, '0') = ?
			OR b.kdPoliAntrian = ?
			OR CAST(b.id AS CHAR) = ?
		)
		LIMIT 1`

	row := r.db.QueryRowContext(ctx, query, id, id, id)
	item, err := scanPoli(row)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Poli{}, ErrNotFound
	}
	if err != nil {
		return model.Poli{}, err
	}
	return item, nil
}

type poliScanner interface {
	Scan(...any) error
}

func scanPoli(scanner poliScanner) (model.Poli, error) {
	var (
		item      model.Poli
		poliSakit int
	)
	err := scanner.Scan(
		&item.ID,
		&item.Code,
		&item.UnitID,
		&item.Name,
		&item.BPJSCode,
		&poliSakit,
		&item.Prefix,
		&item.PoliHFIS,
		&item.QueueUnitName,
		&item.UnitName,
		&item.QueueTotal,
	)
	if err != nil {
		return model.Poli{}, err
	}
	item.PoliSakit = poliSakit == 1
	return item, nil
}
