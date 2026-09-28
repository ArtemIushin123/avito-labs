package config

import (
	"fmt"
	"time"

	"github.com/ilyakaznacheev/cleanenv"
)

type Config struct {
	HTTPAddr                string        `env:"HTTP_ADDR" env-required:"true"`
	HTTPReadTimeout         time.Duration `env:"HTTP_READ_TIMEOUT" env-required:"true"`
	HTTPReadHeaderTimeout   time.Duration `env:"HTTP_READ_HEADER_TIMEOUT" env-required:"true"`
	HTTPWriteTimeout        time.Duration `env:"HTTP_WRITE_TIMEOUT" env-required:"true"`
	HTTPIdleTimeout         time.Duration `env:"HTTP_IDLE_TIMEOUT" env-required:"true"`
	LogLevel                string        `env:"LOG_LEVEL" env-required:"true"`
	ShutdownTimeout         time.Duration `env:"SHUTDOWN_TIMEOUT" env-required:"true"`
	DatabaseURL             string        `env:"DATABASE_URL" env-required:"true"`
	DatabaseMaxConns        int32         `env:"DATABASE_MAX_CONNS" env-required:"true"`
	DatabaseMinConns        int32         `env:"DATABASE_MIN_CONNS" env-required:"true"`
	DatabaseMaxConnLifetime time.Duration `env:"DATABASE_MAX_CONN_LIFETIME" env-required:"true"`
	DatabaseConnectTimeout  time.Duration `env:"DATABASE_CONNECT_TIMEOUT" env-required:"true"`
	DatabaseQueryTimeout    time.Duration `env:"DATABASE_QUERY_TIMEOUT" env-required:"true"`
}

func Load() (*Config, error) {
	var config Config
	if err := cleanenv.ReadEnv(&config); err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}
	return &config, nil
}
