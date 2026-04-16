package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	Port               string
	BaseURL            string
	JWTSecret          string
	JWTExpiration      time.Duration
	GoogleClientID     string
	GoogleClientSecret string
	GoogleRedirectURL  string
	GitHubClientID     string
	GitHubClientSecret string
	GitHubRedirectURL  string
	FrontendURL        string
	MaxLoginAttempts   int
	LockoutDuration    time.Duration
	RateLimitRequests  int
	RateLimitWindow    time.Duration

	DatabaseHost     string
	DatabasePort     string
	DatabaseUser     string
	DatabasePassword string
	DatabaseName     string
	DatabaseSSLMode  string

	RedisURL string

	OllamaURL   string
	OllamaModel string

	PaddleAPIKey        string
	PaddleWebhookSecret string

	GitHubWebhookSecret string

	MetricsRetentionDays int
	ResendAPIKey         string
	EmailFrom            string
	MaxRepoSizeMB        int
}

func Load() *Config {
	paddleWebhookSecret := getEnv("PADDLE_WEBHOOK_SECRET", "")
	metricsRetentionDays := getEnvInt("METRICS_RETENTION_DAYS", 7)
	resendAPIKey := getEnv("RESEND_API_KEY", "")
	emailFrom := getEnv("EMAIL_FROM", "onboarding@debtdrone.dev")

	return &Config{
		Port:               getEnv("PORT", "8080"),
		BaseURL:            getEnv("BASE_URL", "http://localhost:8080"),
		JWTSecret:          getEnv("JWT_SECRET", "your-secret-key-change-in-production"),
		JWTExpiration:      getDuration("JWT_EXPIRATION", 8*time.Hour),
		GoogleClientID:     getEnv("GOOGLE_CLIENT_ID", ""),
		GoogleClientSecret: getEnv("GOOGLE_CLIENT_SECRET", ""),
		GoogleRedirectURL:  getEnv("GOOGLE_REDIRECT_URL", "http://localhost:8080/api/auth/google/callback"),
		GitHubClientID:     getEnv("GITHUB_CLIENT_ID", ""),
		GitHubClientSecret: getEnv("GITHUB_CLIENT_SECRET", ""),
		GitHubRedirectURL:  getEnv("GITHUB_REDIRECT_URL", "http://localhost:8080/api/auth/github/callback"),
		FrontendURL:        getEnv("FRONTEND_URL", "http://localhost:3000"),
		MaxLoginAttempts:   getEnvInt("MAX_LOGIN_ATTEMPTS", 5),
		LockoutDuration:    getDuration("LOCKOUT_DURATION", 15*time.Minute),
		RateLimitRequests:  getEnvInt("RATE_LIMIT_REQUESTS", 300),
		RateLimitWindow:    getDuration("RATE_LIMIT_WINDOW", 1*time.Minute),

		DatabaseHost:     getEnv("DB_HOST", "localhost"),
		DatabasePort:     getEnv("DB_PORT", "5432"),
		DatabaseUser:     getEnv("DB_USER", "debtdrone"),
		DatabasePassword: getEnv("DB_PASSWORD", "debtdrone_password"),
		DatabaseName:     getEnv("DB_NAME", "debtdrone"),
		DatabaseSSLMode:  getEnv("DB_SSLMODE", "disable"),

		RedisURL: getEnv("REDIS_URL", ""),

		OllamaURL:            getEnv("OLLAMA_URL", "http://localhost:11434"),
		OllamaModel:          getEnv("OLLAMA_MODEL", "llama3.2"),
		PaddleAPIKey:         getEnv("PADDLE_API_KEY", ""),
		PaddleWebhookSecret:  paddleWebhookSecret,
		GitHubWebhookSecret:  getEnv("GITHUB_WEBHOOK_SECRET", ""),
		MetricsRetentionDays: metricsRetentionDays,

		ResendAPIKey:  resendAPIKey,
		EmailFrom:     emailFrom,
		MaxRepoSizeMB: getEnvInt("MAX_REPO_SIZE_MB", 500),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}

func (c *Config) ShutdownTimeout() time.Duration {
	return getDuration("SHUTDOWN_TIMEOUT", 30*time.Second)
}
