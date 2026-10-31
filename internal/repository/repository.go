package repository

import (
	"context"
	"database/sql"
	"errors"
)

var ErrNotFound = errors.New("data not found")

type DBTX interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}
