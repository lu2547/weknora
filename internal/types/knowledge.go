package types

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	// KnowledgeTypeManual represents the manual knowledge type
	KnowledgeTypeManual = "manual"
	// KnowledgeTypeFAQ represents the FAQ knowledge type
	KnowledgeTypeFAQ = "faq"
)

// Knowledge parse status constants
const (
	// ParseStatusPending indicates the knowledge is waiting to be processed
	ParseStatusPending = "pending"
	// ParseStatusProcessing indicates the knowledge is being processed
	ParseStatusProcessing = "processing"
	// ParseStatusCompleted indicates the knowledge has been processed successfully
	ParseStatusCompleted = "completed"
	// ParseStatusFailed indicates the knowledge processing failed
	ParseStatusFailed = "failed"
	// ParseStatusDeleting indicates the knowledge is being deleted (used to prevent async task conflicts)
	ParseStatusDeleting = "deleting"
)

// Summary status constants for async summary generation
const (
	// SummaryStatusNone indicates no summary task is needed
	SummaryStatusNone = "none"
	// SummaryStatusPending indicates the summary task is waiting to be processed
	SummaryStatusPending = "pending"
	// SummaryStatusProcessing indicates the summary is being generated
	SummaryStatusProcessing = "processing"
	// SummaryStatusCompleted indicates the summary has been generated successfully
	SummaryStatusCompleted = "completed"
	// SummaryStatusFailed indicates the summary generation failed
	SummaryStatusFailed = "failed"
)

// ManualKnowledgeFormat represents the format of the manual knowledge
const (
	ManualKnowledgeFormatMarkdown = "markdown"
	ManualKnowledgeStatusDraft    = "draft"
	ManualKnowledgeStatusPublish  = "publish"
)

// EnableStatus constants — DDL semantics: '0' = enabled, '1' = disabled (inverted from common convention)
const (
	EnableStatusEnabled  = "0" // 0 = 启用
	EnableStatusDisabled = "1" // 1 = 禁用
)

// Channel constants identify the ingestion source (used by IM/datasource modules).
const (
	ChannelWechat   = "wechat"
	ChannelWecom    = "wecom"
	ChannelFeishu   = "feishu"
	ChannelDingtalk = "dingtalk"
	ChannelSlack    = "slack"
	ChannelIM       = "im"
)

// Knowledge represents a knowledge entity in the system.
// It contains metadata about the knowledge source, its processing status,
// and references to the physical file if applicable.
type Knowledge struct {
	// Unique identifier of the knowledge (PK column: id_knowledge)
	ID string `json:"id"                 gorm:"column:id_knowledge;type:varchar(36);primaryKey"`
	// ID of the knowledge base (FK column: id_knowledge_base)
	KnowledgeBaseID string `json:"knowledge_base_id"  gorm:"column:id_knowledge_base"`
	// Optional tag ID for categorization within a knowledge base
	TagID string `json:"tag_id"             gorm:"type:varchar(36);index"`
	// Type of the knowledge
	Type string `json:"type"`
	// Title of the knowledge
	Title string `json:"title"`
	// Description of the knowledge
	Description string `json:"description"`
	// Parse status of the knowledge
	ParseStatus string `json:"parse_status"`
	// Summary status for async summary generation
	SummaryStatus string `json:"summary_status"     gorm:"type:varchar(32);default:none"`
	// Enable status of the knowledge (DDL: '0'=enabled, '1'=disabled)
	EnableStatus string `json:"enable_status"      gorm:"type:char(1);default:'0'"`
	// ID of the embedding model (historical snapshot of which model was used)
	EmbeddingModelID string `json:"embedding_model_id"`
	// File name of the knowledge
	FileName string `json:"file_name"`
	// File type of the knowledge
	FileType string `json:"file_type"`
	// File size of the knowledge
	FileSize int64 `json:"file_size"`
	// File hash of the knowledge
	FileHash string `json:"file_hash"`
	// File path of the knowledge
	FilePath string `json:"file_path"`
	// Storage size of the knowledge
	StorageSize int64 `json:"storage_size"`
	// Milvus collection name (pre-computed, avoids runtime resolution)
	CollectionName string `json:"collection_name"    gorm:"type:varchar(128)"`
	// Creation time of the knowledge
	CreatedAt time.Time `json:"created_at"`
	// Last updated time of the knowledge
	UpdatedAt time.Time `json:"updated_at"`
	// Processed time of the knowledge
	ProcessedAt *time.Time `json:"processed_at"`
	// Error message of the knowledge
	ErrorMessage string `json:"error_message"`
	// Deletion time of the knowledge
	DeletedAt gorm.DeletedAt `json:"deleted_at"         gorm:"index"`
	// Metadata stores JSON metadata (used by manual knowledge for content storage)
	Metadata JSON `json:"metadata,omitempty"  gorm:"type:jsonb"`
}

// TableName overrides GORM's default plural table name.
func (Knowledge) TableName() string {
	return "knowledge"
}

// IsEnabled returns the semantic boolean for enable_status (0=enabled→true, 1=disabled→false).
func (k *Knowledge) IsEnabled() bool {
	return k != nil && k.EnableStatus != EnableStatusDisabled
}

// IsManual returns true if this is a manual knowledge entry.
func (k *Knowledge) IsManual() bool {
	return k != nil && k.Type == KnowledgeTypeManual
}

// ManualKnowledgeMetadata represents the metadata structure for manual knowledge.
type ManualKnowledgeMetadata struct {
	Content string `json:"content"`
	Status  string `json:"status"`
	Version int    `json:"version"`
}

// NewManualKnowledgeMetadata creates a new ManualKnowledgeMetadata.
func NewManualKnowledgeMetadata(content, status string, version int) *ManualKnowledgeMetadata {
	return &ManualKnowledgeMetadata{
		Content: content,
		Status:  status,
		Version: version,
	}
}

// ManualMetadata decodes the Metadata JSON into ManualKnowledgeMetadata.
func (k *Knowledge) ManualMetadata() (*ManualKnowledgeMetadata, error) {
	if k == nil || len(k.Metadata) == 0 {
		return nil, nil
	}
	var m ManualKnowledgeMetadata
	if err := json.Unmarshal(k.Metadata, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// SetManualMetadata encodes ManualKnowledgeMetadata into the Metadata JSON field.
func (k *Knowledge) SetManualMetadata(m *ManualKnowledgeMetadata) error {
	if m == nil {
		k.Metadata = nil
		return nil
	}
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	k.Metadata = JSON(data)
	return nil
}

// EnsureManualDefaults sets default values for manual knowledge entries.
func (k *Knowledge) EnsureManualDefaults() {
	if k == nil {
		return
	}
	if k.FileType == "" {
		k.FileType = KnowledgeTypeManual
	}
}

// SetLastFAQImportResult stores the FAQ import result in the Knowledge Metadata field.
func (k *Knowledge) SetLastFAQImportResult(result *FAQImportResult) error {
	if result == nil {
		k.Metadata = nil
		return nil
	}
	// Wrap under a key so Metadata can hold other data too
	wrapper := map[string]interface{}{"last_faq_import_result": result}
	data, err := json.Marshal(wrapper)
	if err != nil {
		return err
	}
	k.Metadata = JSON(data)
	return nil
}

// GetLastFAQImportResult reads the FAQ import result from the Knowledge Metadata field.
func (k *Knowledge) GetLastFAQImportResult() (*FAQImportResult, error) {
	if k == nil || len(k.Metadata) == 0 {
		return nil, nil
	}
	var wrapper map[string]json.RawMessage
	if err := json.Unmarshal(k.Metadata, &wrapper); err != nil {
		return nil, err
	}
	raw, ok := wrapper["last_faq_import_result"]
	if !ok || len(raw) == 0 {
		return nil, nil
	}
	var result FAQImportResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// BeforeCreate hook generates a UUID for new Knowledge entities before they are created.
func (k *Knowledge) BeforeCreate(tx *gorm.DB) (err error) {
	if k.ID == "" {
		k.ID = uuid.New().String()
	}
	return nil
}

// KnowledgeSearchScope defines a knowledge_base_id scope for knowledge search.
type KnowledgeSearchScope struct {
	KBID string
}

// ManualKnowledgePayload is the request body for creating/updating manual knowledge.
type ManualKnowledgePayload struct {
	Title   string `json:"title"`
	Content string `json:"content"`
	Status  string `json:"status"`
	TagID   string `json:"tag_id"`
}

// KnowledgeCheckParams defines parameters used to check if knowledge already exists.
type KnowledgeCheckParams struct {
	// File parameters
	FileName string
	FileSize int64
	FileHash string
	// URL parameters
	URL string
	// Text passage parameters
	Passages []string
	// Knowledge type
	Type string
}
