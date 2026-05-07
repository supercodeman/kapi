package config

import (
	"log"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server    ServerConfig    `yaml:"server"`
	MySQL     MySQLConfig     `yaml:"mysql"`
	Redis     RedisConfig     `yaml:"redis"`
	Milvus    MilvusConfig    `yaml:"milvus"`
	JWT       JWTConfig       `yaml:"jwt"`
	LLM       LLMConfig       `yaml:"llm"`
	Embedding EmbeddingConfig `yaml:"embedding"`
}

type ServerConfig struct {
	Port    string `yaml:"port"`
	GinMode string `yaml:"gin_mode"`
}

type MySQLConfig struct {
	DSN string `yaml:"dsn"`
}

type RedisConfig struct {
	Addr     string `yaml:"addr"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
}

type MilvusConfig struct {
	Addr string `yaml:"addr"`
}

type LLMConfig struct {
	BaseURL       string `yaml:"base_url"`
	APIKey        string `yaml:"api_key"`
	Model         string `yaml:"model"`
	MaxConcurrent int    `yaml:"max_concurrent"`
}

type EmbeddingConfig struct {
	BaseURL string `yaml:"base_url"`
	APIKey  string `yaml:"api_key"`
	Model   string `yaml:"model"`
}

type JWTConfig struct {
	Secret      string `yaml:"secret"`
	ExpireHours int    `yaml:"expire_hours"`
}

// Load 加载配置，优先级：环境变量 > config.yaml > 默认值
func Load() *Config {
	cfg := defaultConfig()

	loadFromYAML(cfg, "config.yaml")

	overrideFromEnv(cfg)

	return cfg
}

func defaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Port:    "8080",
			GinMode: "debug",
		},
		MySQL: MySQLConfig{
			DSN: "root:kapi_root_123@tcp(localhost:3306)/kapi?charset=utf8mb4&parseTime=True&loc=Local",
		},
		Redis: RedisConfig{
			Addr:     "localhost:6379",
			Password: "",
			DB:       0,
		},
		Milvus: MilvusConfig{
			Addr: "localhost:19530",
		},
		JWT: JWTConfig{
			Secret:      "kapi-dev-secret-change-in-production",
			ExpireHours: 168,
		},
		LLM: LLMConfig{
			BaseURL:       "",
			APIKey:        "",
			Model:         "",
			MaxConcurrent: 20,
		},
		Embedding: EmbeddingConfig{
			BaseURL: "",
			APIKey:  "",
			Model:   "",
		},
	}
}

func loadFromYAML(cfg *Config, path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		log.Printf("warning: failed to parse config yaml %s: %v", path, err)
	}
}

func overrideFromEnv(cfg *Config) {
	if v := os.Getenv("SERVER_PORT"); v != "" {
		cfg.Server.Port = v
	}
	if v := os.Getenv("GIN_MODE"); v != "" {
		cfg.Server.GinMode = v
	}
	if v := os.Getenv("MYSQL_DSN"); v != "" {
		cfg.MySQL.DSN = v
	}
	if v := os.Getenv("REDIS_ADDR"); v != "" {
		cfg.Redis.Addr = v
	}
	if v := os.Getenv("REDIS_PASSWORD"); v != "" {
		cfg.Redis.Password = v
	}
	if v := os.Getenv("REDIS_DB"); v != "" {
		if intVal, err := strconv.Atoi(v); err == nil {
			cfg.Redis.DB = intVal
		}
	}
	if v := os.Getenv("MILVUS_ADDR"); v != "" {
		cfg.Milvus.Addr = v
	}
	if v := os.Getenv("JWT_SECRET"); v != "" {
		cfg.JWT.Secret = v
	}
	if v := os.Getenv("JWT_EXPIRE_HOURS"); v != "" {
		if intVal, err := strconv.Atoi(v); err == nil {
			cfg.JWT.ExpireHours = intVal
		}
	}
	if v := os.Getenv("LLM_BASE_URL"); v != "" {
		cfg.LLM.BaseURL = v
	}
	if v := os.Getenv("LLM_API_KEY"); v != "" {
		cfg.LLM.APIKey = v
	}
	if v := os.Getenv("LLM_MODEL"); v != "" {
		cfg.LLM.Model = v
	}
}
