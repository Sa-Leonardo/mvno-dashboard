package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	AppAddr                  string
	AdminKey                 string
	Easy2UseBaseURL          string
	Easy2UseUserToken        string
	AllowedCNPJs             []string
	RechargeIntervalDays     int
	RechargeSafetyWindowDays int
	DefaultRechargeQuantity  int
	ProviderRequestDelay     time.Duration
	EnableRealRecharge       bool
	EnableDevRoutes          bool
}

func Load() (Config, error) {
	_ = loadDotEnv(findDotEnv())

	cfg := Config{
		AppAddr:                  getAppAddr(),
		AdminKey:                 os.Getenv("ADMIN_KEY"),
		Easy2UseBaseURL:          strings.TrimRight(getEnv("EASY2USE_BASE_URL", "https://mvno.tipbrasil.com.br/api/public"), "/"),
		Easy2UseUserToken:        os.Getenv("EASY2USE_USER_TOKEN"),
		AllowedCNPJs:             parseCSV(os.Getenv("ALLOWED_CNPJS")),
		RechargeIntervalDays:     getEnvInt("RECHARGE_INTERVAL_DAYS", 90),
		RechargeSafetyWindowDays: getEnvInt("RECHARGE_SAFETY_WINDOW_DAYS", 0),
		DefaultRechargeQuantity:  getEnvInt("DEFAULT_RECHARGE_QUANTITY", 1),
		ProviderRequestDelay:     time.Duration(getEnvInt("PROVIDER_REQUEST_DELAY_MS", 1200)) * time.Millisecond,
		EnableRealRecharge:       getEnvBool("ENABLE_REAL_RECHARGE", false),
		EnableDevRoutes:          getEnvBool("ENABLE_DEV_ROUTES", false),
	}

	if cfg.AdminKey == "" {
		return Config{}, errors.New("ADMIN_KEY is required")
	}
	if cfg.Easy2UseUserToken == "" {
		return Config{}, errors.New("EASY2USE_USER_TOKEN is required")
	}
	if len(cfg.AllowedCNPJs) == 0 {
		return Config{}, errors.New("ALLOWED_CNPJS is required")
	}
	if cfg.RechargeIntervalDays <= 0 {
		return Config{}, errors.New("RECHARGE_INTERVAL_DAYS must be greater than zero")
	}
	if cfg.RechargeSafetyWindowDays < 0 {
		return Config{}, errors.New("RECHARGE_SAFETY_WINDOW_DAYS cannot be negative")
	}
	if cfg.DefaultRechargeQuantity < 1 {
		return Config{}, errors.New("DEFAULT_RECHARGE_QUANTITY must be at least 1")
	}

	return cfg, nil
}

func getAppAddr() string {
	if value := strings.TrimSpace(os.Getenv("APP_ADDR")); value != "" {
		return value
	}
	if port := strings.TrimSpace(os.Getenv("PORT")); port != "" {
		return ":" + port
	}
	return ":8080"
}

func findDotEnv() string {
	dir, err := os.Getwd()
	if err != nil {
		return ".env"
	}
	for {
		path := filepath.Join(dir, ".env")
		if _, err := os.Stat(path); err == nil {
			return path
		}
		next := filepath.Dir(dir)
		if next == dir {
			return ".env"
		}
		dir = next
	}
}

func getEnv(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func getEnvInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getEnvBool(key string, fallback bool) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if value == "" {
		return fallback
	}
	return value == "1" || value == "true" || value == "yes" || value == "sim"
}

func parseCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		clean := OnlyDigits(part)
		if clean != "" {
			out = append(out, clean)
		}
	}
	return out
}

func OnlyDigits(value string) string {
	var b strings.Builder
	for _, r := range value {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func loadDotEnv(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); !exists {
			if err := os.Setenv(key, value); err != nil {
				return fmt.Errorf("set env %s: %w", key, err)
			}
		}
	}
	return scanner.Err()
}
