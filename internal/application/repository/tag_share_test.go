package repository

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupTagShareDB creates an in-memory SQLite database with the
// knowledge_tag_share table.
func setupTagShareDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&types.KnowledgeTagShare{}))
	return db
}

func TestKnowledgeTagShareRepository_CreateAndGet(t *testing.T) {
	db := setupTagShareDB(t)
	repo := NewKnowledgeTagShareRepository(db)
	ctx := context.Background()

	share := &types.KnowledgeTagShare{
		KnowledgeTagID: "tag-1",
		GroupKey:       "org-1",
		SharedByUserID: "user-1",
		Permission:     "viewer",
	}
	require.NoError(t, repo.Create(ctx, share))

	// BeforeCreate hook should populate ID.
	assert.NotEmpty(t, share.ID, "BeforeCreate must generate a UUID")

	// GetByID round-trip.
	got, err := repo.GetByID(ctx, share.ID)
	require.NoError(t, err)
	assert.Equal(t, share.KnowledgeTagID, got.KnowledgeTagID)
	assert.Equal(t, share.GroupKey, got.GroupKey)
	assert.Equal(t, "viewer", got.Permission)

	// GetByTagAndGroup.
	got2, err := repo.GetByTagAndGroup(ctx, "tag-1", "org-1")
	require.NoError(t, err)
	assert.Equal(t, share.ID, got2.ID)
}

func TestKnowledgeTagShareRepository_GetByTagAndGroup_NotFound(t *testing.T) {
	db := setupTagShareDB(t)
	repo := NewKnowledgeTagShareRepository(db)

	_, err := repo.GetByTagAndGroup(context.Background(), "tag-x", "org-x")
	require.Error(t, err)
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func TestKnowledgeTagShareRepository_Update(t *testing.T) {
	db := setupTagShareDB(t)
	repo := NewKnowledgeTagShareRepository(db)
	ctx := context.Background()

	share := &types.KnowledgeTagShare{
		KnowledgeTagID: "tag-1",
		GroupKey:       "org-1",
		Permission:     "viewer",
	}
	require.NoError(t, repo.Create(ctx, share))

	share.Permission = "editor"
	require.NoError(t, repo.Update(ctx, share))

	reloaded, err := repo.GetByID(ctx, share.ID)
	require.NoError(t, err)
	assert.Equal(t, "editor", reloaded.Permission)
}

func TestKnowledgeTagShareRepository_Delete(t *testing.T) {
	db := setupTagShareDB(t)
	repo := NewKnowledgeTagShareRepository(db)
	ctx := context.Background()

	share := &types.KnowledgeTagShare{KnowledgeTagID: "t", GroupKey: "g"}
	require.NoError(t, repo.Create(ctx, share))

	require.NoError(t, repo.Delete(ctx, share.ID))

	_, err := repo.GetByID(ctx, share.ID)
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func TestKnowledgeTagShareRepository_DeleteByTagID(t *testing.T) {
	db := setupTagShareDB(t)
	repo := NewKnowledgeTagShareRepository(db)
	ctx := context.Background()

	require.NoError(t, repo.Create(ctx, &types.KnowledgeTagShare{KnowledgeTagID: "tag-A", GroupKey: "org-1"}))
	require.NoError(t, repo.Create(ctx, &types.KnowledgeTagShare{KnowledgeTagID: "tag-A", GroupKey: "org-2"}))
	require.NoError(t, repo.Create(ctx, &types.KnowledgeTagShare{KnowledgeTagID: "tag-B", GroupKey: "org-1"}))

	require.NoError(t, repo.DeleteByTagID(ctx, "tag-A"))

	a, err := repo.ListByTagID(ctx, "tag-A")
	require.NoError(t, err)
	assert.Empty(t, a, "tag-A shares must be removed")

	b, err := repo.ListByTagID(ctx, "tag-B")
	require.NoError(t, err)
	assert.Len(t, b, 1, "tag-B shares must remain untouched")
}

func TestKnowledgeTagShareRepository_ListByTagID(t *testing.T) {
	db := setupTagShareDB(t)
	repo := NewKnowledgeTagShareRepository(db)
	ctx := context.Background()

	require.NoError(t, repo.Create(ctx, &types.KnowledgeTagShare{KnowledgeTagID: "tag-1", GroupKey: "org-1"}))
	require.NoError(t, repo.Create(ctx, &types.KnowledgeTagShare{KnowledgeTagID: "tag-1", GroupKey: "org-2"}))
	require.NoError(t, repo.Create(ctx, &types.KnowledgeTagShare{KnowledgeTagID: "tag-2", GroupKey: "org-1"}))

	got, err := repo.ListByTagID(ctx, "tag-1")
	require.NoError(t, err)
	assert.Len(t, got, 2)
	for _, s := range got {
		assert.Equal(t, "tag-1", s.KnowledgeTagID)
	}
}

func TestKnowledgeTagShareRepository_ListByGroupKey(t *testing.T) {
	db := setupTagShareDB(t)
	repo := NewKnowledgeTagShareRepository(db)
	ctx := context.Background()

	require.NoError(t, repo.Create(ctx, &types.KnowledgeTagShare{KnowledgeTagID: "tag-1", GroupKey: "org-1"}))
	require.NoError(t, repo.Create(ctx, &types.KnowledgeTagShare{KnowledgeTagID: "tag-2", GroupKey: "org-1"}))
	require.NoError(t, repo.Create(ctx, &types.KnowledgeTagShare{KnowledgeTagID: "tag-3", GroupKey: "org-2"}))

	got, err := repo.ListByGroupKey(ctx, "org-1")
	require.NoError(t, err)
	assert.Len(t, got, 2)

	got2, err := repo.ListByGroupKey(ctx, "org-2")
	require.NoError(t, err)
	assert.Len(t, got2, 1)
	assert.Equal(t, "tag-3", got2[0].KnowledgeTagID)
}

func TestKnowledgeTagShareRepository_GetByID_NotFound(t *testing.T) {
	db := setupTagShareDB(t)
	repo := NewKnowledgeTagShareRepository(db)

	_, err := repo.GetByID(context.Background(), "non-existent")
	require.Error(t, err)
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
}
