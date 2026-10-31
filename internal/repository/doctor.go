package repository

import (
	"context"
	"strconv"
	"strings"

	"github.com/SonyPradana/simpus-antrian-server/internal/model"
)

type DoctorRepository struct {
	db DBTX
}

func NewDoctorRepository(db DBTX) *DoctorRepository {
	return &DoctorRepository{db: db}
}

func (r *DoctorRepository) ListByPoliAndDate(ctx context.Context, poliCode, date string) ([]model.Doctor, error) {
	query := `
SELECT
	kodedokter,
	namadokter,
	jampraktek,
	kapasitas,
	kodepolidokter
FROM finger_mjkn_dokter
WHERE kodepolidokter = ?
	AND tanggaldokter = ?
GROUP BY kodedokter, namadokter, jampraktek, kapasitas, kodepolidokter
ORDER BY namadokter`

	rows, err := r.db.QueryContext(ctx, query, poliCode, date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []model.Doctor
	for rows.Next() {
		var (
			item     model.Doctor
			capacity string
		)
		if err := rows.Scan(&item.Code, &item.Name, &item.Schedule, &capacity, &item.PoliCode); err != nil {
			return nil, err
		}
		item.Capacity = parseCapacity(capacity)
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func parseCapacity(value string) int {
	capacity, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0
	}
	return capacity
}
