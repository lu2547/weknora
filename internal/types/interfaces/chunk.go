package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

// ChunkImageInfo holds (knowledge_id, image_info) pairs for image cleanup before chunk deletion.
type ChunkImageInfo struct {
	KnowledgeID string `gorm:"column:knowledge_id"`
	ImageInfo   string `gorm:"column:image_info"`
}

// ChunkRepository defines the interface for chunk repository operations
type ChunkRepository interface {
	CreateChunks(ctx context.Context, chunks []*types.Chunk) error
	GetChunkByID(ctx context.Context, id string) (*types.Chunk, error)
	GetChunkBySeqID(ctx context.Context, seqID int64) (*types.Chunk, error)
	ListChunksByID(ctx context.Context, ids []string) ([]*types.Chunk, error)
	ListChunksBySeqID(ctx context.Context, seqIDs []int64) ([]*types.Chunk, error)
	ListChunksByKnowledgeID(ctx context.Context, knowledgeID string) ([]*types.Chunk, error)
	ListPagedChunksByKnowledgeID(
		ctx context.Context,
		knowledgeID string,
		page *types.Pagination,
		chunkType []types.ChunkType,
		tagID string,
		keyword string,
		searchField string,
		sortOrder string,
		knowledgeType string,
	) ([]*types.Chunk, int64, error)
	ListChunkByParentID(ctx context.Context, parentID string) ([]*types.Chunk, error)
	ListChunksByParentIDs(ctx context.Context, parentIDs []string) ([]*types.Chunk, error)
	UpdateChunk(ctx context.Context, chunk *types.Chunk) error
	UpdateChunks(ctx context.Context, chunks []*types.Chunk) error
	DeleteChunk(ctx context.Context, id string) error
	DeleteChunks(ctx context.Context, ids []string) error
	DeleteChunksByKnowledgeID(ctx context.Context, knowledgeID string) error
	DeleteByKnowledgeList(ctx context.Context, knowledgeIDs []string) error
	ListImageInfoByKnowledgeIDs(ctx context.Context, knowledgeIDs []string) ([]ChunkImageInfo, error)
	MoveChunksByKnowledgeID(ctx context.Context, knowledgeID string, targetKBID string) error
	DeleteChunksByTagID(ctx context.Context, kbID string, tagID string, excludeIDs []string) ([]string, error)
	CountChunksByKnowledgeBaseID(ctx context.Context, kbID string) (int64, error)
	DeleteUnindexedChunks(ctx context.Context, knowledgeID string) ([]*types.Chunk, error)
	ListAllFAQChunksByKnowledgeID(ctx context.Context, knowledgeID string) ([]*types.Chunk, error)
	ListAllFAQChunksWithMetadataByKnowledgeBaseID(ctx context.Context, kbID string) ([]*types.Chunk, error)
	FindFAQChunkWithDuplicateQuestion(ctx context.Context, kbID string, excludeChunkID string, questions []string) (*types.Chunk, error)
	ListAllFAQChunksForExport(ctx context.Context, knowledgeID string) ([]*types.Chunk, error)
	UpdateChunkFlagsBatch(ctx context.Context, kbID string, setFlags map[string]types.ChunkFlags, clearFlags map[string]types.ChunkFlags) error
	UpdateChunkFieldsByTagID(ctx context.Context, kbID string, tagID string, isEnabled *bool, setFlags types.ChunkFlags, clearFlags types.ChunkFlags, newTagID *string, excludeIDs []string) ([]string, error)
	FAQChunkDiff(ctx context.Context, srcKBID string, dstKBID string) (chunksToAdd []string, chunksToDelete []string, err error)
	ListRecommendedFAQChunks(ctx context.Context, kbIDs []string, knowledgeIDs []string, limit int) ([]*types.Chunk, error)
	ListRecentDocumentChunksWithQuestions(ctx context.Context, kbIDs []string, knowledgeIDs []string, limit int) ([]*types.Chunk, error)
}

// ChunkService defines the interface for chunk service operations
type ChunkService interface {
	CreateChunks(ctx context.Context, chunks []*types.Chunk) error
	GetChunkByID(ctx context.Context, id string) (*types.Chunk, error)
	GetChunkByIDOnly(ctx context.Context, id string) (*types.Chunk, error) // alias for GetChunkByID (tenant_id removed)
	ListChunksByKnowledgeID(ctx context.Context, knowledgeID string) ([]*types.Chunk, error)
	ListPagedChunksByKnowledgeID(
		ctx context.Context,
		knowledgeID string,
		page *types.Pagination,
		chunkType []types.ChunkType,
	) (*types.PageResult, error)
	UpdateChunk(ctx context.Context, chunk *types.Chunk) error
	UpdateChunks(ctx context.Context, chunks []*types.Chunk) error
	DeleteChunk(ctx context.Context, id string) error
	DeleteChunks(ctx context.Context, ids []string) error
	DeleteChunksByKnowledgeID(ctx context.Context, knowledgeID string) error
	DeleteByKnowledgeList(ctx context.Context, ids []string) error
	ListChunkByParentID(ctx context.Context, parentID string) ([]*types.Chunk, error)
	GetRepository() ChunkRepository
	DeleteGeneratedQuestion(ctx context.Context, chunkID string, questionID string) error
}
