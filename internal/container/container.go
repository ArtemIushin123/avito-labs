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
	"github.com/ArtemIushin123/template/internal/gateway/push"
	"github.com/ArtemIushin123/template/internal/http/health"
	"github.com/ArtemIushin123/template/internal/http/middleware"
	hposition "github.com/ArtemIushin123/template/internal/http/position/create"
	hpositionlist "github.com/ArtemIushin123/template/internal/http/position/list"
	"github.com/ArtemIushin123/template/internal/http/router"
	hcreate "github.com/ArtemIushin123/template/internal/http/trip/create"
	hfinish "github.com/ArtemIushin123/template/internal/http/trip/finish"
	hget "github.com/ArtemIushin123/template/internal/http/trip/get"
	"github.com/ArtemIushin123/template/internal/infra/postgres"
	"github.com/ArtemIushin123/template/internal/infra/postgres/transaction"
	"github.com/ArtemIushin123/template/internal/infra/telemetry"
	triprepo "github.com/ArtemIushin123/template/internal/repository/trip"
	uposition "github.com/ArtemIushin123/template/internal/usecase/position/create"
	upositionlist "github.com/ArtemIushin123/template/internal/usecase/position/list"
	ucreate "github.com/ArtemIushin123/template/internal/usecase/trip/create"
	ufinish "github.com/ArtemIushin123/template/internal/usecase/trip/finish"
	uget "github.com/ArtemIushin123/template/internal/usecase/trip/get"
	"github.com/ArtemIushin123/template/internal/worker/positions"
)

var version = "dev"

const instrumentationName = "github.com/ArtemIushin123/template"

type Container struct {
	Server *http.Server
	Logger *slog.Logger

	pushClient            *push.Client
	positionWorkers       *positions.Pool
	positionScanner       *positions.Scanner
	db                    *pgxpool.Pool
	stopActiveTripsMetric func()
	shutdownTelemetry     func(context.Context) error
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

	positionRequests, err := telemetry.NewPositionRequests(logger, meter)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("create position request metrics: %w", err)
	}
	pushClient, err := push.New(cfg.PushHTTPURL, cfg.PushRequestTimeout, positionRequests)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("create push client: %w", err)
	}

	manager := transaction.New(pool, cfg.DatabaseTransactionTimeout)
	repo := triprepo.New(pool, cfg.DatabaseQueryTimeout)

	createUsecase := ucreate.New(repo, manager, events, now, uuid.New)
	finishUsecase := ufinish.New(repo, manager, events, now)
	getUsecase := uget.New(repo)
	positionQueue := positions.NewQueue(cfg.PositionQueueSize, positionRequests)
	positionUsecase := uposition.New(repo, manager, events, positionQueue, now)

	handler := router.New(
		hcreate.New(createUsecase),
		hget.New(getUsecase),
		hfinish.New(finishUsecase),
		hposition.New(positionUsecase),
		hpositionlist.New(upositionlist.New(repo)),
		health.New(pool, cfg.DatabaseConnectTimeout),
		logger,
		httpMetrics,
	)

	stopActiveTripsMetric, err := telemetry.ObserveActiveTrips(meter, repo)
	if err != nil {
		pool.Close()
		return nil, err
	}
	backgroundCtx := context.WithoutCancel(ctx)
	tracer := otel.Tracer(instrumentationName)
	positionWorkers := positions.StartPool(backgroundCtx, positionQueue, pushClient, positionRequests,
		logger, tracer, cfg.PositionWorkers, cfg.PushRequestTimeout)
	positionScanner := positions.StartScanner(backgroundCtx, positionQueue, repo, logger,
		tracer, cfg.PositionCheckInterval, cfg.PositionStaleAfter)

	return &Container{
		Server: &http.Server{
			Addr:              cfg.HTTPAddr,
			Handler:           handler,
			ReadTimeout:       cfg.HTTPReadTimeout,
			ReadHeaderTimeout: cfg.HTTPReadHeaderTimeout,
			WriteTimeout:      cfg.HTTPWriteTimeout,
			IdleTimeout:       cfg.HTTPIdleTimeout,
		},
		Logger:                logger,
		db:                    pool,
		pushClient:            pushClient,
		positionWorkers:       positionWorkers,
		positionScanner:       positionScanner,
		stopActiveTripsMetric: stopActiveTripsMetric,
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

	if err := c.positionScanner.Shutdown(ctx); err != nil {
		shutdownErrors = append(shutdownErrors, fmt.Errorf("stop position scanner: %w", err))
	}
	if err := c.positionWorkers.Shutdown(ctx); err != nil {
		shutdownErrors = append(shutdownErrors, fmt.Errorf("stop position workers: %w", err))
	}
	c.pushClient.CloseIdleConnections()
	c.stopActiveTripsMetric()
	if err := closePool(ctx, c.db); err != nil {
		shutdownErrors = append(shutdownErrors, err)
	}

	if err := c.shutdownTelemetry(ctx); err != nil {
		shutdownErrors = append(shutdownErrors, fmt.Errorf("stop telemetry: %w", err))
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
