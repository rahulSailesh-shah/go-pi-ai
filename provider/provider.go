package provider

import (
	"context"

	"github.com/rahulSailesh-shah/go-pi-ai/types"
)

type Provider interface {
	Stream(
		ctx context.Context,
		conversation types.Context,
	) types.AssistantMessageEventStream

	Complete(
		ctx context.Context,
		conversation types.Context,
	) (types.AssistantMessage, error)

	Model() string

	ProviderType() types.ModelProvider
}
