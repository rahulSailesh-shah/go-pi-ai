package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
	"github.com/rahulshah/go-pi-ai/types"
)

type ProviderConfig struct {
	BaseURL string
	APIKey  string
	Models  []string
}

type Config struct {
	Providers map[types.ModelProvider]ProviderConfig
}

func NewConfig() *Config {
	return &Config{
		Providers: make(map[types.ModelProvider]ProviderConfig),
	}
}

// FromEnv loads configuration from .env file, then falls back to environment variables
func FromEnv() (*Config, error) {
	cfg := NewConfig()

	if err := godotenv.Load(); err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to load .env file: %w", err)
		}
	}

	// NVIDIA configuration
	if nvidiaAPIKey := getEnv("NVIDIA_API_KEY"); nvidiaAPIKey != "" {
		cfg.Providers[types.ProviderNvidia] = ProviderConfig{
			BaseURL: "https://integrate.api.nvidia.com/v1",
			APIKey:  nvidiaAPIKey,
			Models:  []string{"openai/gpt-oss-20b"},
		}
	}

	// OpenAI configuration
	if openaiAPIKey := getEnv("OPENAI_API_KEY"); openaiAPIKey != "" {
		cfg.Providers[types.ProviderOpenAI] = ProviderConfig{
			BaseURL: "https://api.openai.com/v1",
			APIKey:  openaiAPIKey,
			Models:  []string{"openai/gpt-oss-20b"},
		}
	}

	if len(cfg.Providers) == 0 {
		return nil, fmt.Errorf("no provider configurations found")
	}

	return cfg, nil
}

func (c *Config) GetProvider(name types.ModelProvider) (ProviderConfig, error) {
	provider, ok := c.Providers[name]
	if !ok {
		return ProviderConfig{}, fmt.Errorf("provider %s not found", name)
	}
	return provider, nil
}

func (c *Config) SetProvider(name types.ModelProvider, provider ProviderConfig) {
	c.Providers[name] = provider
}

func getEnv(key string) string {
	return os.Getenv(key)
}
