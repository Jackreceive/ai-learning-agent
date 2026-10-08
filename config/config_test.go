package config

import "testing"

func TestLoadValidation(t *testing.T) {
	t.Setenv("ARK_API_KEY", "test-key")
	t.Setenv("ARK_MODEL_NAME", "test-chat")
	t.Setenv("ARK_EMBEDDING_MODEL", "test-embedding")
	for _, key := range []string{"ARK_EMBEDDING_API_TYPE", "RAG_TOP_K", "RAG_CHUNK_SIZE", "RAG_CHUNK_OVERLAP", "MILVUS_ADDRESS", "MILVUS_COLLECTION"} {
		t.Setenv(key, "")
	}
	cfg, err := Load()
	if err != nil || cfg.Milvus.Address != "localhost:19530" {
		t.Fatalf("defaults: cfg=%v err=%v", cfg, err)
	}
	for _, tc := range []struct{ key, value string }{
		{"RAG_TOP_K", "0"}, {"RAG_TOP_K", "x"}, {"RAG_CHUNK_SIZE", "0"},
		{"RAG_CHUNK_OVERLAP", "-1"}, {"RAG_CHUNK_OVERLAP", "500"},
		{"ARK_EMBEDDING_API_TYPE", "invalid"}, {"ARK_EMBEDDING_MODEL", ""},
	} {
		t.Run(tc.key+"="+tc.value, func(t *testing.T) {
			t.Setenv(tc.key, tc.value)
			if _, err := Load(); err == nil {
				t.Fatal("expected invalid configuration to fail")
			}
		})
	}
}
