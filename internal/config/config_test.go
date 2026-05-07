package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig_Defaults(t *testing.T) {
	os.Clearenv()

	cfg := Load()

	if cfg.Server.Port != "8080" {
		t.Errorf("expected default port 8080, got %s", cfg.Server.Port)
	}
	if cfg.JWT.ExpireHours != 168 {
		t.Errorf("expected default expire hours 168, got %d", cfg.JWT.ExpireHours)
	}
	if cfg.Redis.Addr != "localhost:6379" {
		t.Errorf("expected default redis addr localhost:6379, got %s", cfg.Redis.Addr)
	}
}

func TestLoadConfig_FromEnv(t *testing.T) {
	os.Setenv("SERVER_PORT", "9090")
	os.Setenv("MYSQL_DSN", "root:pass@tcp(localhost:3306)/testdb")
	os.Setenv("REDIS_ADDR", "localhost:6380")
	os.Setenv("JWT_SECRET", "test-secret")
	os.Setenv("JWT_EXPIRE_HOURS", "24")
	defer func() {
		os.Unsetenv("SERVER_PORT")
		os.Unsetenv("MYSQL_DSN")
		os.Unsetenv("REDIS_ADDR")
		os.Unsetenv("JWT_SECRET")
		os.Unsetenv("JWT_EXPIRE_HOURS")
	}()

	cfg := Load()

	if cfg.Server.Port != "9090" {
		t.Errorf("expected port 9090, got %s", cfg.Server.Port)
	}
	if cfg.MySQL.DSN != "root:pass@tcp(localhost:3306)/testdb" {
		t.Errorf("unexpected MySQL DSN: %s", cfg.MySQL.DSN)
	}
	if cfg.Redis.Addr != "localhost:6380" {
		t.Errorf("unexpected Redis addr: %s", cfg.Redis.Addr)
	}
	if cfg.JWT.Secret != "test-secret" {
		t.Errorf("unexpected JWT secret: %s", cfg.JWT.Secret)
	}
	if cfg.JWT.ExpireHours != 24 {
		t.Errorf("expected expire hours 24, got %d", cfg.JWT.ExpireHours)
	}
}

func TestLoadConfig_FromYAML(t *testing.T) {
	os.Clearenv()

	yamlContent := `
server:
  port: "7070"
  gin_mode: release
mysql:
  dsn: "root:yaml@tcp(localhost:3306)/yamldb"
redis:
  addr: "localhost:6381"
  db: 2
milvus:
  addr: "localhost:19531"
jwt:
  secret: "yaml-secret"
  expire_hours: 48
`
	yamlPath := filepath.Join("testdata", "config_test.yaml")
	os.MkdirAll("testdata", 0755)
	if err := os.WriteFile(yamlPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write test yaml: %v", err)
	}
	defer os.RemoveAll("testdata")

	cfg := defaultConfig()
	loadFromYAML(cfg, yamlPath)

	if cfg.Server.Port != "7070" {
		t.Errorf("expected port 7070 from yaml, got %s", cfg.Server.Port)
	}
	if cfg.Server.GinMode != "release" {
		t.Errorf("expected gin_mode release from yaml, got %s", cfg.Server.GinMode)
	}
	if cfg.MySQL.DSN != "root:yaml@tcp(localhost:3306)/yamldb" {
		t.Errorf("unexpected MySQL DSN from yaml: %s", cfg.MySQL.DSN)
	}
	if cfg.Redis.Addr != "localhost:6381" {
		t.Errorf("unexpected Redis addr from yaml: %s", cfg.Redis.Addr)
	}
	if cfg.Redis.DB != 2 {
		t.Errorf("expected redis db 2 from yaml, got %d", cfg.Redis.DB)
	}
	if cfg.JWT.Secret != "yaml-secret" {
		t.Errorf("unexpected JWT secret from yaml: %s", cfg.JWT.Secret)
	}
	if cfg.JWT.ExpireHours != 48 {
		t.Errorf("expected expire hours 48 from yaml, got %d", cfg.JWT.ExpireHours)
	}
}

func TestLoadConfig_EnvOverridesYAML(t *testing.T) {
	yamlContent := `
server:
  port: "7070"
jwt:
  secret: "yaml-secret"
  expire_hours: 48
`
	yamlPath := filepath.Join("testdata", "config_override_test.yaml")
	os.MkdirAll("testdata", 0755)
	if err := os.WriteFile(yamlPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write test yaml: %v", err)
	}
	defer os.RemoveAll("testdata")

	os.Setenv("SERVER_PORT", "9999")
	os.Setenv("JWT_SECRET", "env-secret")
	defer func() {
		os.Unsetenv("SERVER_PORT")
		os.Unsetenv("JWT_SECRET")
	}()

	cfg := defaultConfig()
	loadFromYAML(cfg, yamlPath)
	overrideFromEnv(cfg)

	if cfg.Server.Port != "9999" {
		t.Errorf("expected env override port 9999, got %s", cfg.Server.Port)
	}
	if cfg.JWT.Secret != "env-secret" {
		t.Errorf("expected env override secret, got %s", cfg.JWT.Secret)
	}
	if cfg.JWT.ExpireHours != 48 {
		t.Errorf("expected yaml expire hours 48 (not overridden), got %d", cfg.JWT.ExpireHours)
	}
}
