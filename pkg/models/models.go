package models

import (
	"ai/pkg/config"
	"ai/pkg/provider"
	openaiProvider "ai/pkg/provider/openai"
	"ai/pkg/types"
	"sync"
)

var (
	modelRegistry = make(map[types.ModelProvider]map[string]provider.Provider)
	registryMu    sync.RWMutex
)

func init() {
	if nvidiaConfig, ok := config.GetProvider(types.ApiNvidia); ok {
		for _, modelID := range nvidiaConfig.Models {
			nvidiaProvider := openaiProvider.NewOpenAIProvider(
				openaiProvider.OpenAIConfig{
					URL:    nvidiaConfig.BaseURL,
					APIKey: nvidiaConfig.APIKey,
				},
				modelID,
				types.ApiNvidia,
			)
			AddModel(types.ApiNvidia, modelID, nvidiaProvider)
		}
	}
}

func AddModel(providerType types.ModelProvider, id string, model provider.Provider) {
	registryMu.Lock()
	defer registryMu.Unlock()

	if _, ok := modelRegistry[providerType]; !ok {
		modelRegistry[providerType] = make(map[string]provider.Provider)
	}

	modelRegistry[providerType][id] = model
}

func GetModel(providerType types.ModelProvider, id string) provider.Provider {
	registryMu.RLock()
	defer registryMu.RUnlock()

	if models, ok := modelRegistry[providerType]; ok {
		if model, ok := models[id]; ok {
			return model
		}
	}
	return nil
}
