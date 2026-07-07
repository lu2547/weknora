package types

import (
	"database/sql/driver"
	"encoding/json"
)

// SearchTargetType represents the type of search target
type SearchTargetType string

const (
	// SearchTargetTypeKnowledgeBase - search entire knowledge base
	SearchTargetTypeKnowledgeBase SearchTargetType = "knowledge_base"
	// SearchTargetTypeKnowledge - search specific knowledge files within a knowledge base
	SearchTargetTypeKnowledge SearchTargetType = "knowledge"
)

// SearchTarget represents a unified search target
// Either search an entire knowledge base, or specific knowledge files within a knowledge base
type SearchTarget struct {
	// Type of search target
	Type SearchTargetType `json:"type"`
	// KnowledgeBaseID is the ID of the knowledge base to search
	KnowledgeBaseID string `json:"knowledge_base_id"`
	// TenantID is the tenant ID that owns this knowledge base
	// Required for cross-tenant shared KB queries
	TenantID uint64 `json:"tenant_id"`
	// KnowledgeIDs is the list of specific knowledge IDs to search within the knowledge base
	// Only used when Type is SearchTargetTypeKnowledge
	KnowledgeIDs []string `json:"knowledge_ids,omitempty"`
}

// SearchTargets is a list of search targets, pre-computed at request entry point
type SearchTargets []*SearchTarget

// GetAllKnowledgeBaseIDs returns all unique knowledge base IDs from the search targets
func (st SearchTargets) GetAllKnowledgeBaseIDs() []string {
	seen := make(map[string]bool)
	var result []string
	for _, t := range st {
		if !seen[t.KnowledgeBaseID] {
			seen[t.KnowledgeBaseID] = true
			result = append(result, t.KnowledgeBaseID)
		}
	}
	return result
}

// GetKBTenantMap returns a map from knowledge base ID to tenant ID
func (st SearchTargets) GetKBTenantMap() map[string]uint64 {
	result := make(map[string]uint64)
	for _, t := range st {
		if t.KnowledgeBaseID != "" {
			result[t.KnowledgeBaseID] = t.TenantID
		}
	}
	return result
}

// GetTenantIDForKB returns the tenant ID for a given knowledge base ID
// Returns 0 if not found
func (st SearchTargets) GetTenantIDForKB(kbID string) uint64 {
	for _, t := range st {
		if t.KnowledgeBaseID == kbID {
			return t.TenantID
		}
	}
	return 0
}

// ContainsKB checks if the search targets contain a given knowledge base ID
func (st SearchTargets) ContainsKB(kbID string) bool {
	for _, t := range st {
		if t.KnowledgeBaseID == kbID {
			return true
		}
	}
	return false
}

// SearchResult represents the search result
type SearchResult struct {
	// ID
	ID string `gorm:"column:id"              json:"id"`
	// Content
	Content string `gorm:"column:content"         json:"content"`
	// Knowledge ID
	KnowledgeID string `gorm:"column:knowledge_id"    json:"knowledge_id"`
	// Chunk index
	ChunkIndex int `gorm:"column:chunk_index"     json:"chunk_index"`
	// Knowledge title
	KnowledgeTitle string `gorm:"column:knowledge_title" json:"knowledge_title"`
	// Start at
	StartAt int `gorm:"column:start_at"        json:"start_at"`
	// End at
	EndAt int `gorm:"column:end_at"          json:"end_at"`
	// Seq
	Seq int `gorm:"column:seq"             json:"seq"`
	// Score
	Score float64 `                              json:"score"`
	// Match type
	MatchType MatchType `                              json:"match_type"`
	// SubChunkIndex
	SubChunkID []string `                              json:"sub_chunk_id"`
	// Metadata
	Metadata map[string]string `                              json:"metadata"`

	// Chunk 类型
	ChunkType string `json:"chunk_type"`
	// 父 Chunk ID
	ParentChunkID string `json:"parent_chunk_id"`
	// 图片信息 (JSON 格式)
	ImageInfo string `json:"image_info"`

	// Knowledge file name
	// Used for file type knowledge, contains the original file name
	KnowledgeFilename string `json:"knowledge_filename"`

	// Knowledge source
	// Used to indicate the source of the knowledge, such as "url"
	KnowledgeSource string `json:"knowledge_source"`

	// KnowledgeChannel indicates through which channel the knowledge was ingested (web, api, wechat, etc.)
	KnowledgeChannel string `json:"knowledge_channel"`

	// ChunkMetadata stores chunk-level metadata (e.g., generated questions)
	ChunkMetadata JSON `json:"chunk_metadata,omitempty"`

	// MatchedContent is the actual content that was matched in vector search
	// For FAQ: this is the matched question text (standard or similar question)
	MatchedContent string `json:"matched_content,omitempty"`

	// KnowledgeDescription is the description of the knowledge document
	KnowledgeDescription string `json:"knowledge_description,omitempty"`

	// KnowledgeBaseID is the ID of the knowledge base this result belongs to
	KnowledgeBaseID string `json:"knowledge_base_id,omitempty"`
}

// SearchParams represents the search parameters
type SearchParams struct {
	QueryText            string    `json:"query_text"`
	QueryEmbedding       []float32 `json:"query_embedding,omitempty"`
	VectorThreshold      float64   `json:"vector_threshold"`
	KeywordThreshold     float64   `json:"keyword_threshold"`
	MatchCount           int       `json:"match_count"`
	DisableKeywordsMatch bool      `json:"disable_keywords_match"`
	DisableVectorMatch   bool      `json:"disable_vector_match"`
	KnowledgeIDs         []string  `json:"knowledge_ids"`
	TagIDs               []string  `json:"tag_ids"` // 传任意层级的 id_knowledge_tag 都能命中（Milvus ARRAY_CONTAINS_ANY）
	OnlyRecommended      bool      `json:"only_recommended"`
	// KnowledgeBaseIDs overrides the single KB ID passed to HybridSearch,
	// allowing a single retrieval call to span multiple KBs that share the
	// same embedding model. When empty, HybridSearch uses its own id parameter.
	KnowledgeBaseIDs []string `json:"knowledge_base_ids,omitempty"`
	// SkipContextEnrichment skips fetching parent, nearby, and relation chunks
	// in processSearchResults. Used by the chat pipeline where context assembly
	// is handled separately in the merge stage.
	SkipContextEnrichment bool `json:"skip_context_enrichment,omitempty"`
}

// ChunksSearchRequest is the request body for POST /knowledge-chunks/search.
// It is a flatter, KB-id-not-in-path variant of SearchParams used by the
// three-tier KB architecture so callers can search across multiple KBs in a
// single request without picking a path-level id.
type ChunksSearchRequest struct {
	Query            string   `json:"query"`
	KnowledgeBaseIDs []string `json:"knowledge_base_ids"`
	TagIDs           []string `json:"tag_ids,omitempty"`
	KnowledgeIDs     []string `json:"knowledge_ids,omitempty"`
	TopK             int      `json:"top_k,omitempty"`
	// UseBM25 controls whether the keyword (BM25) retrieval branch is enabled.
	// Pointer with omitempty is used so we can distinguish "not set" (default
	// true) from "explicitly disabled" (false). Vector retrieval is always on.
	UseBM25 *bool `json:"use_bm25,omitempty"`
	// VectorThreshold and KeywordThreshold are optional pass-through controls
	// that map directly to SearchParams of the same name.
	VectorThreshold  float64 `json:"vector_threshold,omitempty"`
	KeywordThreshold float64 `json:"keyword_threshold,omitempty"`
}

// ToSearchParams adapts the ChunksSearchRequest into the existing SearchParams
// shape consumed by knowledgeBaseService.HybridSearch. Defaults: TopK=10 when
// missing; UseBM25 defaults to true.
func (r *ChunksSearchRequest) ToSearchParams() SearchParams {
	matchCount := r.TopK
	if matchCount <= 0 {
		matchCount = 10
	}
	disableKeywords := false
	if r.UseBM25 != nil && !*r.UseBM25 {
		disableKeywords = true
	}
	return SearchParams{
		QueryText:            r.Query,
		MatchCount:           matchCount,
		KnowledgeBaseIDs:     r.KnowledgeBaseIDs,
		KnowledgeIDs:         r.KnowledgeIDs,
		TagIDs:               r.TagIDs,
		VectorThreshold:      r.VectorThreshold,
		KeywordThreshold:     r.KeywordThreshold,
		DisableKeywordsMatch: disableKeywords,
	}
}

// Value implements the driver.Valuer interface, used to convert SearchResult to database value
func (c SearchResult) Value() (driver.Value, error) {
	return json.Marshal(c)
}

// Scan implements the sql.Scanner interface, used to convert database value to SearchResult
func (c *SearchResult) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	b, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(b, c)
}

// Pagination represents the pagination parameters
type Pagination struct {
	// Page
	Page int `form:"page"      json:"page"      binding:"omitempty,min=1"`
	// Page size
	PageSize int `form:"page_size" json:"page_size" binding:"omitempty,min=1,max=1000"`
}

// GetPage gets the page number, default is 1
func (p *Pagination) GetPage() int {
	if p.Page < 1 {
		return 1
	}
	return p.Page
}

// GetPageSize gets the page size, default is 20
func (p *Pagination) GetPageSize() int {
	if p.PageSize < 1 {
		return 20
	}
	if p.PageSize > 1000 {
		return 1000
	}
	return p.PageSize
}

// Offset gets the offset for database query
func (p *Pagination) Offset() int {
	return (p.GetPage() - 1) * p.GetPageSize()
}

// Limit gets the limit for database query
func (p *Pagination) Limit() int {
	return p.GetPageSize()
}

// PageResult represents the pagination query result
type PageResult struct {
	Total    int64       `json:"total"`     // Total number of records
	Page     int         `json:"page"`      // Current page number
	PageSize int         `json:"page_size"` // Page size
	Data     interface{} `json:"data"`      // Data
}

// NewPageResult creates a new pagination result
func NewPageResult(total int64, page *Pagination, data interface{}) *PageResult {
	return &PageResult{
		Total:    total,
		Page:     page.GetPage(),
		PageSize: page.GetPageSize(),
		Data:     data,
	}
}
