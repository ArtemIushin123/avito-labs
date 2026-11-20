package trip

import (
	"context"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArtemIushin123/template/internal/infra/postgres/transaction"
)

type Repository struct {
	pool         *pgxpool.Pool
	builder      squirrel.StatementBuilderType
	queryTimeout time.Duration
}

func New(pool *pgxpool.Pool, queryTimeout time.Duration) *Repository {
	return &Repository{
		pool:         pool,
		builder:      squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar),
		queryTimeout: queryTimeout,
	}
}

func (r *Repository) getExecutor(ctx context.Context) executor {
	tx, ok := transaction.CheckTransaction(ctx)
	if !ok {
		return r.pool
	}
	return tx
}
