package repository

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
)

// knowledgeTagShareRepository persists tag share records.
type knowledgeTagShareRepository struct {
	db *gorm.DB
}

// NewKnowledgeTagShareRepository creates a new tag share repository.
func NewKnowledgeTagShareRepository(db *gorm.DB) interfaces.KnowledgeTagShareRepository {
	return &knowledgeTagShareRepository{db: db}
}

// Create inserts a new share record. Caller is responsible for de-duplication
// via GetByTagAndGroup.
func (r *knowledgeTagShareRepository) Create(ctx context.Context, share *types.KnowledgeTagShare) error {
	return r.db.WithContext(ctx).Create(share).Error
}

// GetByID returns a share by its primary key.
func (r *knowledgeTagShareRepository) GetByID(ctx context.Context, id string) (*types.KnowledgeTagShare, error) {
	var share types.KnowledgeTagShare
	if err := r.db.WithContext(ctx).
		Where("id_knowledge_tag_share = ?", id).
		First(&share).Error; err != nil {
		return nil, err
	}
	return &share, nil
}

// GetByTagAndGroup returns the existing share between a tag and a group, if any.
func (r *knowledgeTagShareRepository) GetByTagAndGroup(ctx context.Context, tagID string, groupKey string) (*types.KnowledgeTagShare, error) {
	var share types.KnowledgeTagShare
	if err := r.db.WithContext(ctx).
		Where("id_knowledge_tag = ? AND group_key = ?", tagID, groupKey).
		First(&share).Error; err != nil {
		return nil, err
	}
	return &share, nil
}

// Update updates a share record (typically permission).
func (r *knowledgeTagShareRepository) Update(ctx context.Context, share *types.KnowledgeTagShare) error {
	return r.db.WithContext(ctx).Save(share).Error
}

// Delete removes a share record by ID.
func (r *knowledgeTagShareRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).
		Where("id_knowledge_tag_share = ?", id).
		Delete(&types.KnowledgeTagShare{}).Error
}

// DeleteByTagID removes all share records for a tag.
func (r *knowledgeTagShareRepository) DeleteByTagID(ctx context.Context, tagID string) error {
	return r.db.WithContext(ctx).
		Where("id_knowledge_tag = ?", tagID).
		Delete(&types.KnowledgeTagShare{}).Error
}

// ListByTagID lists all share records of a tag.
func (r *knowledgeTagShareRepository) ListByTagID(ctx context.Context, tagID string) ([]*types.KnowledgeTagShare, error) {
	var shares []*types.KnowledgeTagShare
	if err := r.db.WithContext(ctx).
		Where("id_knowledge_tag = ?", tagID).
		Order("created_at DESC").
		Find(&shares).Error; err != nil {
		return nil, err
	}
	return shares, nil
}

// ListByGroupKey lists all shares granted to a group (e.g. organization id).
func (r *knowledgeTagShareRepository) ListByGroupKey(ctx context.Context, groupKey string) ([]*types.KnowledgeTagShare, error) {
	var shares []*types.KnowledgeTagShare
	if err := r.db.WithContext(ctx).
		Where("group_key = ?", groupKey).
		Order("created_at DESC").
		Find(&shares).Error; err != nil {
		return nil, err
	}
	return shares, nil
}
