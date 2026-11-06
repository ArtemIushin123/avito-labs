package container

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"

	"github.com/ArtemIushin123/template/internal/config"
	"github.com/ArtemIushin123/template/internal/http/health"
	"github.com/ArtemIushin123/template/internal/http/middleware"
	"github.com/ArtemIushin123/template/internal/http/router"
	hcreate "github.com/ArtemIushin123/template/internal/http/trip/create"
	hfinish "github.com/ArtemIushin123/template/internal/http/trip/finish"
	hget "github.com/ArtemIushin123/template/internal/http/trip/get"
	"github.com/ArtemIushin123/template/internal/infra/postgres"
	"github.com/ArtemIushin123/template/internal/infra/postgres/transaction"
	"github.com/ArtemIushin123/template/internal/infra/telemetry"
	triprepo "github.com/ArtemIushin123/template/internal/repository/trip"
	ucreate "github.com/ArtemIushin123/template/internal/usecase/trip/create"
	ufinish "github.com/ArtemIushin123/template/internal/usecase/trip/finish"
	uget "github.com/ArtemIushin123/template/internal/usecase/trip/get"
)

var version = "dev"

const instrumentationName = "github.com/ArtemIushin123/template"

type Container struct {
	Server *http.Server
	Logger *slog.Logger

	db                *pgxpool.Pool
	shutdownTelemetry func(context.Context) error
}

func New(ctx context.Context, cfg *config.Config) (*Container, error) {
	logger, err := telemetry.NewLogger(cfg.LogLevel, cfg.OtelServiceName)
	if err != nil {
		return nil, fmt.Errorf("create logger: %w", err)
	}

	shutdownTelemetry, err := telemetry.Setup(ctx, logger, version)
	if err != nil {
		return nil, fmt.Errorf("setup telemetry: %w", err)
	}

	c, err := build(ctx, cfg, logger)
	if err != nil {
		_ = shutdownTelemetry(ctx)
		return nil, err
	}
	c.shutdownTelemetry = shutdownTelemetry

	return c, nil
}

func build(ctx context.Context, cfg *config.Config, logger *slog.Logger) (*Container, error) {
	pool, err := postgres.NewPool(ctx, postgres.Config{
		URL:             cfg.DatabaseURL,
		MaxConns:        cfg.DatabaseMaxConns,
		MinConns:        cfg.DatabaseMinConns,
		MaxConnLifetime: cfg.DatabaseMaxConnLifetime,
		ConnectTimeout:  cfg.DatabaseConnectTimeout,
	})
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	meter := otel.Meter(instrumentationName)

	events, err := telemetry.NewTripEvents(logger, meter)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("create trip events: %w", err)
	}
	httpMetrics, err := middleware.NewMetrics(meter)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("create http metrics: %w", err)
	}

	manager := transaction.New(pool)
	repo := triprepo.New(pool, cfg.DatabaseQueryTimeout)

	createUsecase := ucreate.New(repo, manager, events, now, uuid.New)
	finishUsecase := ufinish.New(repo, manager, events, now)
	getUsecase := uget.New(repo)

	handler := router.New(
		hcreate.New(createUsecase),
		hget.New(getUsecase),
		hfinish.New(finishUsecase),
		health.New(pool, cfg.DatabaseConnectTimeout),
		logger,
		httpMetrics,
	)

	return &Container{
		Server: &http.Server{
			Addr:              cfg.HTTPAddr,
			Handler:           handler,
			ReadTimeout:       cfg.HTTPReadTimeout,
			ReadHeaderTimeout: cfg.HTTPReadHeaderTimeout,
			WriteTimeout:      cfg.HTTPWriteTimeout,
			IdleTimeout:       cfg.HTTPIdleTimeout,
		},
		Logger: logger,
		db:     pool,
	}, nil
}

func (c *Container) Shutdown(ctx context.Context) error {
	var shutdownErrors []error

	if err := c.Server.Shutdown(ctx); err != nil {
		shutdownErrors = append(shutdownErrors, fmt.Errorf("graceful shutdown timed out, forcing close: %w", err))
		if closeErr := c.Server.Close(); closeErr != nil {
			shutdownErrors = append(shutdownErrors, closeErr)
		}
	}

	if err := closePool(ctx, c.db); err != nil {
		shutdownErrors = append(shutdownErrors, err)
	}

	if err := c.shutdownTelemetry(ctx); err != nil {
		c.Logger.WarnContext(ctx, "telemetry shutdown failed", "error", err)
	}

	return errors.Join(shutdownErrors...)
}

func now() time.Time {
	return time.Now().UTC()
}

func closePool(ctx context.Context, pool *pgxpool.Pool) error {
	done := make(chan struct{})
	go func() {
		pool.Close()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("close pool: %w", ctx.Err())
	}
}
