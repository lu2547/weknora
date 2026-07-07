package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/application/service/retriever"
	werrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"
)

// knowledgeTagService implements KnowledgeTagService.
type knowledgeTagService struct {
	kbService      interfaces.KnowledgeBaseService
	repo           interfaces.KnowledgeTagRepository
	shareRepo      interfaces.KnowledgeTagShareRepository
	knowledgeRepo  interfaces.KnowledgeRepository
	chunkRepo      interfaces.ChunkRepository
	retrieveEngine interfaces.RetrieveEngineRegistry
	modelService   interfaces.ModelService
	task           interfaces.TaskEnqueuer
	kbShareService interfaces.KBShareService
}

// NewKnowledgeTagService creates a new tag service.
func NewKnowledgeTagService(
	kbService interfaces.KnowledgeBaseService,
	repo interfaces.KnowledgeTagRepository,
	shareRepo interfaces.KnowledgeTagShareRepository,
	knowledgeRepo interfaces.KnowledgeRepository,
	chunkRepo interfaces.ChunkRepository,
	retrieveEngine interfaces.RetrieveEngineRegistry,
	modelService interfaces.ModelService,
	task interfaces.TaskEnqueuer,
	kbShareService interfaces.KBShareService,
) (interfaces.KnowledgeTagService, error) {
	return &knowledgeTagService{
		kbService:      kbService,
		repo:           repo,
		shareRepo:      shareRepo,
		knowledgeRepo:  knowledgeRepo,
		chunkRepo:      chunkRepo,
		retrieveEngine: retrieveEngine,
		modelService:   modelService,
		task:           task,
		kbShareService: kbShareService,
	}, nil
}

// ListTags lists all tags for a knowledge base with usage stats.
func (s *knowledgeTagService) ListTags(
	ctx context.Context,
	kbID string,
	page *types.Pagination,
	keyword string,
) (*types.PageResult, error) {
	if kbID == "" {
		return nil, werrors.NewBadRequestError("知识库ID不能为空")
	}
	if page == nil {
		page = &types.Pagination{}
	}
	keyword = strings.TrimSpace(keyword)

	// Ensure KB exists
	if _, err := s.kbService.GetKnowledgeBaseByID(ctx, kbID); err != nil {
		return nil, err
	}

	tags, total, err := s.repo.ListByKB(ctx, kbID, page, keyword)
	if err != nil {
		return nil, err
	}

	if len(tags) == 0 {
		return types.NewPageResult(total, page, []*types.KnowledgeTagWithStats{}), nil
	}

	// Collect all tag IDs for batch query
	tagIDs := make([]string, 0, len(tags))
	for _, tag := range tags {
		if tag != nil {
			tagIDs = append(tagIDs, tag.ID)
		}
	}

	// Batch query all reference counts in 2 SQL queries instead of 2*N
	countsMap, err := s.repo.BatchCountReferences(ctx, kbID, tagIDs)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"kb_id": kbID,
		})
		return nil, err
	}

	results := make([]*types.KnowledgeTagWithStats, 0, len(tags))
	for _, tag := range tags {
		if tag == nil {
			continue
		}
		counts := countsMap[tag.ID]
		results = append(results, &types.KnowledgeTagWithStats{
			KnowledgeTag:   *tag,
			KnowledgeCount: counts.KnowledgeCount,
			ChunkCount:     counts.ChunkCount,
		})
	}

	return types.NewPageResult(total, page, results), nil
}

// CreateTag creates a new tag under a KB. parentID may be empty to create a root-level tag.
func (s *knowledgeTagService) CreateTag(
	ctx context.Context,
	kbID string,
	parentID string,
	name string,
	sort int,
) (*types.KnowledgeTag, error) {
	name = strings.TrimSpace(name)
	if kbID == "" || name == "" {
		return nil, werrors.NewBadRequestError("知识库ID和标签名称不能为空")
	}
	if !types.ValidateTagName(name) {
		return nil, werrors.NewBadRequestError("标签名称不能包含路径分隔符 '" + types.TagPathSeparator + "'")
	}
	if _, err := s.kbService.GetKnowledgeBaseByID(ctx, kbID); err != nil {
		return nil, err
	}

	if parentID != "" {
		parent, err := s.repo.GetByID(ctx, parentID)
		if err != nil {
			return nil, werrors.NewBadRequestError("父标签不存在")
		}
		if parent.KnowledgeBaseID != kbID {
			return nil, werrors.NewBadRequestError("父标签不属于当前知识库")
		}
	}

	// Sibling uniqueness check (same parent + name).
	existingTag, err := s.repo.GetByName(ctx, kbID, parentID, name)
	if err == nil && existingTag != nil {
		return nil, werrors.NewConflictError("同级标签名称已存在")
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	now := time.Now()
	// "未分类" tag should have the lowest sort order to appear first
	if name == types.UntaggedTagName {
		sort = -1
	}
	tag := &types.KnowledgeTag{
		ID:              uuid.New().String(),
		KnowledgeBaseID: kbID,
		ParentID:        parentID,
		Name:            name,
		Sort:            sort,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := s.repo.Create(ctx, tag); err != nil {
		return nil, err
	}
	return tag, nil
}

// UpdateTag updates tag basic information (name and/or sort order).
func (s *knowledgeTagService) UpdateTag(
	ctx context.Context,
	id string,
	name *string,
	sort *int,
) (*types.KnowledgeTag, error) {
	if id == "" {
		return nil, werrors.NewBadRequestError("标签不能为空")
	}
	tag, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if name != nil {
		newName := strings.TrimSpace(*name)
		if !types.ValidateTagName(newName) {
			return nil, werrors.NewBadRequestError("标签名称不能为空或包含路径分隔符")
		}
		if newName != tag.Name {
			// Sibling uniqueness check under the same parent.
			if existing, chkErr := s.repo.GetByName(ctx, tag.KnowledgeBaseID, tag.ParentID, newName); chkErr == nil && existing != nil && existing.ID != tag.ID {
				return nil, werrors.NewConflictError("同级标签名称已存在")
			} else if chkErr != nil && !errors.Is(chkErr, gorm.ErrRecordNotFound) {
				return nil, chkErr
			}
		}
		tag.Name = newName
	}
	if sort != nil {
		tag.Sort = *sort
	}
	tag.UpdatedAt = time.Now()

	if err := s.repo.Update(ctx, tag); err != nil {
		return nil, err
	}
	return tag, nil
}

// MoveTag re-parents a tag to newParentID (empty means root).
func (s *knowledgeTagService) MoveTag(
	ctx context.Context,
	id string,
	newParentID string,
) (*types.KnowledgeTag, error) {
	if id == "" {
		return nil, werrors.NewBadRequestError("标签不能为空")
	}
	tag, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if newParentID == tag.ID {
		return nil, werrors.NewBadRequestError("不能将标签移到自己下")
	}
	if newParentID == tag.ParentID {
		return tag, nil
	}

	if newParentID != "" {
		parent, err := s.repo.GetByID(ctx, newParentID)
		if err != nil {
			return nil, werrors.NewBadRequestError("目标父标签不存在")
		}
		if parent.KnowledgeBaseID != tag.KnowledgeBaseID {
			return nil, werrors.NewBadRequestError("不能跨知识库移动标签")
		}
	}

	// Sibling uniqueness at the new location.
	if existing, chkErr := s.repo.GetByName(ctx, tag.KnowledgeBaseID, newParentID, tag.Name); chkErr == nil && existing != nil && existing.ID != tag.ID {
		return nil, werrors.NewConflictError("目标位置已存在同名标签")
	} else if chkErr != nil && !errors.Is(chkErr, gorm.ErrRecordNotFound) {
		return nil, chkErr
	}

	tag.ParentID = newParentID
	tag.UpdatedAt = time.Now()
	if err := s.repo.Update(ctx, tag); err != nil {
		return nil, err
	}
	return tag, nil
}

// ListTagTree returns the full tag tree for a KB with per-node usage statistics.
func (s *knowledgeTagService) ListTagTree(ctx context.Context, kbID string) ([]*types.KnowledgeTagTreeNode, error) {
	if kbID == "" {
		return nil, werrors.NewBadRequestError("知识库ID不能为空")
	}
	if _, err := s.kbService.GetKnowledgeBaseByID(ctx, kbID); err != nil {
		return nil, err
	}

	tags, err := s.repo.ListAllByKB(ctx, kbID)
	if err != nil {
		return nil, err
	}
	if len(tags) == 0 {
		return []*types.KnowledgeTagTreeNode{}, nil
	}

	tagIDs := make([]string, 0, len(tags))
	for _, t := range tags {
		if t != nil {
			tagIDs = append(tagIDs, t.ID)
		}
	}
	countsMap, err := s.repo.BatchCountReferences(ctx, kbID, tagIDs)
	if err != nil {
		return nil, err
	}

	// Build parent -> children index.
	nodeByID := make(map[string]*types.KnowledgeTagTreeNode, len(tags))
	for _, t := range tags {
		if t == nil {
			continue
		}
		counts := countsMap[t.ID]
		nodeByID[t.ID] = &types.KnowledgeTagTreeNode{
			KnowledgeTagWithStats: types.KnowledgeTagWithStats{
				KnowledgeTag:   *t,
				KnowledgeCount: counts.KnowledgeCount,
				ChunkCount:     counts.ChunkCount,
			},
		}
	}
	roots := make([]*types.KnowledgeTagTreeNode, 0)
	for _, t := range tags {
		if t == nil {
			continue
		}
		node := nodeByID[t.ID]
		if t.ParentID == "" {
			roots = append(roots, node)
			continue
		}
		if parent, ok := nodeByID[t.ParentID]; ok {
			parent.Children = append(parent.Children, node)
		} else {
			// Orphan (parent missing): surface as root to avoid data loss.
			roots = append(roots, node)
		}
	}
	return roots, nil
}

// DeleteTag deletes a tag. When force=true, also deletes all chunks under this tag.
// For document-type knowledge bases, also deletes all knowledge files under this tag.
// When contentOnly=true, only deletes the content under the tag but keeps the tag itself.
func (s *knowledgeTagService) DeleteTag(ctx context.Context, id string, force bool, contentOnly bool, excludeIDs []string) error {
	if id == "" {
		return werrors.NewBadRequestError("标签ID不能为空")
	}
	tag, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	// Get KB info
	kb, err := s.kbService.GetKnowledgeBaseByID(ctx, tag.KnowledgeBaseID)
	if err != nil {
		return err
	}

	kCount, cCount, err := s.repo.CountReferences(ctx, tag.KnowledgeBaseID, tag.ID)
	if err != nil {
		return err
	}

	// Get tenant info for effective engines
	tenantInfo, _ := types.TenantInfoFromContext(ctx)
	tenantID := types.MustTenantIDFromContext(ctx)

	// Helper function to delete chunks and enqueue index deletion task
	deleteChunksAndEnqueueIndexDelete := func() error {
		// Delete chunks and get their IDs
		deletedIDs, err := s.chunkRepo.DeleteChunksByTagID(ctx, tag.KnowledgeBaseID, tag.ID, excludeIDs)
		if err != nil {
			logger.Errorf(ctx, "Failed to delete chunks by tag ID %s: %v", tag.ID, err)
			return werrors.NewInternalServerError("删除标签下的数据失败")
		}

		// Enqueue async index deletion task for the deleted chunks
		if len(deletedIDs) > 0 {
			embeddingModelID := s.getDefaultEmbeddingModelID(ctx)
			s.enqueueIndexDeleteTask(ctx, tenantID, kb.ID, embeddingModelID, string(kb.Type), deletedIDs, tenantInfo.GetEffectiveEngines())
		}

		logger.Infof(ctx, "Deleted %d chunks under tag %s", len(deletedIDs), tag.ID)
		return nil
	}

	// Helper function to enqueue knowledge list delete task for document-type knowledge bases
	enqueueKnowledgeDeleteTask := func() error {
		if kb.Type != types.KnowledgeBaseTypeDocument {
			return nil
		}
		// Get all knowledge IDs under this tag
		knowledgeIDs, err := s.knowledgeRepo.ListIDsByTagID(ctx, kb.ID, tag.ID)
		if err != nil {
			logger.Errorf(ctx, "Failed to list knowledge IDs by tag ID %s: %v", tag.ID, err)
			return werrors.NewInternalServerError("获取标签下的文档失败")
		}
		if len(knowledgeIDs) == 0 {
			return nil
		}
		// Enqueue async task to delete knowledge files
		payload := types.KnowledgeListDeletePayload{
			TenantID:     tenantID,
			KnowledgeIDs: knowledgeIDs,
		}
		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			logger.Errorf(ctx, "Failed to marshal knowledge list delete payload: %v", err)
			return werrors.NewInternalServerError("删除标签下的文档失败")
		}
		task := asynq.NewTask(types.TypeKnowledgeListDelete, payloadBytes, asynq.Queue("low"), asynq.MaxRetry(3))
		info, err := s.task.Enqueue(task)
		if err != nil {
			logger.Errorf(ctx, "Failed to enqueue knowledge list delete task: %v", err)
			return werrors.NewInternalServerError("删除标签下的文档失败")
		}
		logger.Infof(ctx, "Enqueued knowledge list delete task %s for %d knowledge files under tag %s", info.ID, len(knowledgeIDs), tag.ID)
		return nil
	}

	// contentOnly mode: only delete content, keep the tag
	if contentOnly {
		if kb.Type == types.KnowledgeBaseTypeDocument && kCount > 0 {
			if err := enqueueKnowledgeDeleteTask(); err != nil {
				return err
			}
		} else if cCount > 0 {
			if err := deleteChunksAndEnqueueIndexDelete(); err != nil {
				return err
			}
		}
		return nil
	}

	if !force && (kCount > 0 || cCount > 0) {
		return werrors.NewBadRequestError("标签仍有知识或FAQ条目引用，无法删除")
	}

	// When force=true, delete all content under this tag first
	if force {
		if kb.Type == types.KnowledgeBaseTypeDocument && kCount > 0 {
			if err := enqueueKnowledgeDeleteTask(); err != nil {
				return err
			}
		} else if cCount > 0 {
			if err := deleteChunksAndEnqueueIndexDelete(); err != nil {
				return err
			}
		}
	}

	// If there are excludeIDs, we cannot delete the tag itself as it still has content
	if len(excludeIDs) > 0 {
		return nil
	}
	return s.repo.Delete(ctx, id)
}

// getDefaultEmbeddingModelID returns the system default embedding model ID.
func (s *knowledgeTagService) getDefaultEmbeddingModelID(ctx context.Context) string {
	models, err := s.modelService.ListModels(ctx)
	if err != nil {
		return ""
	}
	for _, m := range models {
		if m.Type == types.ModelTypeEmbedding && m.IsDefault {
			return m.ID
		}
	}
	return ""
}

// enqueueIndexDeleteTask enqueues an async task for index deletion (low priority)
func (s *knowledgeTagService) enqueueIndexDeleteTask(ctx context.Context,
	tenantID uint64, kbID, embeddingModelID, kbType string, chunkIDs []string, effectiveEngines []types.RetrieverEngineParams,
) {
	payload := types.IndexDeletePayload{
		TenantID:         tenantID,
		KnowledgeBaseID:  kbID,
		EmbeddingModelID: embeddingModelID,
		KBType:           kbType,
		ChunkIDs:         chunkIDs,
		EffectiveEngines: effectiveEngines,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		logger.Errorf(ctx, "Failed to marshal index delete payload: %v", err)
		return
	}

	task := asynq.NewTask(types.TypeIndexDelete, payloadBytes, asynq.Queue("low"), asynq.MaxRetry(10))
	info, err := s.task.Enqueue(task)
	if err != nil {
		logger.Errorf(ctx, "Failed to enqueue index delete task: %v", err)
		return
	}
	logger.Infof(ctx, "Enqueued index delete task: %s for %d chunks", info.ID, len(chunkIDs))
}

// ProcessIndexDelete handles async index deletion task
func (s *knowledgeTagService) ProcessIndexDelete(ctx context.Context, t *asynq.Task) error {
	var payload types.IndexDeletePayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		logger.Errorf(ctx, "Failed to unmarshal index delete payload: %v", err)
		return err
	}

	// Set tenant context for downstream services
	ctx = context.WithValue(ctx, types.TenantIDContextKey, payload.TenantID)

	logger.Infof(ctx, "Processing index delete task for %d chunks in KB %s", len(payload.ChunkIDs), payload.KnowledgeBaseID)

	// Create retrieve engine
	retrieveEngine, err := retriever.NewCompositeRetrieveEngine(s.retrieveEngine, payload.EffectiveEngines)
	if err != nil {
		logger.Warnf(ctx, "Failed to create retrieve engine for index cleanup: %v", err)
		return err
	}

	// Get embedding model dimensions
	embeddingModel, err := s.modelService.GetEmbeddingModel(ctx, payload.EmbeddingModelID)
	if err != nil {
		logger.Warnf(ctx, "Failed to get embedding model for index cleanup: %v", err)
		return err
	}

	// Delete indices in batches to avoid overwhelming the backend
	const batchSize = 100
	chunkIDs := payload.ChunkIDs
	dimension := embeddingModel.GetDimensions()

	for i := 0; i < len(chunkIDs); i += batchSize {
		end := i + batchSize
		if end > len(chunkIDs) {
			end = len(chunkIDs)
		}
		batch := chunkIDs[i:end]

		if err := retrieveEngine.DeleteByChunkIDList(ctx, payload.KnowledgeBaseID, batch, dimension, payload.KBType); err != nil {
			logger.Warnf(ctx, "Failed to delete indices for chunks batch [%d-%d]: %v", i, end, err)
			return err
		}
		logger.Debugf(ctx, "Deleted indices batch [%d-%d] of %d chunks", i, end, len(chunkIDs))
	}

	logger.Infof(ctx, "Successfully deleted indices for %d chunks", len(payload.ChunkIDs))
	return nil
}

// FindOrCreateTagByName finds a tag by name or creates it if not exists.
func (s *knowledgeTagService) FindOrCreateTagByName(ctx context.Context, kbID string, name string) (*types.KnowledgeTag, error) {
	name = strings.TrimSpace(name)
	if kbID == "" || name == "" {
		return nil, werrors.NewBadRequestError("知识库ID和标签名称不能为空")
	}

	if _, err := s.kbService.GetKnowledgeBaseByID(ctx, kbID); err != nil {
		return nil, err
	}

	// Try to find existing tag (root level)
	tag, err := s.repo.GetByName(ctx, kbID, "", name)
	if err == nil {
		return tag, nil
	}

	// If not a "not found" error, return directly
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	// Create new tag (root level)
	return s.CreateTag(ctx, kbID, "", name, 0)
}

// validTagSharePermission returns true if the permission string is one of the
// supported levels.
func validTagSharePermission(p string) bool {
	switch p {
	case "viewer", "editor", "admin":
		return true
	}
	return false
}

// ShareTag shares the given tag with a target group (e.g. organization).
func (s *knowledgeTagService) ShareTag(
	ctx context.Context,
	tagID string,
	groupKey string,
	sharedByUserID string,
	permission string,
) (*types.KnowledgeTagShare, error) {
	tagID = strings.TrimSpace(tagID)
	groupKey = strings.TrimSpace(groupKey)
	if tagID == "" || groupKey == "" {
		return nil, werrors.NewBadRequestError("标签ID和目标组织ID不能为空")
	}
	permission = strings.TrimSpace(permission)
	if permission == "" {
		permission = "viewer"
	}
	if !validTagSharePermission(permission) {
		return nil, werrors.NewBadRequestError("无效的权限级别")
	}
	if s.shareRepo == nil {
		return nil, werrors.NewInternalServerError("标签共享未启用")
	}

	tag, err := s.repo.GetByID(ctx, tagID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, werrors.NewNotFoundError("标签不存在")
		}
		return nil, err
	}

	// Authorization: tag must belong to an accessible KB
	if _, err := s.kbService.GetKnowledgeBaseByID(ctx, tag.KnowledgeBaseID); err != nil {
		return nil, err
	}

	// Idempotent: update permission when an existing share is found.
	existing, err := s.shareRepo.GetByTagAndGroup(ctx, tagID, groupKey)
	if err == nil {
		if existing.Permission != permission {
			existing.Permission = permission
			if sharedByUserID != "" {
				existing.SharedByUserID = sharedByUserID
			}
			if uErr := s.shareRepo.Update(ctx, existing); uErr != nil {
				return nil, uErr
			}
		}
		return existing, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	share := &types.KnowledgeTagShare{
		ID:             uuid.New().String(),
		KnowledgeTagID: tagID,
		GroupKey:       groupKey,
		SharedByUserID: sharedByUserID,
		Permission:     permission,
	}
	if err := s.shareRepo.Create(ctx, share); err != nil {
		return nil, err
	}
	logger.Infof(ctx, "Tag %s shared to group %s with permission %s by user %s",
		tagID, groupKey, permission, sharedByUserID)
	return share, nil
}

// RevokeTagShare deletes a tag share record.
func (s *knowledgeTagService) RevokeTagShare(ctx context.Context, shareID string, userID string) error {
	shareID = strings.TrimSpace(shareID)
	if shareID == "" {
		return werrors.NewBadRequestError("共享ID不能为空")
	}
	if s.shareRepo == nil {
		return werrors.NewInternalServerError("标签共享未启用")
	}

	share, err := s.shareRepo.GetByID(ctx, shareID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return werrors.NewNotFoundError("共享记录不存在")
		}
		return err
	}

	// Verify the tag's KB is accessible
	tag, err := s.repo.GetByID(ctx, share.KnowledgeTagID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return werrors.NewForbiddenError("无权操作该共享记录")
		}
		return err
	}
	if _, err := s.kbService.GetKnowledgeBaseByID(ctx, tag.KnowledgeBaseID); err != nil {
		return werrors.NewForbiddenError("无权操作该共享记录")
	}

	if err := s.shareRepo.Delete(ctx, shareID); err != nil {
		return err
	}
	logger.Infof(ctx, "Tag share %s revoked by user %s", shareID, userID)
	return nil
}

// ListTagShares lists all share records for a given tag.
func (s *knowledgeTagService) ListTagShares(ctx context.Context, tagID string) ([]*types.KnowledgeTagShare, error) {
	tagID = strings.TrimSpace(tagID)
	if tagID == "" {
		return nil, werrors.NewBadRequestError("标签ID不能为空")
	}
	if s.shareRepo == nil {
		return []*types.KnowledgeTagShare{}, nil
	}

	tag, err := s.repo.GetByID(ctx, tagID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, werrors.NewNotFoundError("标签不存在")
		}
		return nil, err
	}
	if _, err := s.kbService.GetKnowledgeBaseByID(ctx, tag.KnowledgeBaseID); err != nil {
		return nil, werrors.NewForbiddenError("无权查看该标签的共享列表")
	}

	return s.shareRepo.ListByTagID(ctx, tagID)
}
