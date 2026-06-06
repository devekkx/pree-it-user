package config

import (
	"fmt"
	"os"

	"github.com/devekkx/pree-it-user/pkg/secrets"
)

type Config struct {
	// Service
	ServiceName string
	ServicePort string
	Environment string

	// Database
	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string
	DBSSLMode  string

	// NATS
	NATSUrl string

	// OTel
	OTelEndpoint string
}

func Load() (*Config, error) {
	dbPassword, err := secrets.Read("user_db_password")
	if err != nil {
		return nil, fmt.Errorf("loading db password: %w", err)
	}

	cfg := &Config{
		ServiceName: getEnv("USER_SERVICE_NAME", "user-service"),
		ServicePort: getEnv("USER_SERVICE_PORT", "8083"),
		Environment: getEnv("USER_ENVIRONMENT", "production"),

		DBHost:     getEnv("USER_DB_HOST", "postgres"),
		DBPort:     getEnv("USER_DB_PORT", "5432"),
		DBUser:     getEnv("USER_DB_USER", "user_service"),
		DBPassword: dbPassword,
		DBName:     getEnv("USER_DB_NAME", "preeit"),
		DBSSLMode:  getEnv("USER_DB_SSLMODE", "disable"),

		NATSUrl: getEnv("USER_NATS_URL", "nats://nats:4222"),

		OTelEndpoint: getEnv("USER_OTEL_ENDPOINT", "otel-collector:4317"),
	}

	return cfg, nil
}

func (c *Config) DatabaseDSN() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=%s&search_path=user_schema",
		c.DBUser, c.DBPassword, c.DBHost, c.DBPort, c.DBName, c.DBSSLMode,
	)
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
