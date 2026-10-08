package rag

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/Jackreceive/ai-learning-agent/config"
	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"github.com/milvus-io/milvus/client/v2/milvusclient"
)

// Opt-in: calls the configured embedding service and a fresh Milvus collection.
func TestMilvusIntegration(t *testing.T) {
	if os.Getenv("MILVUS_INTEGRATION") != "1" {
		t.Skip("set MILVUS_INTEGRATION=1 to run against Milvus and ARK")
	}
	if err := godotenv.Load("../.env"); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Milvus.Collection = "ai_learning_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	cfg.RAG.ChunkSize, cfg.RAG.ChunkOverlap = 60, 10
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	c, err := New(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if err := c.Client.DropCollection(cleanup, milvusclient.NewDropCollectionOption(c.Collection)); err != nil {
			t.Error(err)
		}
		_ = c.Client.Close(cleanup)
	}()
	text := strings.Repeat("Milvus 是向量数据库，Eino 使用 Retriever 组件检索知识。", 6)
	docs, err := c.Transformer.Transform(ctx, []*schema.Document{{ID: "sample", Content: text, MetaData: map[string]any{"source": "integration"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(docs))
	}
	ids := map[string]bool{}
	for _, d := range docs {
		if ids[d.ID] {
			t.Fatalf("duplicate chunk ID: %s", d.ID)
		}
		ids[d.ID] = true
		if !utf8.ValidString(d.Content) || utf8.RuneCountInString(d.Content) > cfg.RAG.ChunkSize {
			t.Fatalf("invalid Chinese chunk: %q", d.Content)
		}
	}
	if _, err := c.Indexer.Store(ctx, docs); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Indexer.Store(ctx, docs); err != nil {
		t.Fatal(err)
	}
	count, err := c.Count(ctx)
	if err != nil || count != int64(len(docs)) {
		t.Fatalf("upsert count=%d, want=%d, err=%v", count, len(docs), err)
	}
	// Reconnect to prove documents are persisted outside the original instance.
	reopened, err := New(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Client.Close(context.Background())
	hits, err := reopened.Retriever.Retrieve(ctx, "Milvus 是什么？")
	if err != nil || len(hits) == 0 {
		t.Fatalf("retrieval hits=%d err=%v", len(hits), err)
	}
	if hits[0].ID == "" || hits[0].Content == "" || hits[0].MetaData["source"] != "integration" {
		t.Fatalf("document fields not preserved: %+v", hits[0])
	}
	if err := c.Clear(ctx); err != nil {
		t.Fatal(err)
	}
	count, err = reopened.Count(ctx)
	if err != nil || count != 0 {
		t.Fatalf("clear count=%d err=%v", count, err)
	}
	hits, err = reopened.Retriever.Retrieve(ctx, "Milvus 是什么？")
	if err != nil || len(hits) != 0 {
		t.Fatalf("retrieval after clear hits=%d err=%v", len(hits), err)
	}
	// Existing collection is still usable after clear.
	if _, err := c.Indexer.Store(ctx, docs[:1]); err != nil {
		t.Fatal(err)
	}
	count, err = c.Count(ctx)
	if err != nil || count != 1 {
		t.Fatalf("store after clear count=%d err=%v", count, err)
	}
}
