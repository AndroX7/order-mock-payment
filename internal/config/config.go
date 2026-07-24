// Package config loads and validates all runtime configuration.
package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	App      App
	Postgres Postgres
	Redis    Redis
	JWT      JWT
}

type App struct {
	Env             string
	HTTPPort        int
	LogLevel        string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	ShutdownTimeout time.Duration
}

type Postgres struct {
	Host            string
	Port            int
	User            string
	Password        string
	DB              string
	SSLMode         string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
}

// DSN returns a URL-form connection string. url.UserPassword handles
// percent-encoding, so passwords with '@', ' ', or other special characters
// are safe.
func (p Postgres) DSN() string {
	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(p.User, p.Password),
		Host:   net.JoinHostPort(p.Host, strconv.Itoa(p.Port)),
		Path:   p.DB,
	}
	q := u.Query()
	q.Set("sslmode", p.SSLMode)
	u.RawQuery = q.Encode()
	return u.String()
}

type Redis struct {
	Addr        string
	Password    string
	DB          int
	DialTimeout time.Duration
}

type JWT struct {
	Secret string
	TTL    time.Duration
}

// Load reads configuration from environment variables (and .env if present),
// applies defaults, and validates.
func Load() (*Config, error) {
	v := viper.New()
	v.AutomaticEnv()
	setDefaults(v)

	// .env is optional: env vars take over in production.
	if _, err := os.Stat(".env"); err == nil {
		v.SetConfigFile(".env")
		v.SetConfigType("env")
		if err := v.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("read .env: %w", err)
		}
	}

	cfg := &Config{
		App: App{
			Env:             v.GetString("APP_ENV"),
			HTTPPort:        v.GetInt("HTTP_PORT"),
			LogLevel:        v.GetString("LOG_LEVEL"),
			ReadTimeout:     v.GetDuration("HTTP_READ_TIMEOUT"),
			WriteTimeout:    v.GetDuration("HTTP_WRITE_TIMEOUT"),
			IdleTimeout:     v.GetDuration("HTTP_IDLE_TIMEOUT"),
			ShutdownTimeout: v.GetDuration("HTTP_SHUTDOWN_TIMEOUT"),
		},
		Postgres: Postgres{
			Host:            v.GetString("POSTGRES_HOST"),
			Port:            v.GetInt("POSTGRES_PORT"),
			User:            v.GetString("POSTGRES_USER"),
			Password:        v.GetString("POSTGRES_PASSWORD"),
			DB:              v.GetString("POSTGRES_DB"),
			SSLMode:         v.GetString("POSTGRES_SSLMODE"),
			MaxOpenConns:    v.GetInt("POSTGRES_MAX_OPEN_CONNS"),
			MaxIdleConns:    v.GetInt("POSTGRES_MAX_IDLE_CONNS"),
			ConnMaxLifetime: v.GetDuration("POSTGRES_CONN_MAX_LIFETIME"),
			ConnMaxIdleTime: v.GetDuration("POSTGRES_CONN_MAX_IDLE_TIME"),
		},
		Redis: Redis{
			Addr:        v.GetString("REDIS_ADDR"),
			Password:    v.GetString("REDIS_PASSWORD"),
			DB:          v.GetInt("REDIS_DB"),
			DialTimeout: v.GetDuration("REDIS_DIAL_TIMEOUT"),
		},
		JWT: JWT{
			Secret: v.GetString("JWT_SECRET"),
			TTL:    v.GetDuration("JWT_TTL"),
		},
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("config validation: %w", err)
	}
	return cfg, nil
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("APP_ENV", "development")
	v.SetDefault("HTTP_PORT", 8080)
	v.SetDefault("LOG_LEVEL", "info")
	v.SetDefault("HTTP_READ_TIMEOUT", 10*time.Second)
	v.SetDefault("HTTP_WRITE_TIMEOUT", 15*time.Second)
	v.SetDefault("HTTP_IDLE_TIMEOUT", 60*time.Second)
	v.SetDefault("HTTP_SHUTDOWN_TIMEOUT", 30*time.Second)

	v.SetDefault("POSTGRES_HOST", "localhost")
	v.SetDefault("POSTGRES_PORT", 5432)
	v.SetDefault("POSTGRES_SSLMODE", "disable")
	v.SetDefault("POSTGRES_MAX_OPEN_CONNS", 25)
	v.SetDefault("POSTGRES_MAX_IDLE_CONNS", 5)
	v.SetDefault("POSTGRES_CONN_MAX_LIFETIME", 5*time.Minute)
	v.SetDefault("POSTGRES_CONN_MAX_IDLE_TIME", 5*time.Minute)

	v.SetDefault("REDIS_ADDR", "localhost:6379")
	v.SetDefault("REDIS_DB", 0)
	v.SetDefault("REDIS_DIAL_TIMEOUT", 5*time.Second)

	// JWT_SECRET has no default: operators must provide one. Validation rejects
	// secrets shorter than 32 bytes.
	v.SetDefault("JWT_TTL", 24*time.Hour)
}

func (c *Config) validate() error {
	switch c.App.Env {
	case "development", "production":
	default:
		return fmt.Errorf("invalid APP_ENV: %q (must be \"development\" or \"production\")", c.App.Env)
	}
	if c.App.HTTPPort <= 0 || c.App.HTTPPort > 65535 {
		return fmt.Errorf("invalid HTTP_PORT: %d", c.App.HTTPPort)
	}
	if c.Postgres.Host == "" {
		return errors.New("POSTGRES_HOST is required")
	}
	if c.Postgres.User == "" {
		return errors.New("POSTGRES_USER is required")
	}
	if c.Postgres.DB == "" {
		return errors.New("POSTGRES_DB is required")
	}
	if c.Postgres.MaxOpenConns <= 0 {
		return fmt.Errorf("POSTGRES_MAX_OPEN_CONNS must be > 0, got %d", c.Postgres.MaxOpenConns)
	}
	if c.Postgres.MaxIdleConns > c.Postgres.MaxOpenConns {
		return fmt.Errorf(
			"POSTGRES_MAX_IDLE_CONNS (%d) must be <= POSTGRES_MAX_OPEN_CONNS (%d)",
			c.Postgres.MaxIdleConns, c.Postgres.MaxOpenConns,
		)
	}
	if c.Redis.Addr == "" {
		return errors.New("REDIS_ADDR is required")
	}
	if len(c.JWT.Secret) < 32 {
		return fmt.Errorf("JWT_SECRET must be at least 32 bytes, got %d", len(c.JWT.Secret))
	}
	if c.JWT.TTL <= 0 {
		return fmt.Errorf("JWT_TTL must be positive, got %s", c.JWT.TTL)
	}
	return nil
}
