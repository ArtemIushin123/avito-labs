package transaction

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Manager struct {
	pool               *pgxpool.Pool
	transactionTimeout time.Duration
}

type txKeyType struct{}

var txKey = txKeyType{}

func New(pool *pgxpool.Pool, transactionTimeout time.Duration) *Manager {
	return &Manager{pool: pool, transactionTimeout: transactionTimeout}
}

func (m *Manager) Do(ctx context.Context, fn func(context.Context) error) error {
	if _, ok := CheckTransaction(ctx); ok {
		return fn(ctx)
	}

	ctx, cancel := context.WithTimeout(ctx, m.transactionTimeout)
	defer cancel()

	tx, err := m.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := fn(context.WithValue(ctx, txKey, tx)); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

func CheckTransaction(ctx context.Context) (pgx.Tx, bool) {
	tx, ok := ctx.Value(txKey).(pgx.Tx)
	return tx, ok
}
