// Package model wires the official Eino ARK component to application settings.
package model

import (
	"context"
	"time"

	"github.com/Jackreceive/ai-learning-agent/config"
	"github.com/cloudwego/eino-ext/components/model/ark"
)

func NewChatModel(ctx context.Context, cfg *config.ARKConfig) (*ark.ChatModel, error) {
	timeout := 2 * time.Minute
	return ark.NewChatModel(ctx, &ark.ChatModelConfig{
		APIKey: cfg.APIKey, Model: cfg.ChatModel, BaseURL: cfg.BaseURL, Timeout: &timeout,
	})
}
