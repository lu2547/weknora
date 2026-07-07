package milvus

import (
	"context"
	"sync"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/milvus-io/milvus/client/v2/entity"
	client "github.com/milvus-io/milvus/client/v2/milvusclient"
)

// KBLookup is the minimal contract milvusRepository requires to resolve a KB
// by ID into its category + creation time. It is satisfied by the existing
// knowledgeBaseRepository (GetKnowledgeBaseByID) so wiring is a one-liner in
// the container layer.
type KBLookup interface {
	GetKnowledgeBaseByID(ctx context.Context, id string) (*types.KnowledgeBase, error)
}

type milvusRepository struct {
	filter
	client             *client.Client
	collectionBaseName string
	metricType         entity.MetricType
	// Cache for initialized collections (collectionName -> true)
	initializedCollections sync.Map
	// kbLookup resolves KB metadata (Category + CreatedAt) when the cache misses.
	// May be nil in tests; callers must always supply KB metadata in that case
	// via IndexInfo / RetrieveParams to avoid runtime errors.
	kbLookup KBLookup
	// kbInfoCache caches KB collection metadata (kbID -> KBCollectionInfo) so
	// per-write/per-search hot paths can resolve collection names without DB
	// round-trips after the first hit.
	kbInfoCache sync.Map
}

// MilvusVectorEmbedding 严格对齐 docs/milvus_collection 中 chunk 类集合的 10 字段定义：
//
//	id / embedding / chunk_id / knowledge_id / knowledge_base_id /
//	tag_id(Array<VarChar>) / file_name / is_enabled / content + content_sparse(BM25)
//
// TagIDs 写入时已平铺为当前标签 + 全部祖先 id_knowledge_tag 的数组。
type MilvusVectorEmbedding struct {
	ID              string    `json:"id"`
	Content         string    `json:"content"`
	ChunkID         string    `json:"chunk_id"`
	KnowledgeID     string    `json:"knowledge_id"`
	KnowledgeBaseID string    `json:"knowledge_base_id"`
	TagIDs          []string  `json:"tag_id"`
	FileName        string    `json:"file_name"`
	Embedding       []float32 `json:"embedding"`
	IsEnabled       bool      `json:"is_enabled"`
}

type MilvusVectorEmbeddingWithScore struct {
	MilvusVectorEmbedding
	Score float64
}
