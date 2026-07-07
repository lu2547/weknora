package milvus

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
)

// Collection name constants for the three-tier KB architecture.
// Personal and Public KBs share a single collection each;
// Enterprise KBs each get a dedicated collection named by convention.
const (
	PersonalCollectionName = "personal_knowledge_base"
	PublicCollectionName   = "public_knowledge_base"
	SummaryCollectionName  = "summary_knowledge_base"
)

// ErrUnknownCategory is returned when a KB has an unrecognized category.
var ErrUnknownCategory = errors.New("unknown knowledge base category")

// CollectionResolver determines which Milvus collection to use for a given KnowledgeBase.
type CollectionResolver struct{}

// NewCollectionResolver returns a new CollectionResolver instance.
func NewCollectionResolver() *CollectionResolver {
	return &CollectionResolver{}
}

// EmbeddingCollection returns the embedding collection name for the given KB.
func (r *CollectionResolver) EmbeddingCollection(kb *types.KnowledgeBase) (string, error) {
	if kb == nil {
		return "", errors.New("knowledge base is nil")
	}
	switch kb.Category {
	case types.KnowledgeBaseCategoryPersonal:
		return PersonalCollectionName, nil
	case types.KnowledgeBaseCategoryPublic:
		return PublicCollectionName, nil
	case types.KnowledgeBaseCategoryEnterprise:
		return EnterpriseCollectionName(kb), nil
	default:
		return "", fmt.Errorf("%w: %q", ErrUnknownCategory, kb.Category)
	}
}

// SummaryCollection returns the summary collection name (always the global one).
func (r *CollectionResolver) SummaryCollection(_ *types.KnowledgeBase) string {
	return SummaryCollectionName
}

// KBCollectionInfo is the minimal metadata needed by CollectionResolver to
// route a knowledge base to its embedding collection. The Milvus repository
// caches this struct keyed by KB ID so it does not have to carry full
// *types.KnowledgeBase pointers everywhere.
type KBCollectionInfo struct {
	ID              string
	Category        string
	CreatedAtUnixMs int64
}

// FromKnowledgeBase extracts the minimal collection metadata from a KB.
func FromKnowledgeBase(kb *types.KnowledgeBase) KBCollectionInfo {
	if kb == nil {
		return KBCollectionInfo{}
	}
	return KBCollectionInfo{
		ID:              kb.ID,
		Category:        kb.Category,
		CreatedAtUnixMs: kb.CreatedAt.UnixMilli(),
	}
}

// EmbeddingCollectionByMeta resolves the embedding collection name from the
// minimal KB metadata. Used by milvusRepository's internal cache lookups so
// that cache entries do not need to hold full KnowledgeBase pointers.
func EmbeddingCollectionByMeta(info KBCollectionInfo) (string, error) {
	switch info.Category {
	case types.KnowledgeBaseCategoryPersonal:
		return PersonalCollectionName, nil
	case types.KnowledgeBaseCategoryPublic:
		return PublicCollectionName, nil
	case types.KnowledgeBaseCategoryEnterprise:
		return enterpriseCollectionNameFromMeta(info), nil
	default:
		return "", fmt.Errorf("%w: %q", ErrUnknownCategory, info.Category)
	}
}

// enterpriseCollectionNameFromMeta is the metadata-only twin of
// EnterpriseCollectionName. Both produce identical names for the same KB.
// Convention: enterprise_<lower(knowledge_base_id)> (one collection per KB,
// using the full KB ID lowercased; no sanitize, no unix_ms prefix, no truncation).
// Caller (KB creation) MUST guarantee KB ID only contains characters legal in a
// Milvus collection name (letters / digits / underscore; first char a letter or underscore).
func enterpriseCollectionNameFromMeta(info KBCollectionInfo) string {
	return fmt.Sprintf("enterprise_%s", strings.ToLower(info.ID))
}

// EnterpriseCollectionName generates the collection name for an enterprise KB
// using the convention: enterprise_<lower(knowledge_base_id)>.
func EnterpriseCollectionName(kb *types.KnowledgeBase) string {
	return enterpriseCollectionNameFromMeta(FromKnowledgeBase(kb))
}

// GroupByCollection groups a slice of KnowledgeBases by their target embedding collection.
// Returns a map of collection_name -> []kb_id for batch retrieval.
func (r *CollectionResolver) GroupByCollection(kbs []*types.KnowledgeBase) (map[string][]string, error) {
	result := make(map[string][]string)
	for _, kb := range kbs {
		col, err := r.EmbeddingCollection(kb)
		if err != nil {
			return nil, fmt.Errorf("resolve collection for KB %s: %w", kb.ID, err)
		}
		result[col] = append(result[col], kb.ID)
	}
	return result, nil
}
