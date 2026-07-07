package types

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// TagPathSeparator is the delimiter used in the materialized tag path.
// Tag names must not contain this character; see ValidateTagName.
const TagPathSeparator = "/"

// TagMaxDepth is the maximum allowed depth for a tag node (root depth == 0).
// Keep in sync with the VARCHAR(1024) length of knowledge_tags.path.
const TagMaxDepth = 6

// KnowledgeTag represents a tag node in a knowledge base's tag tree.
// Tags are scoped by knowledge base and owner, used to categorize
// Knowledge (documents) and FAQ Chunks.
type KnowledgeTag struct {
	// Unique identifier of the tag (PK column: id_knowledge_tag)
	ID string `json:"id"                gorm:"column:id_knowledge_tag;type:varchar(36);primaryKey"`
	// Knowledge base ID that this tag belongs to (FK column: id_knowledge_base)
	KnowledgeBaseID string `json:"knowledge_base_id" gorm:"column:id_knowledge_base;type:varchar(64);not null;index"`
	// Owner identifier
	Owner string `json:"owner"             gorm:"type:varchar(64);not null;index"`
	// ParentID is the parent tag id; empty for root-level tags. (column: parent_id_knowledge_tag)
	ParentID string `json:"parent_id"         gorm:"column:parent_id_knowledge_tag;type:varchar(36);index"`
	// Sort order within the same parent
	Sort int `json:"sort"              gorm:"type:int4;not null"`
	// Tag name, unique among sibling tags sharing the same parent.
	Name string `json:"name"              gorm:"type:varchar(255);not null"`
	// Creation time
	CreatedAt time.Time `json:"created_at"`
	// Last updated time
	UpdatedAt time.Time `json:"updated_at"`
	// Soft delete time
	DeletedAt gorm.DeletedAt `json:"deleted_at"        gorm:"index"`
}

// TableName overrides GORM's default plural table name.
func (KnowledgeTag) TableName() string {
	return "knowledge_tag"
}

// KnowledgeTagWithStats represents tag information along with usage statistics.
type KnowledgeTagWithStats struct {
	KnowledgeTag
	KnowledgeCount int64 `json:"knowledge_count"`
	ChunkCount     int64 `json:"chunk_count"`
}

// KnowledgeTagTreeNode represents a tag with its children, used for tree responses.
type KnowledgeTagTreeNode struct {
	KnowledgeTagWithStats
	Children []*KnowledgeTagTreeNode `json:"children,omitempty"`
}

// TagReferenceCounts holds the reference counts for a tag.
type TagReferenceCounts struct {
	KnowledgeCount int64
	ChunkCount     int64
}

// ValidateTagName checks whether the provided tag name is allowed.
// Rule: non-empty after trimming, no TagPathSeparator inside.
func ValidateTagName(name string) bool {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return false
	}
	return !strings.Contains(trimmed, TagPathSeparator)
}

// BuildTagPath builds the materialized path given the parent path and the node name.
// The parent path can be empty (root). Name is assumed pre-validated.
func BuildTagPath(parentPath, name string) string {
	name = strings.TrimSpace(name)
	if parentPath == "" {
		return TagPathSeparator + name
	}
	return strings.TrimRight(parentPath, TagPathSeparator) + TagPathSeparator + name
}

// KnowledgeTagShare represents a tag sharing record (who shared a tag to which group).
type KnowledgeTagShare struct {
	// Unique identifier (PK column: id_knowledge_tag_share)
	ID string `json:"id"                     gorm:"column:id_knowledge_tag_share;type:varchar(36);primaryKey"`
	// The tag being shared (FK column: id_knowledge_tag)
	KnowledgeTagID string `json:"knowledge_tag_id"       gorm:"column:id_knowledge_tag;type:varchar(36);index"`
	// Target group key (e.g. org ID)
	GroupKey string `json:"group_key"              gorm:"type:varchar(36);index"`
	// User who initiated the share
	SharedByUserID string `json:"shared_by_user_id"      gorm:"type:varchar(36)"`
	// Permission level: viewer, editor, admin
	Permission string `json:"permission"             gorm:"type:varchar(32);default:'viewer'"`
	// Creation time
	CreatedAt time.Time `json:"created_at"`
	// Last updated time
	UpdatedAt time.Time `json:"updated_at"`
	// Soft delete time
	DeletedAt gorm.DeletedAt `json:"deleted_at"             gorm:"index"`
}

// TableName overrides GORM's default table name.
func (KnowledgeTagShare) TableName() string {
	return "knowledge_tag_share"
}

// BeforeCreate hook generates a UUID for new KnowledgeTagShare entities.
func (s *KnowledgeTagShare) BeforeCreate(tx *gorm.DB) error {
	if s.ID == "" {
		s.ID = uuid.New().String()
	}
	return nil
}
