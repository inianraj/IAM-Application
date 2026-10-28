package config

import (
	"bufio"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr          string
	DatabaseURL       string
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
	RedisAddr         string
	RedisPassword     string
	RedisDB           int
	CacheTTL          time.Duration
	RabbitMQURL       string
	SMTPAddr          string
	MailFrom          string
	SessionTTL        time.Duration
	CookieSecure      bool
	AllowedOrigins    []string
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// loadDotEnv sets variables from a .env file without overriding existing ones.
func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		if os.Getenv(k) == "" {
			os.Setenv(k, strings.Trim(strings.TrimSpace(v), `"'`))
		}
	}
}

func Load() Config {
	loadDotEnv(".env")

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
			url.QueryEscape(getenv("DB_USER", "postgres")),
			url.QueryEscape(os.Getenv("DB_PASSWORD")),
			getenv("DB_HOST", "localhost"),
			getenv("DB_PORT", "5432"),
			getenv("DB_NAME", "postgres"),
		)
	}

	return Config{
		HTTPAddr:          getenv("HTTP_ADDR", ":8080"),
		DatabaseURL:       dsn,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		ShutdownTimeout:   10 * time.Second,
		// An empty RedisAddr disables the cache.
		RedisAddr:     os.Getenv("REDIS_ADDR"),
		RedisPassword: os.Getenv("REDIS_PASSWORD"),
		RedisDB:       atoi(os.Getenv("REDIS_DB"), 0),
		CacheTTL:      time.Duration(atoi(os.Getenv("CACHE_TTL_SECONDS"), 60)) * time.Second,
		// An empty RabbitMQURL disables event publishing in the API.
		RabbitMQURL: os.Getenv("RABBITMQ_URL"),
		SMTPAddr:    getenv("SMTP_ADDR", "localhost:1025"),
		MailFrom:    getenv("MAIL_FROM", "no-reply@crud.local"),
		// Sessions are absolute: they end SessionTTL after login.
		SessionTTL: time.Duration(atoi(os.Getenv("SESSION_TTL_HOURS"), 24)) * time.Hour,
		// Secure cookies are only sent over HTTPS; set COOKIE_SECURE=false for
		// local http development.
		CookieSecure: getenv("COOKIE_SECURE", "true") != "false",
		// Origins allowed to call the API from a browser with credentials.
		AllowedOrigins: splitList(os.Getenv("ALLOWED_ORIGINS")),
	}
}

func splitList(s string) []string {
	var out []string
	for v := range strings.SplitSeq(s, ",") {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func atoi(s string, fallback int) int {
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return fallback
	}
	return n
}
