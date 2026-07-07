package types

// SourceType represents the type of content source
type SourceType int

const (
	ChunkSourceType   SourceType = iota // Source is a text chunk
	PassageSourceType                   // Source is a passage
	SummarySourceType                   // Source is a summary
)

// MatchType represents the type of matching algorithm
type MatchType int

const (
	MatchTypeEmbedding MatchType = iota
	MatchTypeKeywords
	MatchTypeNearByChunk
	MatchTypeHistory
	MatchTypeParentChunk   // 父Chunk匹配类型
	MatchTypeRelationChunk // 关系Chunk匹配类型
	MatchTypeGraph
	MatchTypeWebSearch    // 网络搜索匹配类型
	MatchTypeDirectLoad   // 直接加载匹配类型
	MatchTypeDataAnalysis // 数据分析匹配类型
)

// IndexInfo contains information about indexed content
type IndexInfo struct {
	ID              string     // Unique identifier
	Content         string     // Content text
	SourceID        string     // ID of the source document
	SourceType      SourceType // Type of the source
	ChunkID         string     // ID of the text chunk
	KnowledgeID     string     // ID of the knowledge
	KnowledgeBaseID string     // ID of the knowledge base
	KnowledgeType   string     // Type of the knowledge (e.g., "faq", "manual")
	// TagIDs 是当前 chunk/knowledge 关联标签 + 全部祖先 id_knowledge_tag 的平铺数组。
	// 例如标签路径 a/b/c → [id_a, id_b, id_c]。Milvus tag_id 字段为 Array<VarChar>，
	// 检索时 ARRAY_CONTAINS_ANY 命中任意层级即可。空 = 无标签。
	TagIDs        []string
	FileName      string // 文件名，写入 Milvus 以支持文件名过滤
	IsEnabled     bool   // Whether the chunk is enabled for retrieval
	IsRecommended bool   // Whether the chunk is recommended

	// KBCategory carries the owning knowledge base category
	// ("personal" | "public" | "enterprise"). Required by the Milvus
	// CollectionResolver to route writes/reads to the right collection.
	KBCategory string
	// KBCreatedAtUnixMs is the owning knowledge base's CreatedAt in unix
	// milliseconds. Kept for backward compatibility / metadata; the enterprise
	// collection name now uses the convention `enterprise_<knowledge_base_id>`
	// and no longer depends on this field.
	KBCreatedAtUnixMs int64
}

// ChunkTagUpdate 描述一个 chunk 的标签更新。
// 用于向量库同步 tag_id：TagIDs 已是祖先链平铺数组（root→leaf）。
type ChunkTagUpdate struct {
	TagIDs []string
}

// LeafTagID 返回 TagIDs 中的叶子节点 id（最后一个），主要供 PG/SQLite/ES/Qdrant/Weaviate
// 这类仍以单值字段存储 tag_id 的 retriever 写入时降级使用。空切片返回空串。
func LeafTagID(tagIDs []string) string {
	if len(tagIDs) == 0 {
		return ""
	}
	return tagIDs[len(tagIDs)-1]
}

// SingletonTagIDs 把单值 tag_id 字段（来自非 Milvus retriever）回填成数组，
// 便于上层统一以 TagIDs 形式处理。空串返回 nil。
func SingletonTagIDs(tagID string) []string {
	if tagID == "" {
		return nil
	}
	return []string{tagID}
}
