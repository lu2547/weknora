package types

// SummaryItem represents a knowledge-level document summary that will be
// written to the global summary_knowledge_base Milvus collection.
// 严格对齐 docs/milvus_collection 中 summary collection 的 8 字段定义：
//
//	id / embedding / knowledge_id / knowledge_base_id / tag_id(Array) /
//	file_name / is_enabled / content + content_sparse(BM25 自动生成)
//
// title / file_type / created_at 仅存 PG，不入 Milvus。
type SummaryItem struct {
	// ID 是 Milvus summary collection 的主键（独立 UUID，与 KnowledgeID 解耦，
	// 便于幂等 upsert 与 ID 回查 PG 拉富字段）。
	ID string
	// KnowledgeID 在 Milvus 是普通索引字段，PG 端通过它回拉 title/file_type/created_at
	KnowledgeID string
	// KnowledgeBaseID 帮助按 KB 过滤
	KnowledgeBaseID string
	// TagIDs 是当前知识关联标签 + 全部祖先 id_knowledge_tag 的平铺数组。
	// 例如标签路径 a/b/c → [id_a, id_b, id_c]，写入 Milvus tag_id Array<VarChar>。
	// 检索时 ARRAY_CONTAINS_ANY 命中任意层级即可。空 = 无标签。
	TagIDs []string
	// FileName 必填，用于 LLM/tool caller 展示与命中片段还原。
	FileName string
	// IsEnabled 是否启用，false 时检索过滤掉。
	IsEnabled bool
	// Content 是写入 Milvus content 字段的全文（参与 BM25 + 稠密向量），
	// 通常是 summary 文本本身。
	Content string
	// Vector 是 Content 的 dense embedding，维度 MUST 与 collection 一致。
	Vector []float32

	// ===== 以下字段仅 PG 持久化使用，不写入 Milvus =====
	// Title 标题（PG 端）
	Title string
	// FileType 文件类型（PG 端）
	FileType string
	// CreatedAt unix 毫秒时间戳（PG 端）
	CreatedAt int64
}

// SummaryHit is a single result from the summary collection search.
// 仅包含 Milvus 端能直接拿到的字段；title/file_type/created_at 等富字段
// 业务层 search 命中后按 KnowledgeID 回 PG 拉取。
type SummaryHit struct {
	ID              string
	KnowledgeID     string
	KnowledgeBaseID string
	// TagIDs 命中文档的标签祖先链（写入时已平铺）
	TagIDs   []string
	FileName string
	Content  string
	Score    float64
}

// SummaryFilter 约束摘要检索的范围。
// 多个字段之间是 AND，同一字段的列表内部是 OR。
type SummaryFilter struct {
	// KnowledgeBaseIDs 限定结果所在的知识库（空 = 所有）
	KnowledgeBaseIDs []string
	// KnowledgeIDs 限定为这些具体的知识 ID（空 = 所有）
	KnowledgeIDs []string
	// TagIDs 标签过滤：传任意层级的 id_knowledge_tag 都能命中
	// （Milvus 侧 ARRAY_CONTAINS_ANY(tag_id, [...])）。
	TagIDs []string

	// FileNameKeywords 对 file_name 做模糊包含匹配（like "%kw%"），
	// 多个关键词之间取并集（OR）。
	FileNameKeywords []string

	// 注意：file_type / created_at 仅存 PG，不在 Milvus collection；
	// 如需按这些字段过滤，应在 service 层先查 PG 得到 KnowledgeIDs 再喂给本 Filter。
}
