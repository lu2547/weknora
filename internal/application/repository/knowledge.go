package repository

import (
	"context"
	"errors"
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
)

var ErrKnowledgeNotFound = errors.New("knowledge not found")

// escapeLikeKeyword escapes SQL LIKE wildcards (%, _) in a keyword
// so they are treated as literal characters.
func escapeLikeKeyword(keyword string) string {
	keyword = strings.ReplaceAll(keyword, `\`, `\\`)
	keyword = strings.ReplaceAll(keyword, "%", `\%`)
	keyword = strings.ReplaceAll(keyword, "_", `\_`)
	return keyword
}

// omitFieldsOnUpdate defines fields to omit when updating knowledge
var omitFieldsOnUpdate = []string{"DeletedAt"}

// knowledgeRepository implements knowledge base and knowledge repository interface
type knowledgeRepository struct {
	db *gorm.DB
}

// NewKnowledgeRepository creates a new knowledge repository
func NewKnowledgeRepository(db *gorm.DB) interfaces.KnowledgeRepository {
	return &knowledgeRepository{db: db}
}

// CreateKnowledge creates knowledge
func (r *knowledgeRepository) CreateKnowledge(ctx context.Context, knowledge *types.Knowledge) error {
	err := r.db.WithContext(ctx).Create(knowledge).Error
	return err
}

// GetKnowledgeByID gets knowledge by ID
func (r *knowledgeRepository) GetKnowledgeByID(ctx context.Context, id string) (*types.Knowledge, error) {
	var knowledge types.Knowledge
	if err := r.db.WithContext(ctx).Where("id_knowledge = ?", id).First(&knowledge).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrKnowledgeNotFound
		}
		return nil, err
	}
	return &knowledge, nil
}

// ListKnowledgeByKnowledgeBaseID lists all knowledge in a knowledge base
func (r *knowledgeRepository) ListKnowledgeByKnowledgeBaseID(
	ctx context.Context, kbID string,
) ([]*types.Knowledge, error) {
	var knowledges []*types.Knowledge
	if err := r.db.WithContext(ctx).Where("id_knowledge_base = ?", kbID).
		Order("created_at DESC").Find(&knowledges).Error; err != nil {
		return nil, err
	}
	return knowledges, nil
}

// ListPagedKnowledgeByKnowledgeBaseID lists all knowledge in a knowledge base with pagination
func (r *knowledgeRepository) ListPagedKnowledgeByKnowledgeBaseID(
	ctx context.Context,
	kbID string,
	page *types.Pagination,
	tagID string,
	keyword string,
	fileType string,
) ([]*types.Knowledge, int64, error) {
	var knowledges []*types.Knowledge
	var total int64

	buildFilter := func(db *gorm.DB) *gorm.DB {
		db = db.Where("id_knowledge_base = ?", kbID)
		if tagID != "" {
			db = db.Where("tag_id = ?", tagID)
		}
		if keyword != "" {
			escaped := escapeLikeKeyword(keyword)
			db = db.Where("(file_name LIKE ? OR title LIKE ?)", "%"+escaped+"%", "%"+escaped+"%")
		}
		if fileType != "" {
			if fileType == "manual" {
				db = db.Where("type = ?", "manual")
			} else if fileType == "url" {
				db = db.Where("type = ?", "url")
			} else {
				db = db.Where("file_type = ?", fileType)
			}
		}
		return db
	}

	// Query total count first
	countQuery := buildFilter(r.db.WithContext(ctx).Model(&types.Knowledge{}))
	if err := countQuery.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Then query paginated data
	dataQuery := buildFilter(r.db.WithContext(ctx).Model(&types.Knowledge{}))
	if err := dataQuery.
		Order("created_at DESC").
		Offset(page.Offset()).
		Limit(page.Limit()).
		Find(&knowledges).Error; err != nil {
		return nil, 0, err
	}

	return knowledges, total, nil
}

// UpdateKnowledge updates knowledge
func (r *knowledgeRepository) UpdateKnowledge(ctx context.Context, knowledge *types.Knowledge) error {
	err := r.db.WithContext(ctx).Omit(omitFieldsOnUpdate...).Save(knowledge).Error
	return err
}

// UpdateKnowledgeBatch updates knowledge items in batch
func (r *knowledgeRepository) UpdateKnowledgeBatch(ctx context.Context, knowledgeList []*types.Knowledge) error {
	if len(knowledgeList) == 0 {
		return nil
	}
	return r.db.Debug().WithContext(ctx).Omit(omitFieldsOnUpdate...).Save(knowledgeList).Error
}

// DeleteKnowledge deletes knowledge
func (r *knowledgeRepository) DeleteKnowledge(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Where("id_knowledge = ?", id).Delete(&types.Knowledge{}).Error
}

// DeleteKnowledgeList deletes multiple knowledge entries
func (r *knowledgeRepository) DeleteKnowledgeList(ctx context.Context, ids []string) error {
	return r.db.WithContext(ctx).Where("id_knowledge IN ?", ids).Delete(&types.Knowledge{}).Error
}

// GetKnowledgeBatch gets knowledge in batch
func (r *knowledgeRepository) GetKnowledgeBatch(
	ctx context.Context, ids []string,
) ([]*types.Knowledge, error) {
	var knowledge []*types.Knowledge
	if err := r.db.WithContext(ctx).
		Where("id_knowledge IN ?", ids).
		Find(&knowledge).Error; err != nil {
		return nil, err
	}
	return knowledge, nil
}

// CheckKnowledgeExists checks if knowledge already exists
func (r *knowledgeRepository) CheckKnowledgeExists(
	ctx context.Context,
	kbID string,
	params *types.KnowledgeCheckParams,
) (bool, *types.Knowledge, error) {
	query := r.db.WithContext(ctx).Model(&types.Knowledge{}).
		Where("id_knowledge_base = ? AND parse_status <> ?", kbID, "failed")

	switch params.Type {
	case "file":
		// If file hash exists, prioritize exact match using hash
		if params.FileHash != "" {
			var knowledge types.Knowledge
			err := query.Where("file_hash = ?", params.FileHash).First(&knowledge).Error
			if err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return false, nil, nil
				}
				return false, nil, err
			}
			return true, &knowledge, nil
		}

		// If no hash or hash doesn't match, use filename and size
		if params.FileName != "" && params.FileSize > 0 {
			var knowledge types.Knowledge
			err := query.Where(
				"file_name = ? AND file_size = ?",
				params.FileName, params.FileSize,
			).First(&knowledge).Error
			if err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return false, nil, nil
				}
				return false, nil, err
			}
			return true, &knowledge, nil
		}
	case "url":
		// Use file hash for URL deduplication
		if params.FileHash != "" {
			var knowledge types.Knowledge
			err := query.Where("type = 'url' AND file_hash = ?", params.FileHash).First(&knowledge).Error
			if err == nil && knowledge.ID != "" {
				return true, &knowledge, nil
			}
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return false, nil, err
			}
		}
		return false, nil, nil
	}

	// No valid parameters, default to not existing
	return false, nil, nil
}

func (r *knowledgeRepository) UpdateKnowledgeColumn(
	ctx context.Context,
	id string,
	column string,
	value interface{},
) error {
	err := r.db.WithContext(ctx).Model(&types.Knowledge{}).Where("id_knowledge = ?", id).Update(column, value).Error
	return err
}

// CountKnowledgeByKnowledgeBaseID counts the number of knowledge items in a knowledge base
func (r *knowledgeRepository) CountKnowledgeByKnowledgeBaseID(
	ctx context.Context,
	kbID string,
) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&types.Knowledge{}).
		Where("id_knowledge_base = ?", kbID).
		Count(&count).Error
	return count, err
}

// CountKnowledgeByStatus counts the number of knowledge items with the specified parse status
func (r *knowledgeRepository) CountKnowledgeByStatus(
	ctx context.Context,
	kbID string,
	parseStatuses []string,
) (int64, error) {
	if len(parseStatuses) == 0 {
		return 0, nil
	}

	var count int64
	query := r.db.WithContext(ctx).Model(&types.Knowledge{}).
		Where("id_knowledge_base = ?", kbID).
		Where("parse_status IN ?", parseStatuses)

	if err := query.Count(&count).Error; err != nil {
		return 0, err
	}

	return count, nil
}

// SearchKnowledge searches knowledge items by keyword
// If keyword is empty, returns recent files
// Only returns documents from document-type knowledge bases (excludes FAQ)
// Returns (results, hasMore, error)
func (r *knowledgeRepository) SearchKnowledge(
	ctx context.Context,
	keyword string,
	offset, limit int,
	fileTypes []string,
) ([]*types.Knowledge, bool, error) {
	var results []*types.Knowledge
	query := r.db.WithContext(ctx).
		Table("knowledge").
		Select("knowledge.*").
		Joins("JOIN knowledge_base ON knowledge_base.id_knowledge_base = knowledge.id_knowledge_base").
		Where("knowledge_base.type = ?", types.KnowledgeBaseTypeDocument).
		Where("knowledge.deleted_at IS NULL")

	// If keyword is provided, filter by file_name or title
	if keyword != "" {
		escaped := escapeLikeKeyword(keyword)
		query = query.Where("(knowledge.file_name LIKE ? OR knowledge.title LIKE ?)", "%"+escaped+"%", "%"+escaped+"%")
	}

	// If fileTypes is provided, filter by file extension or type
	if len(fileTypes) > 0 {
		query = applyFileTypeFilter(query, fileTypes)
	}

	// Fetch limit+1 to check if there are more results
	err := query.Order("knowledge.created_at DESC").
		Offset(offset).
		Limit(limit + 1).
		Scan(&results).Error
	if err != nil {
		return nil, false, err
	}

	// Check if there are more results
	hasMore := len(results) > limit
	if hasMore {
		results = results[:limit]
	}

	return results, hasMore, nil
}

// SearchKnowledgeInScopes searches knowledge items by keyword within the given kb_id scopes.
func (r *knowledgeRepository) SearchKnowledgeInScopes(
	ctx context.Context,
	scopes []types.KnowledgeSearchScope,
	keyword string,
	offset, limit int,
	fileTypes []string,
) ([]*types.Knowledge, bool, error) {
	if len(scopes) == 0 {
		return nil, false, nil
	}

	// Collect KB IDs from scopes
	kbIDs := make([]string, len(scopes))
	for i, s := range scopes {
		kbIDs[i] = s.KBID
	}

	var results []*types.Knowledge
	query := r.db.WithContext(ctx).
		Table("knowledge").
		Select("knowledge.*").
		Joins("JOIN knowledge_base ON knowledge_base.id_knowledge_base = knowledge.id_knowledge_base").
		Where("knowledge.id_knowledge_base IN ?", kbIDs).
		Where("knowledge_base.type = ?", types.KnowledgeBaseTypeDocument).
		Where("knowledge.deleted_at IS NULL")

	if keyword != "" {
		escaped := escapeLikeKeyword(keyword)
		query = query.Where("(knowledge.file_name LIKE ? OR knowledge.title LIKE ?)", "%"+escaped+"%", "%"+escaped+"%")
	}

	if len(fileTypes) > 0 {
		query = applyFileTypeFilter(query, fileTypes)
	}

	err := query.Order("knowledge.created_at DESC").
		Offset(offset).
		Limit(limit + 1).
		Scan(&results).Error
	if err != nil {
		return nil, false, err
	}

	hasMore := len(results) > limit
	if hasMore {
		results = results[:limit]
	}

	return results, hasMore, nil
}

// ListIDsByTagID returns all knowledge IDs that have the specified tag ID
func (r *knowledgeRepository) ListIDsByTagID(
	ctx context.Context,
	kbID, tagID string,
) ([]string, error) {
	var ids []string
	err := r.db.WithContext(ctx).Model(&types.Knowledge{}).
		Where("id_knowledge_base = ? AND tag_id = ?", kbID, tagID).
		Pluck("id_knowledge", &ids).Error
	return ids, err
}

// applyFileTypeFilter adds file type filter conditions to a query
func applyFileTypeFilter(query *gorm.DB, fileTypes []string) *gorm.DB {
	seen := make(map[string]bool)
	var uniquePatterns []string
	includeURL := false
	for _, ft := range fileTypes {
		ft = strings.ToLower(strings.TrimPrefix(ft, "."))
		if ft == "url" || ft == "html" {
			includeURL = true
			continue
		}
		pattern := "%." + ft
		if !seen[pattern] {
			seen[pattern] = true
			uniquePatterns = append(uniquePatterns, pattern)
		}
		// Handle common aliases
		var aliases []string
		switch ft {
		case "xlsx":
			aliases = []string{"%.xls"}
		case "xls":
			aliases = []string{"%.xlsx"}
		case "docx":
			aliases = []string{"%.doc"}
		case "doc":
			aliases = []string{"%.docx"}
		case "jpg":
			aliases = []string{"%.jpeg", "%.png"}
		case "jpeg":
			aliases = []string{"%.jpg", "%.png"}
		case "png":
			aliases = []string{"%.jpg", "%.jpeg"}
		}
		for _, alias := range aliases {
			if !seen[alias] {
				seen[alias] = true
				uniquePatterns = append(uniquePatterns, alias)
			}
		}
	}
	var orConditions []string
	var args []interface{}
	for _, p := range uniquePatterns {
		orConditions = append(orConditions, "LOWER(knowledge.file_name) LIKE ?")
		args = append(args, p)
	}
	if includeURL {
		orConditions = append(orConditions, "knowledge.type = ?")
		args = append(args, "url")
	}
	if len(orConditions) > 0 {
		query = query.Where("("+strings.Join(orConditions, " OR ")+")", args...)
	}
	return query
}
