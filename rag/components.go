// Package rag initializes Eino components; callers use their native interfaces.
package rag

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Jackreceive/ai-learning-agent/config"
	"github.com/cloudwego/eino-ext/components/document/transformer/splitter/recursive"
	"github.com/cloudwego/eino-ext/components/embedding/ark"
	milvusindexer "github.com/cloudwego/eino-ext/components/indexer/milvus2"
	milvusretriever "github.com/cloudwego/eino-ext/components/retriever/milvus2"
	"github.com/cloudwego/eino-ext/components/retriever/milvus2/search_mode"
	"github.com/cloudwego/eino/components/document"
	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/components/indexer"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
	"github.com/milvus-io/milvus/client/v2/entity"
	"github.com/milvus-io/milvus/client/v2/milvusclient"
)

// Components holds shared resources, without wrapping Eino's operations.
type Components struct {
	Embedding   embedding.Embedder
	Transformer document.Transformer
	Indexer     indexer.Indexer
	Retriever   retriever.Retriever
	Client      *milvusclient.Client
	Collection  string
}

func New(ctx context.Context, cfg *config.Config) (_ *Components, err error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	timeout := time.Minute
	apiType := ark.APIType(cfg.ARK.EmbeddingAPIType)
	emb, err := ark.NewEmbedder(ctx, &ark.EmbeddingConfig{
		APIKey: cfg.ARK.APIKey, Model: cfg.ARK.EmbeddingModel,
		BaseURL: cfg.ARK.BaseURL, APIType: &apiType, Timeout: &timeout,
	})
	if err != nil {
		return nil, err
	}
	// Discover the actual model dimension instead of guessing a collection schema.
	vectors, err := emb.EmbedStrings(ctx, []string{"向量维度检查"})
	if err != nil {
		return nil, fmt.Errorf("检查 embedding 模型失败: %w", err)
	}
	if len(vectors) != 1 || len(vectors[0]) == 0 {
		return nil, fmt.Errorf("embedding 返回空向量")
	}
	dim := len(vectors[0])
	splitter, err := recursive.NewSplitter(ctx, &recursive.Config{
		ChunkSize: cfg.RAG.ChunkSize, OverlapSize: cfg.RAG.ChunkOverlap,
		Separators: []string{"\n\n", "\n", "。", "！", "？", ". ", " ", ""},
		LenFunc:    utf8.RuneCountInString, KeepType: recursive.KeepTypeEnd,
		IDGenerator: func(_ context.Context, id string, n int) string { return fmt.Sprintf("%s_%d", id, n) },
	})
	if err != nil {
		return nil, err
	}
	cli, err := milvusclient.New(ctx, &milvusclient.ClientConfig{
		Address: cfg.Milvus.Address, Username: cfg.Milvus.Username, Password: cfg.Milvus.Password,
	})
	if err != nil {
		return nil, fmt.Errorf("连接 Milvus 失败: %w", err)
	}
	defer func() {
		if err != nil {
			_ = cli.Close(context.Background())
		}
	}()
	exists, err := cli.HasCollection(ctx, milvusclient.NewHasCollectionOption(cfg.Milvus.Collection))
	if err != nil {
		return nil, err
	}
	if exists {
		collection, err := cli.DescribeCollection(ctx, milvusclient.NewDescribeCollectionOption(cfg.Milvus.Collection))
		if err != nil {
			return nil, err
		}
		expectedDescription := "AI learning knowledge; embedding=" + cfg.ARK.EmbeddingModel
		if strings.HasPrefix(collection.Schema.Description, "AI learning knowledge; embedding=") && collection.Schema.Description != expectedDescription {
			return nil, fmt.Errorf("collection %s 使用了其他 embedding 模型，请设置新的 MILVUS_COLLECTION 并重新导入知识", cfg.Milvus.Collection)
		}
		fields := map[string]entity.FieldType{"id": entity.FieldTypeVarChar, "content": entity.FieldTypeVarChar, "metadata": entity.FieldTypeJSON, "vector": entity.FieldTypeFloatVector}
		for _, field := range collection.Schema.Fields {
			expected, ok := fields[field.Name]
			if !ok {
				continue
			}
			if field.DataType != expected || (field.Name == "id" && (!field.PrimaryKey || field.AutoID)) || (field.Name == "vector" && field.TypeParams["dim"] != strconv.Itoa(dim)) {
				return nil, fmt.Errorf("collection %s 的字段 %s 与当前模型/组件不兼容，请设置新的 MILVUS_COLLECTION", cfg.Milvus.Collection, field.Name)
			}
			delete(fields, field.Name)
		}
		if len(fields) != 0 {
			return nil, fmt.Errorf("collection %s 缺少 Eino 所需字段，请设置新的 MILVUS_COLLECTION", cfg.Milvus.Collection)
		}
	}
	idx, err := milvusindexer.NewIndexer(ctx, &milvusindexer.IndexerConfig{
		Client: cli, Collection: cfg.Milvus.Collection, Embedding: emb,
		Description:      "AI learning knowledge; embedding=" + cfg.ARK.EmbeddingModel,
		ConsistencyLevel: milvusindexer.ConsistencyLevelStrong,
		Vector:           &milvusindexer.VectorConfig{Dimension: int64(dim), MetricType: milvusindexer.COSINE},
	})
	if err != nil {
		return nil, fmt.Errorf("初始化 Milvus indexer 失败: %w", err)
	}
	ret, err := milvusretriever.NewRetriever(ctx, &milvusretriever.RetrieverConfig{
		Client: cli, Collection: cfg.Milvus.Collection, Embedding: emb, TopK: cfg.RAG.TopK,
		OutputFields:     []string{"id", "content", "metadata"},
		ConsistencyLevel: milvusretriever.ConsistencyLevelStrong,
		SearchMode:       search_mode.NewRange(milvusretriever.COSINE, 0.3),
	})
	if err != nil {
		return nil, err
	}
	return &Components{Embedding: emb, Transformer: splitter, Indexer: idx, Retriever: ret, Client: cli, Collection: cfg.Milvus.Collection}, nil
}

// Count counts live chunks, including recent writes and deletes.
func (c *Components) Count(ctx context.Context) (int64, error) {
	result, err := c.Client.Query(ctx, milvusclient.NewQueryOption(c.Collection).
		WithOutputFields("count(*)").WithConsistencyLevel(entity.ClStrong))
	if err != nil {
		return 0, err
	}
	return result.GetColumn("count(*)").GetAsInt64(0)
}

// Clear removes this application's knowledge without dropping the collection/index.
func (c *Components) Clear(ctx context.Context) error {
	_, err := c.Client.Delete(ctx, milvusclient.NewDeleteOption(c.Collection).WithExpr(`id != ""`))
	return err
}

func BuildContext(docs []*schema.Document) string {
	if len(docs) == 0 {
		return "（知识库未命中相关内容）"
	}
	var b strings.Builder
	for i, doc := range docs {
		fmt.Fprintf(&b, "【参考资料 %d】(相关度: %.2f)\n%s\n\n", i+1, doc.Score(), doc.Content)
	}
	return b.String()
}
