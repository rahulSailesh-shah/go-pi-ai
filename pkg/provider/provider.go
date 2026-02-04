package provider

import (
	"ai/pkg/types"
	"context"
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
}
