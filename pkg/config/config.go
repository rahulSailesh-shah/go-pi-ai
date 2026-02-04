package config

import (
	"ai/pkg/types"
	"log"
	"os"

	"github.com/joho/godotenv"
)

type ProviderConfig struct {
	BaseURL string
	APIKey  string
	Models  []string
}

type Config struct {
	Providers map[types.ModelProvider]ProviderConfig
}

var AppConfig *Config

func init() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables.")
	}

	AppConfig = &Config{
		Providers: map[types.ModelProvider]ProviderConfig{
			types.ApiNvidia: {
				BaseURL: getEnv("NVIDIA_API_URL"),
				APIKey:  getEnv("NVIDIA_API_KEY"),
				Models:  []string{"openai/gpt-oss-20b"},
			},
		},
	}
}

func getEnv(key string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	panic("environment variable " + key + " not set")
}

func GetProvider(name types.ModelProvider) (ProviderConfig, bool) {
	provider, ok := AppConfig.Providers[name]
	return provider, ok
}
