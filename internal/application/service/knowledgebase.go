package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Tencent/WeKnora/internal/application/service/retriever"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/hibiken/asynq"
)

// generateKnowledgeBaseID returns the canonical KB ID format: "KB<unix_ms>".
// Used by both CreateKnowledgeBase and CopyKnowledgeBase so the PG
// id_knowledge_base column and the derived Milvus collection name
// (enterprise_<lower(id)>) stay consistent across all creation paths.
func generateKnowledgeBaseID() string {
	return fmt.Sprintf("KB%d", time.Now().UnixMilli())
}

// knowledgeBaseService implements the knowledge base service interface
type knowledgeBaseService struct {
	repo           interfaces.KnowledgeBaseRepository
	kgRepo         interfaces.KnowledgeRepository
	chunkRepo      interfaces.ChunkRepository
	shareRepo      interfaces.KBShareRepository
	kbShareService interfaces.KBShareService
	modelService   interfaces.ModelService
	retrieveEngine interfaces.RetrieveEngineRegistry
	summaryIndex   interfaces.SummaryIndex
	tenantRepo     interfaces.TenantRepository
	fileSvc        interfaces.FileService
	graphEngine    interfaces.RetrieveGraphRepository
	asynqClient    interfaces.TaskEnqueuer
}

// NewKnowledgeBaseService creates a new knowledge base service
func NewKnowledgeBaseService(repo interfaces.KnowledgeBaseRepository,
	kgRepo interfaces.KnowledgeRepository,
	chunkRepo interfaces.ChunkRepository,
	shareRepo interfaces.KBShareRepository,
	kbShareService interfaces.KBShareService,
	modelService interfaces.ModelService,
	retrieveEngine interfaces.RetrieveEngineRegistry,
	summaryIndex interfaces.SummaryIndex,
	tenantRepo interfaces.TenantRepository,
	fileSvc interfaces.FileService,
	graphEngine interfaces.RetrieveGraphRepository,
	asynqClient interfaces.TaskEnqueuer,
) interfaces.KnowledgeBaseService {
	return &knowledgeBaseService{
		repo:           repo,
		kgRepo:         kgRepo,
		chunkRepo:      chunkRepo,
		shareRepo:      shareRepo,
		kbShareService: kbShareService,
		modelService:   modelService,
		retrieveEngine: retrieveEngine,
		summaryIndex:   summaryIndex,
		tenantRepo:     tenantRepo,
		fileSvc:        fileSvc,
		graphEngine:    graphEngine,
		asynqClient:    asynqClient,
	}
}

// GetRepository gets the knowledge base repository
// Parameters:
//   - ctx: Context with authentication and request information
//
// Returns:
//   - interfaces.KnowledgeBaseRepository: Knowledge base repository
func (s *knowledgeBaseService) GetRepository() interfaces.KnowledgeBaseRepository {
	return s.repo
}

// CreateKnowledgeBase creates a new knowledge base
func (s *knowledgeBaseService) CreateKnowledgeBase(ctx context.Context,
	kb *types.KnowledgeBase,
) (*types.KnowledgeBase, error) {
	// 三级知识库类别校验：前端未传 → EnsureDefaults 兑底为 personal；
	// 传了但不合法 → 直接报错，避免下游 Milvus collection 路由拿到未知 category。
	if kb.Category != "" && !types.IsValidKnowledgeBaseCategory(kb.Category) {
		logger.Errorf(ctx, "Invalid knowledge base category: %q", kb.Category)
		return nil, errors.New("invalid knowledge base category, must be personal/public/enterprise")
	}

	// Generate UUID and set creation timestamps
	if kb.ID == "" {
		kb.ID = generateKnowledgeBaseID()
	}
	// Owner: derive from context user ID if caller didn't set it.
	// This is the source of truth for ownership-based permission checks (frontend isOwner).
	if kb.Owner == "" {
		if uid, ok := ctx.Value(types.UserIDContextKey).(string); ok && uid != "" {
			kb.Owner = uid
		}
	}
	kb.CreatedAt = time.Now()
	kb.UpdatedAt = time.Now()
	kb.EnsureDefaults()

	logger.Infof(ctx, "Creating knowledge base, ID: %s, name: %s, category: %s", kb.ID, kb.Name, kb.Category)

	if err := s.repo.CreateKnowledgeBase(ctx, kb); err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"knowledge_base_id": kb.ID,
		})
		return nil, err
	}

	// Eagerly provision the underlying Milvus collection (personal/public/enterprise),
	// so it shows up immediately instead of being lazy-created on first document upload.
	// Failure here is non-fatal: the lazy path in Save/BatchSave will still create it later.
	s.ensureKBCollection(ctx, kb)

	logger.Infof(ctx, "Knowledge base created successfully, ID: %s, name: %s", kb.ID, kb.Name)
	return kb, nil
}

// ensureKBCollection eagerly creates the per-KB collection in the configured retrieve
// engines (real impl on milvus; no-op on others). All errors are logged and swallowed —
// CreateKnowledgeBase must succeed even if Milvus is temporarily unavailable.
func (s *knowledgeBaseService) ensureKBCollection(ctx context.Context, kb *types.KnowledgeBase) {
	if kb == nil || kb.ID == "" {
		return
	}
	tenantInfo, ok := types.TenantInfoFromContext(ctx)
	if !ok || tenantInfo == nil {
		logger.Warnf(ctx, "ensureKBCollection: missing tenant info in context, skip eager provisioning for KB %s", kb.ID)
		return
	}
	effectiveEngines := tenantInfo.GetEffectiveEngines()
	if len(effectiveEngines) == 0 {
		return
	}
	// KB no longer stores embedding model; find the system default embedding model.
	models, err := s.modelService.ListModels(ctx)
	if err != nil {
		logger.Warnf(ctx, "ensureKBCollection: failed to list models for KB %s: %v", kb.ID, err)
		return
	}
	var defaultModelID string
	for _, m := range models {
		if m.Type == types.ModelTypeEmbedding && m.IsDefault {
			defaultModelID = m.ID
			break
		}
	}
	if defaultModelID == "" {
		logger.Warnf(ctx, "ensureKBCollection: no default embedding model found, skip for KB %s", kb.ID)
		return
	}
	embeddingModel, err := s.modelService.GetEmbeddingModel(ctx, defaultModelID)
	if err != nil {
		logger.Warnf(ctx, "ensureKBCollection: failed to get embedding model %s for KB %s: %v", defaultModelID, kb.ID, err)
		return
	}
	dimension := embeddingModel.GetDimensions()
	if dimension <= 0 {
		logger.Warnf(ctx, "ensureKBCollection: invalid embedding dimension %d for KB %s", dimension, kb.ID)
		return
	}
	retrieveEngine, err := retriever.NewCompositeRetrieveEngine(s.retrieveEngine, effectiveEngines)
	if err != nil {
		logger.Warnf(ctx, "ensureKBCollection: failed to build composite retrieve engine for KB %s: %v", kb.ID, err)
		return
	}
	if err := retrieveEngine.EnsureCollection(ctx, kb.ID, dimension); err != nil {
		logger.Warnf(ctx, "ensureKBCollection: failed to ensure collection for KB %s (dim=%d, category=%s): %v", kb.ID, dimension, kb.Category, err)
		return
	}
	logger.Infof(ctx, "ensureKBCollection: provisioned collection for KB %s (category=%s, dim=%d)", kb.ID, kb.Category, dimension)
}

// GetKnowledgeBaseByID retrieves a knowledge base by its ID
func (s *knowledgeBaseService) GetKnowledgeBaseByID(ctx context.Context, id string) (*types.KnowledgeBase, error) {
	if id == "" {
		logger.Error(ctx, "Knowledge base ID is empty")
		return nil, errors.New("knowledge base ID cannot be empty")
	}

	kb, err := s.repo.GetKnowledgeBaseByID(ctx, id)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"knowledge_base_id": id,
		})
		return nil, err
	}

	kb.EnsureDefaults()
	return kb, nil
}

// GetKnowledgeBaseByIDOnly retrieves knowledge base by ID without tenant filter
// Used for cross-tenant shared KB access where permission is checked elsewhere
func (s *knowledgeBaseService) GetKnowledgeBaseByIDOnly(ctx context.Context, id string) (*types.KnowledgeBase, error) {
	if id == "" {
		logger.Error(ctx, "Knowledge base ID is empty")
		return nil, errors.New("knowledge base ID cannot be empty")
	}

	kb, err := s.repo.GetKnowledgeBaseByID(ctx, id)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"knowledge_base_id": id,
		})
		return nil, err
	}

	kb.EnsureDefaults()
	return kb, nil
}

// GetKnowledgeBasesByIDsOnly retrieves knowledge bases by IDs without tenant filter (batch).
func (s *knowledgeBaseService) GetKnowledgeBasesByIDsOnly(ctx context.Context, ids []string) ([]*types.KnowledgeBase, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	kbs, err := s.repo.GetKnowledgeBaseByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, kb := range kbs {
		if kb != nil {
			kb.EnsureDefaults()
		}
	}
	return kbs, nil
}

// ListKnowledgeBases returns all knowledge bases
func (s *knowledgeBaseService) ListKnowledgeBases(ctx context.Context) ([]*types.KnowledgeBase, error) {
	kbs, err := s.repo.ListKnowledgeBases(ctx)
	if err != nil {
		logger.Errorf(ctx, "Failed to list knowledge bases: %v", err)
		return nil, err
	}

	for _, kb := range kbs {
		kb.EnsureDefaults()

		// Check if there is a processing import task
		processingCount, err := s.kgRepo.CountKnowledgeByStatus(ctx, kb.ID, []string{"pending", "processing"})
		if err != nil {
			logger.Warnf(ctx, "Failed to check processing status for knowledge base %s: %v", kb.ID, err)
		} else {
			kb.ProcessingCount = processingCount
		}
	}
	return kbs, nil
}

// FillKnowledgeBaseCounts fills ProcessingCount for the given KB.
func (s *knowledgeBaseService) FillKnowledgeBaseCounts(ctx context.Context, kb *types.KnowledgeBase) error {
	if kb == nil {
		return nil
	}
	kb.EnsureDefaults()
	if processingCount, err := s.kgRepo.CountKnowledgeByStatus(ctx, kb.ID, []string{"pending", "processing"}); err == nil {
		kb.ProcessingCount = processingCount
	}
	return nil
}

// CheckModelsConfigured checks models table and returns (embeddingModelID, summaryModelID).
// Both empty means no models configured.
func (s *knowledgeBaseService) CheckModelsConfigured(ctx context.Context) (string, string) {
	models, err := s.modelService.ListModels(ctx)
	if err != nil {
		logger.Warnf(ctx, "CheckModelsConfigured: ListModels failed: %v", err)
		return "", ""
	}
	var embID, summaryID, firstEmb, firstChat string
	for _, m := range models {
		switch m.Type {
		case types.ModelTypeEmbedding:
			if firstEmb == "" {
				firstEmb = m.ID
			}
			if m.IsDefault {
				embID = m.ID
			}
		case types.ModelTypeKnowledgeQA:
			if firstChat == "" {
				firstChat = m.ID
			}
			if m.IsDefault {
				summaryID = m.ID
			}
		}
	}
	if embID == "" {
		embID = firstEmb
	}
	if summaryID == "" {
		summaryID = firstChat
	}
	return embID, summaryID
}

// UpdateKnowledgeBase updates a knowledge base's properties
func (s *knowledgeBaseService) UpdateKnowledgeBase(ctx context.Context,
	id string,
	name string,
	description string,
	config *types.KnowledgeBaseConfig,
) (*types.KnowledgeBase, error) {
	if id == "" {
		logger.Error(ctx, "Knowledge base ID is empty")
		return nil, errors.New("knowledge base ID cannot be empty")
	}

	logger.Infof(ctx, "Updating knowledge base, ID: %s, name: %s", id, name)

	// Get existing knowledge base
	kb, err := s.repo.GetKnowledgeBaseByID(ctx, id)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"knowledge_base_id": id,
		})
		return nil, err
	}

	// Update the knowledge base properties
	kb.Name = name
	kb.Description = description
	if config != nil {
		kb.ChunkingConfig = config.ChunkingConfig
		if config.FAQConfig != nil {
			kb.FAQConfig = config.FAQConfig
		}
	}
	kb.UpdatedAt = time.Now()
	kb.EnsureDefaults()

	logger.Info(ctx, "Saving knowledge base update")
	if err := s.repo.UpdateKnowledgeBase(ctx, kb); err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"knowledge_base_id": id,
		})
		return nil, err
	}

	logger.Infof(ctx, "Knowledge base updated successfully, ID: %s, name: %s", kb.ID, kb.Name)
	return kb, nil
}

// TogglePinKnowledgeBase toggles the pin status of a knowledge base
func (s *knowledgeBaseService) TogglePinKnowledgeBase(ctx context.Context, id string) (*types.KnowledgeBase, error) {
	if id == "" {
		return nil, errors.New("knowledge base ID cannot be empty")
	}
	kb, err := s.repo.TogglePinKnowledgeBase(ctx, id)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"knowledge_base_id": id,
		})
		return nil, err
	}
	logger.Infof(ctx, "Knowledge base pin toggled, ID: %s, is_pinned: %v", id, kb.IsPinned)
	return kb, nil
}

// DeleteKnowledgeBase deletes a knowledge base by its ID
// This method marks the knowledge base as deleted and enqueues an async task
// to handle the heavy cleanup operations (embeddings, chunks, files, graph data)
func (s *knowledgeBaseService) DeleteKnowledgeBase(ctx context.Context, id string) error {
	if id == "" {
		logger.Error(ctx, "Knowledge base ID is empty")
		return errors.New("knowledge base ID cannot be empty")
	}

	logger.Infof(ctx, "Deleting knowledge base, ID: %s", id)

	// Get tenant ID from context
	tenantID := types.MustTenantIDFromContext(ctx)
	tenantInfo, _ := types.TenantInfoFromContext(ctx)

	// Step 1: Delete the knowledge base record first (mark as deleted)
	logger.Infof(ctx, "Deleting knowledge base from database")
	err := s.repo.DeleteKnowledgeBase(ctx, id)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"knowledge_base_id": id,
		})
		return err
	}

	// Step 1b: Remove all organization shares for this KB so org settings no longer show them
	if delErr := s.shareRepo.DeleteByKnowledgeBaseID(ctx, id); delErr != nil {
		logger.Warnf(ctx, "Failed to delete KB shares for knowledge base %s: %v", id, delErr)
	}

	// Step 2: Enqueue async task for heavy cleanup operations
	payload := types.KBDeletePayload{
		TenantID:         tenantID,
		KnowledgeBaseID:  id,
		EffectiveEngines: tenantInfo.GetEffectiveEngines(),
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		logger.Warnf(ctx, "Failed to marshal KB delete payload: %v", err)
		// Don't fail the request, the KB record is already deleted
		return nil
	}

	task := asynq.NewTask(types.TypeKBDelete, payloadBytes, asynq.Queue("low"), asynq.MaxRetry(3))
	info, err := s.asynqClient.Enqueue(task)
	if err != nil {
		logger.Warnf(ctx, "Failed to enqueue KB delete task: %v", err)
		// Don't fail the request, the KB record is already deleted
		return nil
	}

	logger.Infof(ctx, "KB delete task enqueued: %s, knowledge base ID: %s", info.ID, id)
	logger.Infof(ctx, "Knowledge base deleted successfully, ID: %s", id)
	return nil
}

// ProcessKBDelete handles async knowledge base deletion task
// This method performs heavy cleanup operations: deleting embeddings, chunks, files, and graph data
func (s *knowledgeBaseService) ProcessKBDelete(ctx context.Context, t *asynq.Task) error {
	var payload types.KBDeletePayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		logger.Errorf(ctx, "Failed to unmarshal KB delete payload: %v", err)
		return err
	}

	tenantID := payload.TenantID
	kbID := payload.KnowledgeBaseID

	// Set tenant context for downstream services
	ctx = context.WithValue(ctx, types.TenantIDContextKey, tenantID)

	logger.Infof(ctx, "Processing KB delete task for knowledge base: %s", kbID)

	// Step 1: Get all knowledge entries in this knowledge base
	logger.Infof(ctx, "Fetching all knowledge entries in knowledge base, ID: %s", kbID)
	knowledgeList, err := s.kgRepo.ListKnowledgeByKnowledgeBaseID(ctx, kbID)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"knowledge_base_id": kbID,
		})
		return err
	}
	logger.Infof(ctx, "Found %d knowledge entries to delete", len(knowledgeList))

	// Step 2: Delete all knowledge entries and their resources
	if len(knowledgeList) > 0 {
		knowledgeIDs := make([]string, 0, len(knowledgeList))
		for _, knowledge := range knowledgeList {
			knowledgeIDs = append(knowledgeIDs, knowledge.ID)
		}

		logger.Infof(ctx, "Deleting all knowledge entries and their resources")

		// Delete embeddings from vector store
		logger.Infof(ctx, "Deleting embeddings from vector store")
		retrieveEngine, err := retriever.NewCompositeRetrieveEngine(
			s.retrieveEngine,
			payload.EffectiveEngines,
		)
		if err != nil {
			logger.Warnf(ctx, "Failed to create retrieve engine: %v", err)
		} else {
			// Group knowledge by embedding model and type
			type groupKey struct {
				EmbeddingModelID string
				Type             string
			}
			embeddingGroups := make(map[groupKey][]string)
			for _, knowledge := range knowledgeList {
				key := groupKey{EmbeddingModelID: knowledge.EmbeddingModelID, Type: knowledge.Type}
				embeddingGroups[key] = append(embeddingGroups[key], knowledge.ID)
			}

			for key, knowledgeGroup := range embeddingGroups {
				embeddingModel, err := s.modelService.GetEmbeddingModel(ctx, key.EmbeddingModelID)
				if err != nil {
					logger.Warnf(ctx, "Failed to get embedding model %s: %v", key.EmbeddingModelID, err)
					continue
				}
				if err := retrieveEngine.DeleteByKnowledgeIDList(ctx, kbID, knowledgeGroup, embeddingModel.GetDimensions(), key.Type); err != nil {
					logger.Warnf(ctx, "Failed to delete embeddings for model %s: %v", key.EmbeddingModelID, err)
				}
			}

			// Drop the per-KB collection (milvus only; other backends no-op)
			if err := retrieveEngine.DropKnowledgeBaseCollection(ctx, kbID); err != nil {
				logger.Warnf(ctx, "Failed to drop KB collection for %s: %v", kbID, err)
			}
		}

		// Delete all per-knowledge rows from the global summary collection for this KB
		if s.summaryIndex != nil {
			if err := s.summaryIndex.DeleteKnowledgeBaseSummaries(ctx, kbID); err != nil {
				logger.Warnf(ctx, "Failed to delete summary rows for KB %s: %v", kbID, err)
			}
		}

		// Collect image URLs before chunks are deleted
		chunkImageInfos, imgErr := s.chunkRepo.ListImageInfoByKnowledgeIDs(ctx, knowledgeIDs)
		if imgErr != nil {
			logger.Warnf(ctx, "Failed to collect image URLs for KB delete: %v", imgErr)
		}
		var imageInfoStrs []string
		for _, ci := range chunkImageInfos {
			imageInfoStrs = append(imageInfoStrs, ci.ImageInfo)
		}
		imageURLs := collectImageURLs(ctx, imageInfoStrs)

		// Delete all chunks
		logger.Infof(ctx, "Deleting all chunks in knowledge base")
		for _, knowledgeID := range knowledgeIDs {
			if err := s.chunkRepo.DeleteChunksByKnowledgeID(ctx, knowledgeID); err != nil {
				logger.Warnf(ctx, "Failed to delete chunks for knowledge %s: %v", knowledgeID, err)
			}
		}

		// Delete physical files, extracted images, and adjust storage
		logger.Infof(ctx, "Deleting physical files and extracted images")
		storageAdjust := int64(0)
		for _, knowledge := range knowledgeList {
			if knowledge.FilePath != "" {
				if err := s.fileSvc.DeleteFile(ctx, knowledge.FilePath); err != nil {
					logger.Warnf(ctx, "Failed to delete file %s: %v", knowledge.FilePath, err)
				}
			}
			storageAdjust -= knowledge.StorageSize
		}
		deleteExtractedImages(ctx, s.fileSvc, imageURLs)
		if storageAdjust != 0 {
			if err := s.tenantRepo.AdjustStorageUsed(ctx, tenantID, storageAdjust); err != nil {
				logger.Warnf(ctx, "Failed to adjust tenant storage: %v", err)
			}
		}

		// Delete knowledge graph data
		logger.Infof(ctx, "Deleting knowledge graph data")
		namespaces := make([]types.NameSpace, 0, len(knowledgeList))
		for _, knowledge := range knowledgeList {
			namespaces = append(namespaces, types.NameSpace{
				KnowledgeBase: knowledge.KnowledgeBaseID,
				Knowledge:     knowledge.ID,
			})
		}
		if s.graphEngine != nil && len(namespaces) > 0 {
			if err := s.graphEngine.DelGraph(ctx, namespaces); err != nil {
				logger.Warnf(ctx, "Failed to delete knowledge graph: %v", err)
			}
		}

		// Delete all knowledge entries from database
		logger.Infof(ctx, "Deleting knowledge entries from database")
		if err := s.kgRepo.DeleteKnowledgeList(ctx, knowledgeIDs); err != nil {
			logger.ErrorWithFields(ctx, err, map[string]interface{}{
				"knowledge_base_id": kbID,
			})
			return err
		}
	}

	logger.Infof(ctx, "KB delete task completed successfully, knowledge base ID: %s", kbID)
	return nil
}

// CopyKnowledgeBase copies a knowledge base to a new knowledge base (shallow copy).
func (s *knowledgeBaseService) CopyKnowledgeBase(ctx context.Context,
	srcKB string, dstKB string,
) (*types.KnowledgeBase, *types.KnowledgeBase, error) {
	sourceKB, err := s.repo.GetKnowledgeBaseByID(ctx, srcKB)
	if err != nil {
		logger.Errorf(ctx, "Get source knowledge base failed: %v", err)
		return nil, nil, err
	}
	sourceKB.EnsureDefaults()
	var targetKB *types.KnowledgeBase
	if dstKB != "" {
		targetKB, err = s.repo.GetKnowledgeBaseByID(ctx, dstKB)
		if err != nil {
			return nil, nil, err
		}
	} else {
		var faqConfig *types.FAQConfig
		if sourceKB.FAQConfig != nil {
			cfg := *sourceKB.FAQConfig
			faqConfig = &cfg
		}
		targetKB = &types.KnowledgeBase{
			ID:             generateKnowledgeBaseID(),
			Name:           sourceKB.Name,
			Type:           sourceKB.Type,
			Category:       sourceKB.Category,
			Description:    sourceKB.Description,
			ChunkingConfig: sourceKB.ChunkingConfig,
			FAQConfig:      faqConfig,
		}
		targetKB.EnsureDefaults()
		if err := s.repo.CreateKnowledgeBase(ctx, targetKB); err != nil {
			return nil, nil, err
		}
	}
	return sourceKB, targetKB, nil
}
