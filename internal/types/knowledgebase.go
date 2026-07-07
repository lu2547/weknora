package types

import (
	"database/sql/driver"
	"encoding/json"
	"strings"
	"time"

	"gorm.io/gorm"
)

// KnowledgeBaseType represents the type of the knowledge base
const (
	// KnowledgeBaseTypeDocument represents the document knowledge base type
	KnowledgeBaseTypeDocument = "document"
	KnowledgeBaseTypeFAQ      = "faq"
)

// KnowledgeBaseCategory represents the category/tier of the knowledge base
const (
	KnowledgeBaseCategoryPersonal   = "personal"
	KnowledgeBaseCategoryPublic     = "public"
	KnowledgeBaseCategoryEnterprise = "enterprise"
)

// FAQIndexMode represents the FAQ index mode: only index questions or index questions and answers
type FAQIndexMode string

const (
	// FAQIndexModeQuestionOnly only index questions and similar questions
	FAQIndexModeQuestionOnly FAQIndexMode = "question_only"
	// FAQIndexModeQuestionAnswer index questions and answers together
	FAQIndexModeQuestionAnswer FAQIndexMode = "question_answer"
)

// FAQQuestionIndexMode represents the FAQ question index mode: index together or index separately
type FAQQuestionIndexMode string

const (
	// FAQQuestionIndexModeCombined index questions and similar questions together
	FAQQuestionIndexModeCombined FAQQuestionIndexMode = "combined"
	// FAQQuestionIndexModeSeparate index questions and similar questions separately
	FAQQuestionIndexModeSeparate FAQQuestionIndexMode = "separate"
)

// KnowledgeBase represents a knowledge base entity
type KnowledgeBase struct {
	// Unique identifier of the knowledge base (PK column: id_knowledge_base)
	ID string `yaml:"id"                      json:"id"                      gorm:"column:id_knowledge_base;type:varchar(36);primaryKey"`
	// Category of the knowledge base: personal, public, enterprise
	Category string `yaml:"category"                json:"category"                gorm:"type:varchar(32);default:'personal'"`
	// Name of the knowledge base
	Name string `yaml:"name"                    json:"name"`
	// Type of the knowledge base (document, faq, etc.)
	Type string `yaml:"type"                    json:"type"                    gorm:"type:varchar(32);default:'document'"`
	// Owner identifier (user/org who owns this KB)
	Owner string `yaml:"owner"                   json:"owner"                   gorm:"type:varchar(64);not null"`
	// Description of the knowledge base
	Description string `yaml:"description"             json:"description"`
	// Chunking configuration
	ChunkingConfig ChunkingConfig `yaml:"chunking_config"         json:"chunking_config"         gorm:"type:jsonb"`
	// FAQConfig stores FAQ specific configuration such as indexing strategy
	FAQConfig *FAQConfig `yaml:"faq_config"              json:"faq_config"              gorm:"column:faq_config;type:jsonb"`
	// QuestionGenerationConfig stores question generation configuration for document knowledge bases
	QuestionGenerationConfig *QuestionGenerationConfig `yaml:"question_generation_config" json:"question_generation_config" gorm:"column:question_generation_config;type:jsonb"`
	// Whether this knowledge base is pinned to the top of the list
	IsPinned bool `yaml:"is_pinned"               json:"is_pinned"               gorm:"default:false"`
	// Time when the knowledge base was pinned (nil if not pinned)
	PinnedAt *time.Time `yaml:"pinned_at"               json:"pinned_at"`
	// Creation time of the knowledge base
	CreatedAt time.Time `yaml:"created_at"              json:"created_at"`
	// Last updated time of the knowledge base
	UpdatedAt time.Time `yaml:"updated_at"              json:"updated_at"`
	// Deletion time of the knowledge base
	DeletedAt gorm.DeletedAt `yaml:"deleted_at"              json:"deleted_at"              gorm:"index"`
	// ProcessingCount indicates the number of knowledge items being processed (for document type knowledge bases)
	ProcessingCount int64 `yaml:"processing_count"        json:"processing_count"        gorm:"-"`
	// KnowledgeCount indicates the number of knowledge items in this KB (computed, not stored)
	KnowledgeCount int64 `yaml:"knowledge_count"         json:"knowledge_count"         gorm:"-"`
	// ChunkCount indicates the number of chunks in this KB (computed, not stored)
	ChunkCount int64 `yaml:"chunk_count"             json:"chunk_count"             gorm:"-"`
	// ShareCount indicates the number of organizations this knowledge base is shared with (not stored in database)
	ShareCount int64 `yaml:"share_count"             json:"share_count"             gorm:"-"`
}

// KnowledgeBaseConfig represents the knowledge base configuration
type KnowledgeBaseConfig struct {
	// Chunking configuration
	ChunkingConfig ChunkingConfig `yaml:"chunking_config"         json:"chunking_config"`
	// FAQ configuration (only for FAQ type knowledge bases)
	FAQConfig *FAQConfig `yaml:"faq_config"              json:"faq_config"`
}

// ParserEngineRule maps a set of file types to a specific parser engine.
type ParserEngineRule struct {
	FileTypes []string `yaml:"file_types" json:"file_types"`
	Engine    string   `yaml:"engine"     json:"engine"`
}

// ChunkingConfig represents the document splitting configuration
type ChunkingConfig struct {
	// Chunk size
	ChunkSize int `yaml:"chunk_size"    json:"chunk_size"`
	// Chunk overlap
	ChunkOverlap int `yaml:"chunk_overlap" json:"chunk_overlap"`
	// Separators
	Separators []string `yaml:"separators"    json:"separators"`
	// EnableMultimodal (deprecated, kept for backward compatibility with old data)
	EnableMultimodal bool `yaml:"enable_multimodal,omitempty" json:"enable_multimodal,omitempty"`
	// ParserEngineRules configures which parser engine to use for each file type.
	// When empty, the builtin engine is used for all types.
	ParserEngineRules []ParserEngineRule `yaml:"parser_engine_rules,omitempty" json:"parser_engine_rules,omitempty"`
	// EnableParentChild enables two-level parent-child chunking strategy.
	// When enabled, large parent chunks provide context while small child chunks
	// are used for vector matching. Retrieval matches on child but returns parent content.
	EnableParentChild bool `yaml:"enable_parent_child,omitempty" json:"enable_parent_child,omitempty"`
	// ParentChunkSize is the size of parent chunks (default: 4096).
	// Only used when EnableParentChild is true.
	ParentChunkSize int `yaml:"parent_chunk_size,omitempty" json:"parent_chunk_size,omitempty"`
	// ChildChunkSize is the size of child chunks used for embedding (default: 384).
	// Only used when EnableParentChild is true.
	ChildChunkSize int `yaml:"child_chunk_size,omitempty" json:"child_chunk_size,omitempty"`
}

// ResolveParserEngine returns the engine name for the given file type
// based on the configured rules. Returns empty string (builtin) when
// no rule matches.
func (c ChunkingConfig) ResolveParserEngine(fileType string) string {
	for _, rule := range c.ParserEngineRules {
		for _, ft := range rule.FileTypes {
			if ft == fileType {
				return rule.Engine
			}
		}
	}
	return ""
}

// Value implements the driver.Valuer interface, used to convert ChunkingConfig to database value
func (c ChunkingConfig) Value() (driver.Value, error) {
	return json.Marshal(c)
}

// Scan implements the sql.Scanner interface, used to convert database value to ChunkingConfig
func (c *ChunkingConfig) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	b, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(b, c)
}

// QuestionGenerationConfig represents the question generation configuration for document knowledge bases
// When enabled, the system will use LLM to generate questions for each chunk during document parsing
// These generated questions will be indexed separately to improve recall
type QuestionGenerationConfig struct {
	Enabled bool `yaml:"enabled"  json:"enabled"`
	// Number of questions to generate per chunk (default: 3, max: 10)
	QuestionCount int `yaml:"question_count" json:"question_count"`
}

// Value implements the driver.Valuer interface
func (c QuestionGenerationConfig) Value() (driver.Value, error) {
	return json.Marshal(c)
}

// Scan implements the sql.Scanner interface
func (c *QuestionGenerationConfig) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	b, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(b, c)
}

// FAQConfig 存储 FAQ 知识库的特有配置
type FAQConfig struct {
	IndexMode         FAQIndexMode         `yaml:"index_mode"          json:"index_mode"`
	QuestionIndexMode FAQQuestionIndexMode `yaml:"question_index_mode" json:"question_index_mode"`
}

// Value implements driver.Valuer
func (f FAQConfig) Value() (driver.Value, error) {
	return json.Marshal(f)
}

// Scan implements sql.Scanner
func (f *FAQConfig) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	b, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(b, f)
}

// EnsureDefaults 确保类型与配置具备默认值
func (kb *KnowledgeBase) EnsureDefaults() {
	if kb == nil {
		return
	}
	// 三级知识库类别空值兑底：保证 Milvus collection 路由能拿到合法 category。
	// 非法值在 service 层会被拒绝，这里只负责空值默认。
	if kb.Category == "" {
		kb.Category = KnowledgeBaseCategoryPersonal
	}
	if kb.Type == "" {
		kb.Type = KnowledgeBaseTypeDocument
	}
	if kb.Type != KnowledgeBaseTypeFAQ {
		kb.FAQConfig = nil
		return
	}
	if kb.FAQConfig == nil {
		kb.FAQConfig = &FAQConfig{
			IndexMode:         FAQIndexModeQuestionAnswer,
			QuestionIndexMode: FAQQuestionIndexModeCombined,
		}
		return
	}
	if kb.FAQConfig.IndexMode == "" {
		kb.FAQConfig.IndexMode = FAQIndexModeQuestionAnswer
	}
	if kb.FAQConfig.QuestionIndexMode == "" {
		kb.FAQConfig.QuestionIndexMode = FAQQuestionIndexModeCombined
	}
}

// TableName overrides GORM's default plural table name.
func (KnowledgeBase) TableName() string {
	return "knowledge_base"
}

// IsEnterprise returns true if this KB uses a dedicated enterprise collection.
func (kb *KnowledgeBase) IsEnterprise() bool {
	return kb != nil && kb.Category == KnowledgeBaseCategoryEnterprise
}

// ResolveCollectionName 返回该知识库在 Milvus 中的 collection 名。
// 双生于 milvus 包中的同名函数，面向 service 层提供不引入 milvus 包依赖的调用。
// 未知/非法 category 返回空字符串，调用方需自行判空以避免写入后与 milvus 实际写入路径不一致。
func (kb *KnowledgeBase) ResolveCollectionName() string {
	if kb == nil {
		return ""
	}
	switch kb.Category {
	case KnowledgeBaseCategoryPersonal:
		return "personal_knowledge_base"
	case KnowledgeBaseCategoryPublic:
		return "public_knowledge_base"
	case KnowledgeBaseCategoryEnterprise:
		return "enterprise_" + strings.ToLower(kb.ID)
	default:
		return ""
	}
}

// IsValidKnowledgeBaseCategory 判断 category 字符串是否为三级知识库合法枚举。
// 用于 service 层验收前端/外部调用者传入的值，避免非法字串写入数据库。
func IsValidKnowledgeBaseCategory(c string) bool {
	switch c {
	case KnowledgeBaseCategoryPersonal,
		KnowledgeBaseCategoryPublic,
		KnowledgeBaseCategoryEnterprise:
		return true
	}
	return false
}

// InferStorageFromFilePath infers the storage provider from a file path scheme.
// Returns "local", "minio", "cos", "tos", "s3" etc., or empty string if unknown.
func InferStorageFromFilePath(filePath string) string {
	if filePath == "" {
		return ""
	}
	// Check scheme-based paths like "local://...", "minio://..."
	schemes := []string{"local", "minio", "cos", "tos", "s3"}
	for _, s := range schemes {
		if strings.HasPrefix(filePath, s+"://") {
			return s
		}
	}
	// Check domain-based COS URLs
	if strings.Contains(filePath, ".cos.") && strings.Contains(filePath, ".myqcloud.com") {
		return "cos"
	}
	return ""
}

// ParseProviderScheme extracts the storage provider scheme from a URL.
// Returns "local", "minio", "cos", "tos", "s3" etc., or empty string if not a recognized scheme.
func ParseProviderScheme(url string) string {
	if url == "" {
		return ""
	}
	schemes := []string{"local", "minio", "cos", "tos", "s3"}
	for _, s := range schemes {
		if strings.HasPrefix(url, s+"://") {
			return s
		}
	}
	return ""
}
