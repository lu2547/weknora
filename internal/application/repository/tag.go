package repository

import (
	"context"
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
)

// knowledgeTagRepository is a repository for knowledge tags
type knowledgeTagRepository struct {
	db *gorm.DB
}

// NewKnowledgeTagRepository creates a new tag repository.
func NewKnowledgeTagRepository(db *gorm.DB) interfaces.KnowledgeTagRepository {
	return &knowledgeTagRepository{db: db}
}

// Create creates a new knowledge tag
func (r *knowledgeTagRepository) Create(ctx context.Context, tag *types.KnowledgeTag) error {
	return r.db.WithContext(ctx).Create(tag).Error
}

// Update updates a knowledge tag
func (r *knowledgeTagRepository) Update(ctx context.Context, tag *types.KnowledgeTag) error {
	return r.db.WithContext(ctx).Save(tag).Error
}

// GetByID gets a knowledge tag by ID
func (r *knowledgeTagRepository) GetByID(ctx context.Context, id string) (*types.KnowledgeTag, error) {
	var tag types.KnowledgeTag
	if err := r.db.WithContext(ctx).
		Where("id_knowledge_tag = ?", id).
		First(&tag).Error; err != nil {
		return nil, err
	}
	return &tag, nil
}

// GetByIDs retrieves multiple tags by their IDs in a single query
func (r *knowledgeTagRepository) GetByIDs(ctx context.Context, ids []string) ([]*types.KnowledgeTag, error) {
	if len(ids) == 0 {
		return []*types.KnowledgeTag{}, nil
	}
	var tags []*types.KnowledgeTag
	if err := r.db.WithContext(ctx).
		Where("id_knowledge_tag IN (?)", ids).
		Find(&tags).Error; err != nil {
		return nil, err
	}
	return tags, nil
}

// GetByName gets a knowledge tag by (kbID, parentID, name). An empty parentID means root.
func (r *knowledgeTagRepository) GetByName(ctx context.Context, kbID string, parentID string, name string) (*types.KnowledgeTag, error) {
	var tag types.KnowledgeTag
	q := r.db.WithContext(ctx).
		Where("id_knowledge_base = ? AND name = ?", kbID, name)
	if parentID == "" {
		q = q.Where("(parent_id_knowledge_tag IS NULL OR parent_id_knowledge_tag = '')")
	} else {
		q = q.Where("parent_id_knowledge_tag = ?", parentID)
	}
	if err := q.First(&tag).Error; err != nil {
		return nil, err
	}
	return &tag, nil
}

// ListByKB lists knowledge tags by knowledge base ID with pagination and optional keyword filtering.
func (r *knowledgeTagRepository) ListByKB(
	ctx context.Context,
	kbID string,
	page *types.Pagination,
	keyword string,
) ([]*types.KnowledgeTag, int64, error) {
	if page == nil {
		page = &types.Pagination{}
	}
	keyword = strings.TrimSpace(keyword)

	var total int64
	baseQuery := r.db.WithContext(ctx).Model(&types.KnowledgeTag{}).
		Where("id_knowledge_base = ?", kbID)
	if keyword != "" {
		escaped := escapeLikeKeyword(keyword)
		baseQuery = baseQuery.Where("name LIKE ?", "%"+escaped+"%")
	}

	if err := baseQuery.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	dataQuery := r.db.WithContext(ctx).
		Where("id_knowledge_base = ?", kbID)
	if keyword != "" {
		escaped := escapeLikeKeyword(keyword)
		dataQuery = dataQuery.Where("name LIKE ?", "%"+escaped+"%")
	}

	var tags []*types.KnowledgeTag
	if err := dataQuery.
		Order("sort ASC, created_at DESC").
		Offset(page.Offset()).
		Limit(page.Limit()).
		Find(&tags).Error; err != nil {
		return nil, 0, err
	}

	return tags, total, nil
}

// ListAllByKB returns every tag under a KB without pagination.
func (r *knowledgeTagRepository) ListAllByKB(
	ctx context.Context,
	kbID string,
) ([]*types.KnowledgeTag, error) {
	var tags []*types.KnowledgeTag
	if err := r.db.WithContext(ctx).
		Where("id_knowledge_base = ?", kbID).
		Order("sort ASC, created_at ASC").
		Find(&tags).Error; err != nil {
		return nil, err
	}
	return tags, nil
}

// ListChildren returns all direct children of a given parent tag.
func (r *knowledgeTagRepository) ListChildren(ctx context.Context, parentID string) ([]*types.KnowledgeTag, error) {
	var tags []*types.KnowledgeTag
	if err := r.db.WithContext(ctx).
		Where("parent_id_knowledge_tag = ?", parentID).
		Order("sort ASC, created_at ASC").
		Find(&tags).Error; err != nil {
		return nil, err
	}
	return tags, nil
}

// Delete deletes a knowledge tag
func (r *knowledgeTagRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).
		Where("id_knowledge_tag = ?", id).
		Delete(&types.KnowledgeTag{}).Error
}

// CountReferences returns the number of knowledges and chunks that reference this tag
func (r *knowledgeTagRepository) CountReferences(
	ctx context.Context,
	kbID string,
	tagID string,
) (knowledgeCount int64, chunkCount int64, err error) {
	if err = r.db.WithContext(ctx).
		Model(&types.Knowledge{}).
		Where("id_knowledge_base = ? AND tag_id = ?", kbID, tagID).
		Count(&knowledgeCount).Error; err != nil {
		return
	}
	if err = r.db.WithContext(ctx).
		Model(&types.Chunk{}).
		Where("id_knowledge_base = ? AND tag_id = ?", kbID, tagID).
		Count(&chunkCount).Error; err != nil {
		return
	}
	return
}

// tagCountResult is used to scan the result of batch count queries
type tagCountResult struct {
	TagID string `gorm:"column:tag_id"`
	Count int64  `gorm:"column:count"`
}

// BatchCountReferences returns the number of knowledges and chunks for multiple tags in a single query.
func (r *knowledgeTagRepository) BatchCountReferences(
	ctx context.Context,
	kbID string,
	tagIDs []string,
) (map[string]types.TagReferenceCounts, error) {
	result := make(map[string]types.TagReferenceCounts)
	if len(tagIDs) == 0 {
		return result, nil
	}

	// Initialize result with zero counts for all tagIDs
	for _, tagID := range tagIDs {
		result[tagID] = types.TagReferenceCounts{}
	}

	// Count knowledge references in a single query
	var knowledgeCounts []tagCountResult
	if err := r.db.WithContext(ctx).
		Model(&types.Knowledge{}).
		Select("tag_id, COUNT(*) as count").
		Where("id_knowledge_base = ? AND tag_id IN (?)", kbID, tagIDs).
		Group("tag_id").
		Find(&knowledgeCounts).Error; err != nil {
		return nil, err
	}
	for _, kc := range knowledgeCounts {
		counts := result[kc.TagID]
		counts.KnowledgeCount = kc.Count
		result[kc.TagID] = counts
	}

	// Count chunk references in a single query
	var chunkCounts []tagCountResult
	if err := r.db.WithContext(ctx).
		Model(&types.Chunk{}).
		Select("tag_id, COUNT(*) as count").
		Where("id_knowledge_base = ? AND tag_id IN (?)", kbID, tagIDs).
		Group("tag_id").
		Find(&chunkCounts).Error; err != nil {
		return nil, err
	}
	for _, cc := range chunkCounts {
		counts := result[cc.TagID]
		counts.ChunkCount = cc.Count
		result[cc.TagID] = counts
	}

	return result, nil
}

// DeleteUnusedTags deletes tags that are not referenced by any knowledge or chunk.
// Returns the number of deleted tags.
func (r *knowledgeTagRepository) DeleteUnusedTags(ctx context.Context, kbID string) (int64, error) {
	// Delete tags that have no references in both knowledge and chunk tables (excluding soft-deleted records)
	result := r.db.WithContext(ctx).
		Where("id_knowledge_base = ?", kbID).
		Where("id_knowledge_tag NOT IN (SELECT DISTINCT tag_id FROM knowledge WHERE id_knowledge_base = ? AND tag_id IS NOT NULL AND tag_id != '' AND deleted_at IS NULL)", kbID).
		Where("id_knowledge_tag NOT IN (SELECT DISTINCT tag_id FROM chunk WHERE id_knowledge_base = ? AND tag_id IS NOT NULL AND tag_id != '' AND deleted_at IS NULL)", kbID).
		Delete(&types.KnowledgeTag{})
	return result.RowsAffected, result.Error
}

// AncestorIDs 返回从根到 id 节点的祖先链 id 平铺列表（含自身，顺序 root→leaf）。
// 实现选型：逐层从子到根查 ParentID，最后反转。TagMaxDepth=6 废上限，迭代不会造成问题。
func (r *knowledgeTagRepository) AncestorIDs(ctx context.Context, id string) ([]string, error) {
	if id == "" {
		return nil, nil
	}
	cursor := id
	chain := make([]string, 0, types.TagMaxDepth+1)
	visited := make(map[string]struct{}, types.TagMaxDepth+1)
	for i := 0; i <= types.TagMaxDepth+1 && cursor != ""; i++ {
		if _, ok := visited[cursor]; ok {
			// 防止脱锰 parent_id 环
			break
		}
		var tag types.KnowledgeTag
		if err := r.db.WithContext(ctx).
			Select("id_knowledge_tag, parent_id_knowledge_tag").
			Where("id_knowledge_tag = ?", cursor).
			First(&tag).Error; err != nil {
			return nil, err
		}
		chain = append(chain, tag.ID)
		visited[cursor] = struct{}{}
		cursor = tag.ParentID
	}
	// reverse to root→leaf
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	return chain, nil
}
