package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/ArtemIushin123/template/internal/config"
	"github.com/ArtemIushin123/template/internal/container"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cont, err := container.New(ctx, cfg)
	if err != nil {
		return fmt.Errorf("container: %w", err)
	}
	logger := cont.Logger

	chanErr := make(chan error, 1)
	go func() {
		chanErr <- cont.Server.ListenAndServe()
	}()
	logger.InfoContext(ctx, "service started", "addr", cfg.HTTPAddr)

	select {
	case <-ctx.Done():
		stop()
		logger.Info("shutdown signal received")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()

		if err := cont.Shutdown(shutdownCtx); err != nil {
			logger.Error("forced shutdown", "error", err)
			return fmt.Errorf("shutdown: %w", err)
		}
		logger.Info("service stopped")
		return nil

	case err := <-chanErr:
		logger.Error("server stopped unexpectedly", "error", err)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		return errors.Join(fmt.Errorf("server stopped unexpectedly: %w", err), cont.Shutdown(shutdownCtx))
	}
}
