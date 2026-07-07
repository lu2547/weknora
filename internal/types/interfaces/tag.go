package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/hibiken/asynq"
)

// KnowledgeTagService defines operations on knowledge base scoped tags.
type KnowledgeTagService interface {
	ListTags(ctx context.Context, kbID string, page *types.Pagination, keyword string) (*types.PageResult, error)
	ListTagTree(ctx context.Context, kbID string) ([]*types.KnowledgeTagTreeNode, error)
	CreateTag(ctx context.Context, kbID string, parentID string, name string, sort int) (*types.KnowledgeTag, error)
	UpdateTag(ctx context.Context, id string, name *string, sort *int) (*types.KnowledgeTag, error)
	MoveTag(ctx context.Context, id string, newParentID string) (*types.KnowledgeTag, error)
	DeleteTag(ctx context.Context, id string, force bool, contentOnly bool, excludeIDs []string) error
	FindOrCreateTagByName(ctx context.Context, kbID string, name string) (*types.KnowledgeTag, error)
	ProcessIndexDelete(ctx context.Context, t *asynq.Task) error
	ShareTag(ctx context.Context, tagID string, groupKey string, sharedByUserID string, permission string) (*types.KnowledgeTagShare, error)
	RevokeTagShare(ctx context.Context, shareID string, userID string) error
	ListTagShares(ctx context.Context, tagID string) ([]*types.KnowledgeTagShare, error)
}

// KnowledgeTagRepository defines persistence operations for tags.
type KnowledgeTagRepository interface {
	Create(ctx context.Context, tag *types.KnowledgeTag) error
	Update(ctx context.Context, tag *types.KnowledgeTag) error
	GetByID(ctx context.Context, id string) (*types.KnowledgeTag, error)
	GetByIDs(ctx context.Context, ids []string) ([]*types.KnowledgeTag, error)
	GetByName(ctx context.Context, kbID string, parentID string, name string) (*types.KnowledgeTag, error)
	ListByKB(
		ctx context.Context,
		kbID string,
		page *types.Pagination,
		keyword string,
	) ([]*types.KnowledgeTag, int64, error)
	ListAllByKB(ctx context.Context, kbID string) ([]*types.KnowledgeTag, error)
	ListChildren(ctx context.Context, parentID string) ([]*types.KnowledgeTag, error)
	Delete(ctx context.Context, id string) error
	CountReferences(
		ctx context.Context,
		kbID string,
		tagID string,
	) (knowledgeCount int64, chunkCount int64, err error)
	BatchCountReferences(
		ctx context.Context,
		kbID string,
		tagIDs []string,
	) (map[string]types.TagReferenceCounts, error)
	DeleteUnusedTags(ctx context.Context, kbID string) (int64, error)
	// AncestorIDs 返回从根节点到该标签节点的祖先链平铺 id 列表（含自身，顺序 root→leaf）。
	// 供向量库写入 tag_id Array 字段使用：例如路径 a/b/c 返回 [id_a, id_b, id_c]。
	// id 为空串返回 nil；查不到或查询失败返回 error。
	AncestorIDs(ctx context.Context, id string) ([]string, error)
}

// KnowledgeTagShareRepository defines persistence operations for tag shares.
type KnowledgeTagShareRepository interface {
	Create(ctx context.Context, share *types.KnowledgeTagShare) error
	GetByID(ctx context.Context, id string) (*types.KnowledgeTagShare, error)
	// GetByTagAndGroup looks up a share by (tagID, groupKey).
	GetByTagAndGroup(ctx context.Context, tagID string, groupKey string) (*types.KnowledgeTagShare, error)
	Update(ctx context.Context, share *types.KnowledgeTagShare) error
	Delete(ctx context.Context, id string) error
	// DeleteByTagID removes all shares for a tag (e.g. when tag is deleted).
	DeleteByTagID(ctx context.Context, tagID string) error
	// ListByTagID lists all share records of a tag.
	ListByTagID(ctx context.Context, tagID string) ([]*types.KnowledgeTagShare, error)
	// ListByGroupKey lists all share records granted to a group (e.g. organization id).
	ListByGroupKey(ctx context.Context, groupKey string) ([]*types.KnowledgeTagShare, error)
}
