package agent

import (
	"context"
	"time"

	"github.com/chronocrystal/chronocrystal-go/internal/provider"
)

// Classification is the result of classifying an incoming message.
type Classification string

const (
	ClassChat  Classification = "chat"
	ClassOrder Classification = "order"
	ClassStop  Classification = "stop"
)

// Classify sends the message to the LLM for classification and returns one of
// chat, order, or stop. On any failure or ambiguous result, it defaults to ClassChat.
// A 5-second timeout is applied to prevent classification from blocking the loop.
func Classify(ctx context.Context, p *provider.Provider, message string) Classification {
	classCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	result, err := p.Classify(classCtx, message)
	if err != nil {
		return ClassChat
	}

	switch result {
	case "order":
		return ClassOrder
	case "stop":
		return ClassStop
	default:
		return ClassChat
	}
}