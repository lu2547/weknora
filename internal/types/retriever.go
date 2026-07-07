package types

// RetrieverEngineType represents the type of retriever engine
type RetrieverEngineType string

// RetrieverEngineType constants
const (
	PostgresRetrieverEngineType      RetrieverEngineType = "postgres"
	ElasticsearchRetrieverEngineType RetrieverEngineType = "elasticsearch"
	InfinityRetrieverEngineType      RetrieverEngineType = "infinity"
	ElasticFaissRetrieverEngineType  RetrieverEngineType = "elasticfaiss"
	QdrantRetrieverEngineType        RetrieverEngineType = "qdrant"
	MilvusRetrieverEngineType        RetrieverEngineType = "milvus"
	WeaviateRetrieverEngineType      RetrieverEngineType = "weaviate"
	SQLiteRetrieverEngineType        RetrieverEngineType = "sqlite"
)

// RetrieverType represents the type of retriever
type RetrieverType string

// RetrieverType constants
const (
	KeywordsRetrieverType  RetrieverType = "keywords"  // Keywords retriever
	VectorRetrieverType    RetrieverType = "vector"    // Vector retriever
	WebSearchRetrieverType RetrieverType = "websearch" // Web search retriever
)

// RetrieveParams represents the parameters for retrieval
type RetrieveParams struct {
	// Query text
	Query string
	// Query embedding (used for vector retrieval)
	Embedding []float32
	// Knowledge base IDs
	KnowledgeBaseIDs []string
	// Knowledge IDs
	KnowledgeIDs []string
	// TagIDs 过滤列表：传任意层级的 id_knowledge_tag 都能命中（Milvus 侧 ARRAY_CONTAINS_ANY）
	TagIDs []string
	// Excluded knowledge IDs
	ExcludeKnowledgeIDs []string
	// Excluded chunk IDs
	ExcludeChunkIDs []string
	// Number of results to return
	TopK int
	// Similarity threshold
	Threshold float64
	// Knowledge type (e.g., "faq", "manual") - determines which index to use
	KnowledgeType string
	// Additional parameters, different retrievers may require different parameters
	AdditionalParams map[string]interface{}
	// Retriever type
	RetrieverType RetrieverType // Retriever type

	// KnowledgeBases is an optional snapshot of the KBs participating in this
	// retrieve call. Engines that need per-KB metadata (e.g., Milvus's
	// CollectionResolver, which routes by KB.Category and KB.CreatedAt) MUST
	// rely on this field rather than on KnowledgeBaseIDs alone. Engines that
	// don't care can ignore it.
	KnowledgeBases []*KnowledgeBase
}

// RetrieverEngineParams represents the parameters for retriever engine
type RetrieverEngineParams struct {
	// Retriever engine type
	RetrieverEngineType RetrieverEngineType `yaml:"retriever_engine_type" json:"retriever_engine_type"`
	// Retriever type
	RetrieverType RetrieverType `yaml:"retriever_type"        json:"retriever_type"`
}

// IndexWithScore represents the index with score
type IndexWithScore struct {
	// ID
	ID string
	// Content
	Content string
	// Source ID
	SourceID string
	// Source type
	SourceType SourceType
	// Chunk ID
	ChunkID string
	// Knowledge ID
	KnowledgeID string
	// Knowledge base ID
	KnowledgeBaseID string
	// TagIDs 包含该 chunk 关联标签 + 全部祖先 id_knowledge_tag（祖先链平铺）
	TagIDs []string
	// Score
	Score float64
	// Match type
	MatchType MatchType
	// IsEnabled
	IsEnabled bool
}

// GetScore returns the score for ScoreComparable interface
func (i *IndexWithScore) GetScore() float64 {
	return i.Score
}

// RetrieveResult represents the result of retrieval
type RetrieveResult struct {
	Results             []*IndexWithScore   // Retrieval results
	RetrieverEngineType RetrieverEngineType // Retrieval source type
	RetrieverType       RetrieverType       // Retrieval type
	Error               error               // Retrieval error
}
