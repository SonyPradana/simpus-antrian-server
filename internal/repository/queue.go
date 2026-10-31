package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/SonyPradana/simpus-antrian-server/internal/model"
)

type QueueRepository struct {
	db DBTX
}

func NewQueueRepository(db DBTX) *QueueRepository {
	return &QueueRepository{db: db}
}

func (r *QueueRepository) ServerDate(ctx context.Context) (string, error) {
	var date string
	err := r.db.QueryRowContext(ctx, "SELECT DATE_FORMAT(CURDATE(), '%Y-%m-%d')").Scan(&date)
	return date, err
}

func (r *QueueRepository) ServerDateTime(ctx context.Context) (string, error) {
	var value string
	err := r.db.QueryRowContext(ctx, "SELECT DATE_FORMAT(NOW(), '%Y-%m-%d %H:%i:%s')").Scan(&value)
	return value, err
}

func (r *QueueRepository) ServerDateTimeTx(ctx context.Context, tx *sql.Tx) (string, error) {
	var value string
	err := tx.QueryRowContext(ctx, "SELECT DATE_FORMAT(NOW(), '%Y-%m-%d %H:%i:%s')").Scan(&value)
	return value, err
}

func (r *QueueRepository) AcquireLock(ctx context.Context, db DBTX, name string, timeoutSeconds int) error {
	var result sql.NullInt64
	if err := db.QueryRowContext(ctx, "SELECT GET_LOCK(?, ?)", name, timeoutSeconds).Scan(&result); err != nil {
		return err
	}
	if !result.Valid || result.Int64 != 1 {
		return fmt.Errorf("queue lock is busy")
	}
	return nil
}

func (r *QueueRepository) ReleaseLock(ctx context.Context, db DBTX, name string) {
	var result sql.NullInt64
	_ = db.QueryRowContext(ctx, "SELECT RELEASE_LOCK(?)", name).Scan(&result)
}

func (r *QueueRepository) CurrentCounters(ctx context.Context, tx *sql.Tx, date, poliName string) (int, int, error) {
	query := `
SELECT
	COALESCE((
		SELECT CAST(MAX(NoUrut) AS UNSIGNED)
		FROM finger_unit
		WHERE UnitID = 'Pendaftaran'
			AND SUBSTRING(TanggalFingger, 1, 10) = ?
	), 0),
	COALESCE((
		SELECT CAST(MAX(NoUrut) AS UNSIGNED)
		FROM finger_unit
		WHERE UnitID = ?
			AND SUBSTRING(TanggalFingger, 1, 10) = ?
	), 0)`

	var global, poli int
	err := tx.QueryRowContext(ctx, query, date, poliName, date).Scan(&global, &poli)
	return global, poli, err
}

func (r *QueueRepository) WaitingAhead(ctx context.Context, tx *sql.Tx, date, poliName string, number int) (int, error) {
	query := `
SELECT COUNT(*)
FROM finger_unit
WHERE UnitID = ?
	AND SUBSTRING(TanggalFingger, 1, 10) = ?
	AND Status = 'Belum'
	AND CAST(NoUrut AS UNSIGNED) < ?`

	var count int
	err := tx.QueryRowContext(ctx, query, poliName, date, number).Scan(&count)
	return count, err
}

func (r *QueueRepository) CountDuplicateVisits(ctx context.Context, tx *sql.Tx, date, patientID, unitID string, isBPJS bool) (int, error) {
	query := `
SELECT COUNT(*)
FROM visits v
LEFT JOIN typepatients t ON v.typepatient_id = t.id
WHERE v.tanggal = ?
	AND v.patient_id = ?
	AND v.unit_id = ?
	AND COALESCE(t.bpjs, 0) = ?`

	var count int
	bpjs := 0
	if isBPJS {
		bpjs = 1
	}
	err := tx.QueryRowContext(ctx, query, date, patientID, unitID, bpjs).Scan(&count)
	return count, err
}

func (r *QueueRepository) CountDuplicateQueue(ctx context.Context, tx *sql.Tx, date, patientID, poliName string) (int, error) {
	query := `
SELECT COUNT(*)
FROM finger_unit
WHERE NoPasien = ?
	AND SUBSTRING(TanggalFingger, 1, 10) = ?
	AND Poli = ?
	AND UnitID = 'Pendaftaran'`

	var count int
	err := tx.QueryRowContext(ctx, query, patientID, date, poliName).Scan(&count)
	return count, err
}

func (r *QueueRepository) FindExistingQueue(ctx context.Context, tx *sql.Tx, date, patientID, poliName string) (model.Queue, bool, error) {
	query := `
SELECT
	Kode,
	COALESCE(CAST(NoUrut AS UNSIGNED), 0),
	COALESCE(Poli, ''),
	COALESCE(namadoktermjkn, ''),
	COALESCE(kodedoktermjkn, ''),
	COALESCE(NULLIF(sisakuota, ''), '0')
FROM finger_unit
WHERE NoPasien = ?
	AND SUBSTRING(TanggalFingger, 1, 10) = ?
	AND Poli = ?
	AND UnitID = 'Pendaftaran'
LIMIT 1`

	var (
		queue     model.Queue
		global    int
		remaining string
	)
	err := tx.QueryRowContext(ctx, query, patientID, date, poliName).Scan(
		&queue.Code,
		&global,
		&queue.Poli,
		&queue.DoctorName,
		&queue.DoctorCode,
		&remaining,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Queue{}, false, nil
	}
	if err != nil {
		return model.Queue{}, false, err
	}
	queue.GlobalNumber = global
	queue.Number = parseQueueNumber(queue.Code)
	queue.Date = date
	queue.RemainingQuota, _ = strconv.Atoi(strings.TrimSpace(remaining))
	return queue, true, nil
}

func NextQueueCode(prefix string, number int) string {
	return fmt.Sprintf("%s%03d", prefix, number)
}

func parseQueueNumber(code string) int {
	index := len(code)
	for index > 0 {
		if code[index-1] < '0' || code[index-1] > '9' {
			break
		}
		index--
	}
	number, err := strconv.Atoi(code[index:])
	if err != nil {
		return 0
	}
	return number
}

func (r *QueueRepository) LatestVisitID(ctx context.Context, tx *sql.Tx, patientID, date string) (int64, error) {
	var id int64
	err := tx.QueryRowContext(ctx, `
SELECT id
FROM visits
WHERE patient_id = ?
	AND tanggal = ?
ORDER BY id DESC
LIMIT 1`, patientID, date).Scan(&id)
	return id, err
}

func (r *QueueRepository) InsertFingerReport(ctx context.Context, tx *sql.Tx, code, patientID, bpjsCode string) error {
	_, err := tx.ExecContext(ctx, `
INSERT INTO finger_laporan (nama, kode, tanggal, idpasien, test, poli_bpjs)
VALUES ('Daftar', ?, NOW(), ?, 'OnSite', ?)`, code, patientID, bpjsCode)
	return err
}

func (r *QueueRepository) InsertEmptyFingerReport(ctx context.Context, tx *sql.Tx, code, queueTime, bpjsCode string) error {
	_, err := tx.ExecContext(ctx, `
INSERT INTO finger_laporan (nama, kode, tanggal, idpasien, test, poli_bpjs)
VALUES ('Daftar', ?, ?, NULL, 'OnSite', ?)`, code, queueTime, bpjsCode)
	return err
}

func (r *QueueRepository) CompensateQueueOnly(ctx context.Context, db DBTX, code, queueTime string) error {
	if _, err := db.ExecContext(ctx, `
DELETE FROM finger_laporan
WHERE kode = ?
	AND tanggal = ?
	AND idpasien IS NULL`, code, queueTime); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `
DELETE FROM finger_unit
WHERE Kode = ?
	AND TanggalFingger = ?`, code, queueTime); err != nil {
		return err
	}
	return nil
}

func (r *QueueRepository) CallSPapmsimpus(ctx context.Context, tx *sql.Tx, args ...any) error {
	const query = `CALL SPapmsimpus(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	return drainRows(rows)
}

func (r *QueueRepository) CallSPinsertFingerUnit(ctx context.Context, tx *sql.Tx, args ...any) error {
	const query = `CALL SPinsertFingerUnit(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	return drainRows(rows)
}

func drainRows(rows *sql.Rows) error {
	defer rows.Close()
	for {
		for rows.Next() {
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if !rows.NextResultSet() {
			break
		}
	}
	return rows.Err()
}

func (r *QueueRepository) Compensate(ctx context.Context, tx *sql.Tx, patientID, queueTime, code string, visitID int64) error {
	if visitID > 0 {
		if _, err := tx.ExecContext(ctx, "DELETE FROM bpjs_visits WHERE visit_id = ?", visitID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM bpjs_registrations WHERE visit_id = ?", visitID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM expenses WHERE visit_id = ?", visitID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM histories WHERE visit_id = ?", visitID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM visits WHERE id = ?", visitID); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `
DELETE FROM finger_unit
WHERE NoPasien = ?
	AND TanggalFingger = ?`, patientID, queueTime); err != nil {
		return err
	}
	if code != "" {
		if _, err := tx.ExecContext(ctx, `
DELETE FROM finger_laporan
WHERE kode = ?
	AND idpasien = ?`, code, patientID); err != nil {
			return err
		}
	}
	return nil
}

func NormalizeCapacity(value string) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	number, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return number
}
