package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Transactor struct {
	db *pgxpool.Pool
}

func NewTransactor(db *pgxpool.Pool) *Transactor {
	return &Transactor{
		db: db,
	}
}

func (t *Transactor) WithinTransaction(ctx context.Context, fn func(tx pgx.Tx) error) error {
	tx, err := t.db.Begin(ctx)

	if err != nil {
		return fmt.Errorf(
			"begin transaction: %w",
			err,
		)
	}

	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if err := fn(tx); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}
