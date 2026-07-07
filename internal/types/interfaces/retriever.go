package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/models/embedding"
	"github.com/Tencent/WeKnora/internal/types"
)

// RetrieveEngine defines the retrieve engine interface
type RetrieveEngine interface {
	// EngineType gets the retrieve engine type
	EngineType() types.RetrieverEngineType

	// Retrieve executes the retrieve
	Retrieve(ctx context.Context, params types.RetrieveParams) ([]*types.RetrieveResult, error)

	// Support gets the supported retrieve types
	Support() []types.RetrieverType
}

// RetrieveEngineRepository defines the retrieve engine repository interface
type RetrieveEngineRepository interface {
	// Save saves the index info
	Save(ctx context.Context, indexInfo *types.IndexInfo, params map[string]any) error

	// BatchSave saves the index info list
	BatchSave(ctx context.Context, indexInfoList []*types.IndexInfo, params map[string]any) error

	// EstimateStorageSize estimates the storage size
	EstimateStorageSize(ctx context.Context, indexInfoList []*types.IndexInfo, params map[string]any) int64

	// DeleteByChunkIDList deletes the index info by chunk id list
	DeleteByChunkIDList(ctx context.Context, knowledgeBaseID string, indexIDList []string, dimension int, knowledgeType string) error
	// DeleteBySourceIDList deletes the index info by source id list
	DeleteBySourceIDList(ctx context.Context, knowledgeBaseID string, sourceIDList []string, dimension int, knowledgeType string) error
	// DropKnowledgeBaseCollection drops the per-knowledge-base collection (used when deleting a KB)
	DropKnowledgeBaseCollection(ctx context.Context, knowledgeBaseID string) error
	// EnsureCollection eagerly provisions the collection that this KB will write into.
	// Intended to be called right after CreateKnowledgeBase so users see the Milvus
	// collection (personal/public/enterprise) appear immediately, instead of waiting
	// for the first document upload to lazily create it.
	// Backends without a collection concept (sqlite/pg/es) implement this as a no-op.
	EnsureCollection(ctx context.Context, knowledgeBaseID string, dimension int) error
	// 复制索引数据
	// sourceKnowledgeBaseID: 源知识库ID
	// sourceToTargetChunkIDMap: 源分块ID到目标分块ID的映射关系
	// targetKnowledgeBaseID: 目标知识库ID
	// params: 额外参数，如向量表示等
	CopyIndices(
		ctx context.Context,
		sourceKnowledgeBaseID string,
		sourceToTargetKBIDMap map[string]string,
		sourceToTargetChunkIDMap map[string]string,
		targetKnowledgeBaseID string,
		dimension int,
		knowledgeType string,
	) error

	// DeleteByKnowledgeIDList deletes the index info by knowledge id list
	DeleteByKnowledgeIDList(ctx context.Context, knowledgeBaseID string, knowledgeIDList []string, dimension int, knowledgeType string) error

	// BatchUpdateChunkEnabledStatus updates the enabled status of chunks in batch within a single KB
	// chunkStatusMap: map of chunk ID to enabled status (true = enabled, false = disabled)
	BatchUpdateChunkEnabledStatus(ctx context.Context, knowledgeBaseID string, chunkStatusMap map[string]bool) error

	// BatchUpdateChunkTagID updates the tag ID (and path) of chunks in batch within a single KB
	// chunkTagMap: map of chunk ID -> target tag meta (ID+Path). Empty TagID/TagPath means clearing tag.
	BatchUpdateChunkTagID(ctx context.Context, knowledgeBaseID string, chunkTagMap map[string]types.ChunkTagUpdate) error

	// RetrieveEngine retrieves the engine
	RetrieveEngine
}

// RetrieveEngineRegistry defines the retrieve engine registry interface
type RetrieveEngineRegistry interface {
	// Register registers the retrieve engine service
	Register(indexService RetrieveEngineService) error
	// GetRetrieveEngineService gets the retrieve engine service
	GetRetrieveEngineService(engineType types.RetrieverEngineType) (RetrieveEngineService, error)
	// GetAllRetrieveEngineServices gets all retrieve engine services
	GetAllRetrieveEngineServices() []RetrieveEngineService
}

// RetrieveEngineService defines the retrieve engine service interface
type RetrieveEngineService interface {
	// Index indexes the index info
	Index(ctx context.Context,
		embedder embedding.Embedder,
		indexInfo *types.IndexInfo,
		retrieverTypes []types.RetrieverType,
	) error

	// BatchIndex indexes the index info list
	BatchIndex(ctx context.Context,
		embedder embedding.Embedder,
		indexInfoList []*types.IndexInfo,
		retrieverTypes []types.RetrieverType,
	) error

	// EstimateStorageSize estimates the storage size
	EstimateStorageSize(ctx context.Context,
		embedder embedding.Embedder,
		indexInfoList []*types.IndexInfo,
		retrieverTypes []types.RetrieverType,
	) int64
	// CopyIndices 从源知识库复制索引到目标知识库，免去重新计算嵌入向量的开销
	// sourceKnowledgeBaseID: 源知识库ID
	// sourceToTargetChunkIDMap: 源分块ID到目标分块ID的映射关系，key为源分块ID，value为目标分块ID
	// targetKnowledgeBaseID: 目标知识库ID
	CopyIndices(
		ctx context.Context,
		sourceKnowledgeBaseID string,
		sourceToTargetKBIDMap map[string]string,
		sourceToTargetChunkIDMap map[string]string,
		targetKnowledgeBaseID string,
		dimension int,
		knowledgeType string,
	) error

	// DeleteByChunkIDList deletes the index info by chunk id list
	DeleteByChunkIDList(ctx context.Context, knowledgeBaseID string, indexIDList []string, dimension int, knowledgeType string) error

	// DeleteBySourceIDList deletes the index info by source id list
	DeleteBySourceIDList(ctx context.Context, knowledgeBaseID string, sourceIDList []string, dimension int, knowledgeType string) error

	// DeleteByKnowledgeIDList deletes the index info by knowledge id list
	DeleteByKnowledgeIDList(ctx context.Context, knowledgeBaseID string, knowledgeIDList []string, dimension int, knowledgeType string) error

	// DropKnowledgeBaseCollection drops the per-knowledge-base collection (used when deleting a KB)
	DropKnowledgeBaseCollection(ctx context.Context, knowledgeBaseID string) error

	// EnsureCollection eagerly provisions the underlying collection for this KB.
	// Service-layer counterpart of RetrieveEngineRepository.EnsureCollection.
	EnsureCollection(ctx context.Context, knowledgeBaseID string, dimension int) error

	// BatchUpdateChunkEnabledStatus updates the enabled status of chunks in batch within a single KB
	// chunkStatusMap: map of chunk ID to enabled status (true = enabled, false = disabled)
	BatchUpdateChunkEnabledStatus(ctx context.Context, knowledgeBaseID string, chunkStatusMap map[string]bool) error

	// BatchUpdateChunkTagID updates the tag ID (and path) of chunks in batch within a single KB
	// chunkTagMap: map of chunk ID -> target tag meta (ID+Path). Empty TagID/TagPath means clearing tag.
	BatchUpdateChunkTagID(ctx context.Context, knowledgeBaseID string, chunkTagMap map[string]types.ChunkTagUpdate) error

	// RetrieveEngine retrieves the engine
	RetrieveEngine
}
