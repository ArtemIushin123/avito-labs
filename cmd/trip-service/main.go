package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/ArtemIushin123/template/internal/config"
	"github.com/ArtemIushin123/template/internal/repository"
	httpserver "github.com/ArtemIushin123/template/internal/server"
	"github.com/ArtemIushin123/template/internal/transaction"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Println("config load error:", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.DatabaseConnectTimeout)
	defer cancel()

	poolConfig, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		fmt.Println("config parse error", err)
		os.Exit(1)
	}

	poolConfig.MaxConns = cfg.DatabaseMaxConns
	poolConfig.MinConns = cfg.DatabaseMinConns
	poolConfig.MaxConnLifetime = cfg.DatabaseMaxConnLifetime
	poolConfig.ConnConfig.ConnectTimeout = cfg.DatabaseConnectTimeout

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		fmt.Println("create pool error ", err)
		os.Exit(1)
	}

	if err := pool.Ping(ctx); err != nil {
		fmt.Println("pool unavailable", err)
		os.Exit(1)
	}

	manager := transaction.NewPgxTxManager(pool)
	repo := repository.NewTripRepository(pool)
	handler := httpserver.NewHandler(pool, cfg.DatabaseConnectTimeout, cfg.DatabaseQueryTimeout, repo, manager)
	server := httpserver.NewServer(handler, cfg)

	chanErr := make(chan error, 1)
	go func() {
		chanErr <- server.ListenAndServe()
	}()

	chanSign := make(chan os.Signal, 1)
	signal.Notify(chanSign, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-chanSign:
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer shutdownCancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			fmt.Println("graceful shutdown timed out, forcing close:", err)
			_ = server.Close()
		}

		if !closePool(shutdownCtx, pool) {
			fmt.Println("pool doesn't close for SHUTDOWN_TIMEOUT")
			os.Exit(1)
		}

	case err := <-chanErr:
		fmt.Println("not regular server stop", err)
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer shutdownCancel()

		if !closePool(shutdownCtx, pool) {
			fmt.Println("pool doesn't close for SHUTDOWN_TIMEOUT")
		}
		os.Exit(1)
	}
}

func closePool(ctx context.Context, pool *pgxpool.Pool) bool {
	chanPool := make(chan struct{})
	go func() {
		pool.Close()
		close(chanPool)
	}()

	select {
	case <-ctx.Done():
		return false
	case <-chanPool:
		return true
	}
}
