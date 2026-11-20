package config

import (
	"fmt"
	"net/url"
	"time"

	"github.com/ilyakaznacheev/cleanenv"
)

type Config struct {
	HTTPAddr              string        `env:"HTTP_ADDR" env-required:"true"`
	HTTPReadTimeout       time.Duration `env:"HTTP_READ_TIMEOUT" env-required:"true"`
	HTTPReadHeaderTimeout time.Duration `env:"HTTP_READ_HEADER_TIMEOUT" env-required:"true"`
	HTTPWriteTimeout      time.Duration `env:"HTTP_WRITE_TIMEOUT" env-required:"true"`
	HTTPIdleTimeout       time.Duration `env:"HTTP_IDLE_TIMEOUT" env-required:"true"`

	LogLevel        string        `env:"LOG_LEVEL" env-required:"true"`
	OtelServiceName string        `env:"OTEL_SERVICE_NAME" env-default:"trip-service"`
	ShutdownTimeout time.Duration `env:"SHUTDOWN_TIMEOUT" env-required:"true"`

	DatabaseURL                string        `env:"DATABASE_URL" env-required:"true"`
	DatabaseMaxConns           int32         `env:"DATABASE_MAX_CONNS" env-required:"true"`
	DatabaseMinConns           int32         `env:"DATABASE_MIN_CONNS" env-required:"true"`
	DatabaseMaxConnLifetime    time.Duration `env:"DATABASE_MAX_CONN_LIFETIME" env-required:"true"`
	DatabaseConnectTimeout     time.Duration `env:"DATABASE_CONNECT_TIMEOUT" env-required:"true"`
	DatabaseQueryTimeout       time.Duration `env:"DATABASE_QUERY_TIMEOUT" env-required:"true"`
	DatabaseTransactionTimeout time.Duration `env:"DATABASE_TRANSACTION_TIMEOUT" env-required:"true"`

	PositionCheckInterval time.Duration `env:"POSITION_CHECK_INTERVAL"`
	PositionStaleAfter    time.Duration `env:"POSITION_STALE_AFTER"`
	PositionWorkers       int           `env:"POSITION_WORKERS"`
	PositionQueueSize     int           `env:"POSITION_QUEUE_SIZE"`
	PushHTTPURL           string        `env:"PUSH_HTTP_URL"`
	PushRequestTimeout    time.Duration `env:"PUSH_REQUEST_TIMEOUT"`
}

func Load() (*Config, error) {
	var config Config

	if err := cleanenv.ReadEnv(&config); err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	if err := validate(&config); err != nil {
		return nil, fmt.Errorf("invalid values in config: %w", err)
	}

	return &config, nil
}

func validate(config *Config) error {
	checks := []struct {
		name  string
		value time.Duration
	}{
		{"HTTP_READ_TIMEOUT", config.HTTPReadTimeout},
		{"HTTP_READ_HEADER_TIMEOUT", config.HTTPReadHeaderTimeout},
		{"HTTP_WRITE_TIMEOUT", config.HTTPWriteTimeout},
		{"HTTP_IDLE_TIMEOUT", config.HTTPIdleTimeout},
		{"SHUTDOWN_TIMEOUT", config.ShutdownTimeout},
		{"DATABASE_MAX_CONN_LIFETIME", config.DatabaseMaxConnLifetime},
		{"DATABASE_CONNECT_TIMEOUT", config.DatabaseConnectTimeout},
		{"DATABASE_QUERY_TIMEOUT", config.DatabaseQueryTimeout},
		{"DATABASE_TRANSACTION_TIMEOUT", config.DatabaseTransactionTimeout},
		{"POSITION_CHECK_INTERVAL", config.PositionCheckInterval},
		{"POSITION_STALE_AFTER", config.PositionStaleAfter},
		{"PUSH_REQUEST_TIMEOUT", config.PushRequestTimeout},
	}

	for _, c := range checks {
		if c.value <= 0 {
			return fmt.Errorf("%s must be > 0, got %s", c.name, c.value)
		}
	}

	if config.DatabaseMaxConns <= 0 {
		return fmt.Errorf("DATABASE_MAX_CONNS must be > 0, got %d", config.DatabaseMaxConns)
	}
	if config.DatabaseMinConns < 0 {
		return fmt.Errorf("DATABASE_MIN_CONNS must be >= 0, got %d", config.DatabaseMinConns)
	}
	if config.DatabaseMinConns > config.DatabaseMaxConns {
		return fmt.Errorf("DATABASE_MIN_CONNS (%d) must be <= DATABASE_MAX_CONNS (%d)",
			config.DatabaseMinConns, config.DatabaseMaxConns)
	}

	if config.HTTPAddr == "" {
		return fmt.Errorf("HTTP_ADDR must not be empty")
	}
	if config.DatabaseURL == "" {
		return fmt.Errorf("DATABASE_URL must not be empty")
	}
	if config.PositionWorkers <= 0 {
		return fmt.Errorf("POSITION_WORKERS must be > 0, got %d", config.PositionWorkers)
	}
	if config.PositionQueueSize <= 0 {
		return fmt.Errorf("POSITION_QUEUE_SIZE must be > 0, got %d", config.PositionQueueSize)
	}
	pushURL, err := url.Parse(config.PushHTTPURL)
	if err != nil || pushURL.Hostname() == "" || (pushURL.Scheme != "http" && pushURL.Scheme != "https") {
		return fmt.Errorf("PUSH_HTTP_URL must be an absolute HTTP or HTTPS URL with a host")
	}
	return nil
}
