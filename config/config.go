// Package config loads application settings from environment variables.
package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	ARK    ARKConfig
	Milvus MilvusConfig
	RAG    RAGConfig
}

type ARKConfig struct {
	APIKey           string
	ChatModel        string
	EmbeddingModel   string
	EmbeddingAPIType string
	BaseURL          string
}

type MilvusConfig struct {
	Address    string
	Collection string
	Username   string
	Password   string
}

type RAGConfig struct {
	TopK         int
	ChunkSize    int
	ChunkOverlap int
}

func Load() (*Config, error) {
	cfg := &Config{
		ARK: ARKConfig{
			APIKey:           os.Getenv("ARK_API_KEY"),
			ChatModel:        os.Getenv("ARK_MODEL_NAME"),
			EmbeddingModel:   os.Getenv("ARK_EMBEDDING_MODEL"),
			EmbeddingAPIType: getEnvOrDefault("ARK_EMBEDDING_API_TYPE", "multi_modal_api"),
			BaseURL:          getEnvOrDefault("ARK_BASE_URL", "https://ark.cn-beijing.volces.com/api/v3"),
		},
		Milvus: MilvusConfig{
			Address:    getEnvOrDefault("MILVUS_ADDRESS", "localhost:19530"),
			Collection: getEnvOrDefault("MILVUS_COLLECTION", "ai_learning_knowledge"),
			Username:   os.Getenv("MILVUS_USERNAME"),
			Password:   os.Getenv("MILVUS_PASSWORD"),
		},
	}
	for _, key := range []string{"ARK_API_KEY", "ARK_MODEL_NAME", "ARK_EMBEDDING_MODEL"} {
		if os.Getenv(key) == "" {
			return nil, fmt.Errorf("缺少环境变量 %s", key)
		}
	}
	for _, setting := range []struct {
		key      string
		fallback int
		target   *int
	}{
		{"RAG_TOP_K", 5, &cfg.RAG.TopK},
		{"RAG_CHUNK_SIZE", 500, &cfg.RAG.ChunkSize},
		{"RAG_CHUNK_OVERLAP", 50, &cfg.RAG.ChunkOverlap},
	} {
		value, err := strconv.Atoi(getEnvOrDefault(setting.key, strconv.Itoa(setting.fallback)))
		if err != nil {
			return nil, fmt.Errorf("%s 必须是整数: %w", setting.key, err)
		}
		*setting.target = value
	}
	if cfg.RAG.TopK <= 0 || cfg.RAG.ChunkSize <= 0 || cfg.RAG.ChunkOverlap < 0 || cfg.RAG.ChunkOverlap >= cfg.RAG.ChunkSize {
		return nil, fmt.Errorf("RAG_TOP_K、RAG_CHUNK_SIZE 必须大于 0，且 0 <= RAG_CHUNK_OVERLAP < RAG_CHUNK_SIZE")
	}
	if cfg.ARK.EmbeddingAPIType != "text_api" && cfg.ARK.EmbeddingAPIType != "multi_modal_api" {
		return nil, fmt.Errorf("ARK_EMBEDDING_API_TYPE 必须是 text_api 或 multi_modal_api")
	}
	return cfg, nil
}

func getEnvOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
