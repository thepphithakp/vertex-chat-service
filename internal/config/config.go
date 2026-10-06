package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port         string
	DB           DBConfig
	JWT          JWTConfig
	Notification NotificationConfig
	PetService   PetServiceConfig
	WS           WSConfig
	Log          LogConfig
	Shutdown     ShutdownConfig
}

type DBConfig struct {
	Host       string
	Port       string
	User       string
	Password   string
	Name       string
	SSLMode    string
	SearchPath string
}

type JWTConfig struct {
	PublicKeys string
	Issuer     string
	Audience   string
}

type NotificationConfig struct {
	ServiceURL string
	Token      string
}

// PetServiceConfig คุมการเรียก pet-service เพื่อตรวจว่าสองคนมีสัตว์เลี้ยง
// ร่วมกันไหม — ใช้ token เดียวกับที่ ev-service/notification-service ใช้กัน
// เรียกข้ามบริการ (X-Service-Token รูปแบบเดียวกันทั้งระบบ)
type PetServiceConfig struct {
	ServiceURL string
	Token      string
}

// WSConfig คุมการยืนยันตัวตนและ URL ที่ให้ client ต่อ WebSocket ตรง — ไม่ผ่าน
// Cloudflare Worker proxy (พิสูจน์แล้วว่าพังเพราะ origin เสิร์ฟ HTTP/2 ด้วย)
type WSConfig struct {
	// AllowedOrigins คือ origin ของหน้าเว็บที่อนุญาตให้เปิด WebSocket มาหา
	// service นี้ได้ — กัน cross-site WebSocket hijacking (CSWSH)
	AllowedOrigins []string
	// PublicBaseURL คือ wss://... เต็มรูปที่ client ต้องต่อ ไม่ใช่ path สัมพัทธ์
	// เพราะต้องข้าม Worker ไปเลย
	PublicBaseURL string
}

type LogConfig struct {
	Level string
}

type ShutdownConfig struct {
	DrainDelay time.Duration
	Timeout    time.Duration
}

func (c DBConfig) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s search_path=%s TimeZone=Asia/Bangkok",
		c.Host, c.Port, c.User, c.Password, c.Name, c.SSLMode, c.SearchPath,
	)
}

func (c DBConfig) Redacted() string {
	return fmt.Sprintf("host=%s port=%s user=%s dbname=%s search_path=%s",
		c.Host, c.Port, c.User, c.Name, c.SearchPath)
}

const minServiceTokenLength = 32

func Load() (Config, error) {
	cfg := Config{
		Port: env("PORT", "4005"),
		DB: DBConfig{
			Host:       env("DB_HOST", "localhost"),
			Port:       env("DB_PORT", "5432"),
			User:       os.Getenv("DB_USER"),
			Password:   os.Getenv("DB_PASSWORD"),
			Name:       env("DB_NAME", "vertex"),
			SSLMode:    env("DB_SSL_MODE", "disable"),
			SearchPath: env("DB_SEARCH_PATH", "chat"),
		},
		JWT: JWTConfig{
			PublicKeys: os.Getenv("JWT_PUBLIC_KEYS"),
			Issuer:     os.Getenv("JWT_ISSUER"),
			Audience:   os.Getenv("JWT_AUDIENCE"),
		},
		Notification: NotificationConfig{
			ServiceURL: os.Getenv("NOTIFICATION_SERVICE_URL"),
			Token:      os.Getenv("PUSH_SERVICE_TOKEN"),
		},
		PetService: PetServiceConfig{
			ServiceURL: os.Getenv("PET_SERVICE_URL"),
			Token:      os.Getenv("PET_SERVICE_TOKEN"),
		},
		WS: WSConfig{
			AllowedOrigins: splitCSV(os.Getenv("CHAT_ALLOWED_ORIGINS")),
			PublicBaseURL:  os.Getenv("CHAT_PUBLIC_WS_URL"),
		},
		Log: LogConfig{Level: env("LOG_LEVEL", "info")},
		Shutdown: ShutdownConfig{
			DrainDelay: envDuration("SHUTDOWN_DRAIN_DELAY", 5*time.Second),
			Timeout:    envDuration("SHUTDOWN_TIMEOUT", 20*time.Second),
		},
	}

	var missing []string
	if cfg.DB.User == "" {
		missing = append(missing, "DB_USER")
	}
	if cfg.DB.Password == "" {
		missing = append(missing, "DB_PASSWORD")
	}
	if cfg.JWT.PublicKeys == "" {
		missing = append(missing, "JWT_PUBLIC_KEYS")
	}
	if cfg.Notification.ServiceURL == "" {
		missing = append(missing, "NOTIFICATION_SERVICE_URL")
	}
	if cfg.Notification.Token == "" {
		missing = append(missing, "PUSH_SERVICE_TOKEN")
	}
	if cfg.PetService.ServiceURL == "" {
		missing = append(missing, "PET_SERVICE_URL")
	}
	if cfg.PetService.Token == "" {
		missing = append(missing, "PET_SERVICE_TOKEN")
	}
	if len(cfg.WS.AllowedOrigins) == 0 {
		missing = append(missing, "CHAT_ALLOWED_ORIGINS")
	}
	if cfg.WS.PublicBaseURL == "" {
		missing = append(missing, "CHAT_PUBLIC_WS_URL")
	}
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("ไม่ได้ตั้ง environment variable ที่จำเป็น: %s", strings.Join(missing, ", "))
	}

	if len(cfg.Notification.Token) < minServiceTokenLength {
		return Config{}, fmt.Errorf("PUSH_SERVICE_TOKEN สั้นเกินไป (%d ตัวอักษร) ต้องอย่างน้อย %d",
			len(cfg.Notification.Token), minServiceTokenLength)
	}
	if len(cfg.PetService.Token) < minServiceTokenLength {
		return Config{}, fmt.Errorf("PET_SERVICE_TOKEN สั้นเกินไป (%d ตัวอักษร) ต้องอย่างน้อย %d",
			len(cfg.PetService.Token), minServiceTokenLength)
	}
	if !strings.HasPrefix(cfg.WS.PublicBaseURL, "wss://") && !strings.HasPrefix(cfg.WS.PublicBaseURL, "ws://") {
		return Config{}, fmt.Errorf("CHAT_PUBLIC_WS_URL ต้องขึ้นต้นด้วย wss:// หรือ ws:// (ได้ %q)", cfg.WS.PublicBaseURL)
	}

	return cfg, nil
}

func splitCSV(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	if d, err := time.ParseDuration(v); err == nil {
		return d
	}
	if n, err := strconv.Atoi(v); err == nil {
		return time.Duration(n) * time.Second
	}
	return fallback
}
