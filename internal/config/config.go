package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Login             string
	Password          string
	SessionSecret     string
	ProxyURL          string
	GoogleAPIKey      string
	DataDir           string
	Port              string
	FileTTL           time.Duration
	MaxConcurrentJobs int
	CookiesFile       string
}

func Load() (*Config, error) {
	_ = godotenv.Load() // .env is optional; real env wins in docker

	c := &Config{
		Login:             os.Getenv("AUTH_LOGIN"),
		Password:          os.Getenv("AUTH_PASSWORD"),
		SessionSecret:     os.Getenv("SESSION_SECRET"),
		ProxyURL:          os.Getenv("PROXY_URL"),
		GoogleAPIKey:      os.Getenv("GOOGLE_API_KEY"),
		DataDir:           getenv("DATA_DIR", "/data"),
		Port:              getenv("PORT", "8080"),
		FileTTL:           time.Hour,
		MaxConcurrentJobs: 2,
		CookiesFile:       os.Getenv("COOKIES_FILE"),
	}
	if c.Login == "" || c.Password == "" || c.SessionSecret == "" {
		return nil, fmt.Errorf("AUTH_LOGIN, AUTH_PASSWORD and SESSION_SECRET are required")
	}
	if v := os.Getenv("FILE_TTL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("bad FILE_TTL: %w", err)
		}
		c.FileTTL = d
	}
	if v := os.Getenv("MAX_CONCURRENT_JOBS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return nil, fmt.Errorf("bad MAX_CONCURRENT_JOBS: %q", v)
		}
		c.MaxConcurrentJobs = n
	}
	return c, nil
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
