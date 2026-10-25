package tata

import (
	"context"
	"database/sql"
)

type tagResult struct{ sql.Result }

func (t tagResult) RowsAffected() int64 {
	if t.Result == nil {
		return 0
	}
	n, err := t.Result.RowsAffected()
	if err != nil {
		return 0
	}
	return n
}

func (s *Store) QueryRow(ctx context.Context, query string, args ...any) *sql.Row {
	return s.DB.QueryRowContext(ctx, query, args...)
}

func (s *Store) Query(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return s.DB.QueryContext(ctx, query, args...)
}

func (s *Store) Exec(ctx context.Context, query string, args ...any) (tagResult, error) {
	res, err := s.DB.ExecContext(ctx, query, args...)
	return tagResult{res}, err
}

type Tx struct{ *sql.Tx }

func (s *Store) Begin(ctx context.Context) (Tx, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	return Tx{tx}, err
}

func (t Tx) QueryRow(ctx context.Context, query string, args ...any) *sql.Row {
	return t.Tx.QueryRowContext(ctx, query, args...)
}

func (t Tx) Exec(ctx context.Context, query string, args ...any) (tagResult, error) {
	res, err := t.Tx.ExecContext(ctx, query, args...)
	return tagResult{res}, err
}

func (t Tx) Commit(ctx context.Context) error { return t.Tx.Commit() }

func (t Tx) Rollback(ctx context.Context) error { return t.Tx.Rollback() }
