package provider

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/rahulSailesh-shah/go-pi-ai/config"
	openaiProvider "github.com/rahulSailesh-shah/go-pi-ai/internal/provider/openai"
	"github.com/rahulSailesh-shah/go-pi-ai/types"
)

// --- Global Registry ---
var (
	globalRegistry atomic.Pointer[Registry]
	globalInitOnce sync.Once
	globalInitErr  atomic.Value
)

func GetRegistry() (*Registry, error) {
	globalInitOnce.Do(func() {
		cfg, err := config.FromEnv()
		if err != nil {
			globalInitErr.Store(err)
			return
		}

		registry := NewRegistry()
		if err := registry.RegisterFromConfig(cfg); err != nil {
			globalInitErr.Store(err)
			return
		}

		globalRegistry.Store(registry)
	})

	if err, ok := globalInitErr.Load().(error); ok && err != nil {
		return nil, err
	}

	return globalRegistry.Load(), nil
}

func GetModel(providerType types.ModelProvider, modelID string) (Provider, error) {
	registry, err := GetRegistry()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize registry: %w", err)
	}
	return registry.Get(providerType, modelID)
}

type Registry struct {
	models map[types.ModelProvider]map[string]Provider
	mu     sync.RWMutex
}

// --- New Custom Registry ---
func NewRegistry() *Registry {
	return &Registry{
		models: make(map[types.ModelProvider]map[string]Provider),
	}
}

func (r *Registry) RegisterFromConfig(cfg *config.Config) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for providerName, providerCfg := range cfg.Providers {
		switch providerName {
		case types.ProviderNvidia:
			for _, modelID := range providerCfg.Models {
				p := openaiProvider.New(
					openaiProvider.Config{
						URL:    providerCfg.BaseURL,
						APIKey: providerCfg.APIKey,
					},
					modelID,
					types.ProviderNvidia,
				)
				r.register(providerName, modelID, p)
			}
		case types.ProviderOpenAI:
			for _, modelID := range providerCfg.Models {
				p := openaiProvider.New(
					openaiProvider.Config{
						URL:    providerCfg.BaseURL,
						APIKey: providerCfg.APIKey,
					},
					modelID,
					types.ProviderOpenAI,
				)
				r.register(providerName, modelID, p)
			}
		default:
			continue
		}
	}

	return nil
}

func (r *Registry) Register(providerType types.ModelProvider, modelID string, provider Provider) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.register(providerType, modelID, provider)
}

func (r *Registry) register(providerType types.ModelProvider, modelID string, provider Provider) error {
	if _, ok := r.models[providerType]; !ok {
		r.models[providerType] = make(map[string]Provider)
	}
	r.models[providerType][modelID] = provider
	return nil
}

func (r *Registry) Get(providerType types.ModelProvider, modelID string) (Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	providers, ok := r.models[providerType]
	if !ok {
		return nil, fmt.Errorf("%w: provider %s", types.ErrProviderNotFound, providerType)
	}

	provider, ok := providers[modelID]
	if !ok {
		return nil, fmt.Errorf("%w: model %s", types.ErrModelNotFound, modelID)
	}

	return provider, nil
}

func (r *Registry) ListProviders() []types.ModelProvider {
	r.mu.RLock()
	defer r.mu.RUnlock()

	providers := make([]types.ModelProvider, 0, len(r.models))
	for provider := range r.models {
		providers = append(providers, provider)
	}
	return providers
}

func (r *Registry) ListModels(providerType types.ModelProvider) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	providers, ok := r.models[providerType]
	if !ok {
		return nil
	}

	models := make([]string, 0, len(providers))
	for modelID := range providers {
		models = append(models, modelID)
	}
	return models
}
