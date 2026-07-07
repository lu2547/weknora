package milvus

import (
	"context"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/milvus-io/milvus/client/v2/column"
	"github.com/milvus-io/milvus/client/v2/entity"
	"github.com/milvus-io/milvus/client/v2/index"
	client "github.com/milvus-io/milvus/client/v2/milvusclient"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

const (
	envMilvusCollection   = "MILVUS_COLLECTION"
	envMilvusMetricType   = "MILVUS_METRIC_TYPE"
	defaultCollectionName = "weknora_kb"
	fieldContent          = "content"
	fieldChunkID          = "chunk_id"
	fieldKnowledgeID      = "knowledge_id"
	fieldKnowledgeBaseID  = "knowledge_base_id"
	fieldTagID            = "tag_id"
	fieldEmbedding        = "embedding"
	fieldIsEnabled        = "is_enabled"
	fieldID               = "id"
	fieldContentSparse    = "content_sparse"
	fieldFileName         = "file_name"
)

var (
	// allFields 严格对齐 docs/milvus_collection 中 chunk 类集合的字段。
	// id / content / chunk_id / knowledge_id / knowledge_base_id / tag_id(Array) /
	// is_enabled / embedding / file_name。
	allFields = []string{fieldID, fieldContent, fieldChunkID,
		fieldKnowledgeID, fieldKnowledgeBaseID, fieldTagID, fieldIsEnabled, fieldEmbedding, fieldFileName}
)

// NewMilvusRetrieveEngineRepository creates and initializes a new Milvus repository
func NewMilvusRetrieveEngineRepository(client *client.Client, kbLookup KBLookup) interfaces.RetrieveEngineRepository {
	log := logger.GetLogger(context.Background())
	log.Info("[Milvus] Initializing Milvus retriever engine repository")

	collectionBaseName := os.Getenv(envMilvusCollection)
	if collectionBaseName == "" {
		log.Warn("[Milvus] MILVUS_COLLECTION environment variable not set, using default collection name")
		collectionBaseName = defaultCollectionName
	}

	metricType := entity.IP
	if mt := os.Getenv(envMilvusMetricType); mt != "" {
		switch strings.ToUpper(mt) {
		case "COSINE":
			metricType = entity.COSINE
		case "L2":
			metricType = entity.L2
		case "IP":
			metricType = entity.IP
		default:
			log.Warnf("[Milvus] Unknown MILVUS_METRIC_TYPE '%s', using default IP", mt)
		}
	}
	log.Infof("[Milvus] Using metric type: %s", metricType)

	res := &milvusRepository{
		filter:             filter{},
		client:             client,
		collectionBaseName: collectionBaseName,
		metricType:         metricType,
		kbLookup:           kbLookup,
	}

	log.Info("[Milvus] Successfully initialized repository")
	return res
}

// rememberKBInfo caches the minimal KB metadata needed to resolve collection
// names. Idempotent; safe to call from any write/search hot path.
func (m *milvusRepository) rememberKBInfo(info KBCollectionInfo) {
	if info.ID == "" || info.Category == "" {
		return
	}
	m.kbInfoCache.Store(info.ID, info)
}

// resolveKBInfo returns the KBCollectionInfo for a given KB ID. It hits the
// in-memory cache first and falls back to KBLookup (DB) on miss.
func (m *milvusRepository) resolveKBInfo(ctx context.Context, kbID string) (KBCollectionInfo, error) {
	if kbID == "" {
		return KBCollectionInfo{}, fmt.Errorf("resolveKBInfo: kbID is empty")
	}
	if cached, ok := m.kbInfoCache.Load(kbID); ok {
		if info, ok := cached.(KBCollectionInfo); ok && info.Category != "" {
			return info, nil
		}
	}
	if m.kbLookup == nil {
		return KBCollectionInfo{}, fmt.Errorf("resolveKBInfo: kb %s not in cache and no KBLookup configured", kbID)
	}
	kb, err := m.kbLookup.GetKnowledgeBaseByID(ctx, kbID)
	if err != nil {
		return KBCollectionInfo{}, fmt.Errorf("resolveKBInfo: lookup kb %s: %w", kbID, err)
	}
	info := FromKnowledgeBase(kb)
	m.rememberKBInfo(info)
	return info, nil
}

// getCollectionName returns the embedding collection name for a KB. It always
// goes through CollectionResolver (three-tier KB architecture); the legacy
// per-KB "weknora_kb_<sanitized_id>" naming is no longer used.
func (m *milvusRepository) getCollectionName(ctx context.Context, knowledgeBaseID string) (string, error) {
	info, err := m.resolveKBInfo(ctx, knowledgeBaseID)
	if err != nil {
		return "", err
	}
	return EmbeddingCollectionByMeta(info)
}

// sanitizeCollectionSuffix 将 KB ID 中 Milvus 不允许的字符（例如 '-'）替换为 '_'
func sanitizeCollectionSuffix(s string) string {
	if s == "" {
		return "default"
	}
	replacer := strings.NewReplacer("-", "_", ".", "_", "/", "_")
	return replacer.Replace(s)
}

// ensureCollection ensures the per-knowledge-base collection exists for the given dimension
func (m *milvusRepository) ensureCollection(ctx context.Context, knowledgeBaseID string, dimension int) error {
	if knowledgeBaseID == "" {
		return fmt.Errorf("ensureCollection: knowledgeBaseID is empty")
	}
	if dimension <= 0 {
		return fmt.Errorf("ensureCollection: invalid dimension %d for kb %s", dimension, knowledgeBaseID)
	}
	collectionName, err := m.getCollectionName(ctx, knowledgeBaseID)
	if err != nil {
		return fmt.Errorf("ensureCollection: resolve collection name: %w", err)
	}

	// Check cache first
	if _, ok := m.initializedCollections.Load(collectionName); ok {
		return nil
	}

	log := logger.GetLogger(ctx)

	// Check if collection exists
	hasCollection, err := m.client.HasCollection(ctx, client.NewHasCollectionOption(collectionName))
	if err != nil {
		log.Errorf("[Milvus] Failed to check collection existence: %v", err)
		return fmt.Errorf("failed to check collection existence: %w", err)
	}

	if hasCollection {
		// Check whether the existing collection has the BM25 function bound.
		// If not (i.e., collection was created before full-text search support was added),
		// drop and recreate it so the BM25 function and sparse index are in place.
		// WARNING: this destroys all data in the collection; re-indexing is required.
		if needsRecreate, checkErr := m.collectionLacksBM25Function(ctx, collectionName); checkErr != nil {
			log.Warnf("[Milvus] Could not verify BM25 schema for %s, skipping recreate: %v", collectionName, checkErr)
		} else if needsRecreate {
			log.Warnf("[Milvus] Collection %s is missing BM25 function — dropping and recreating. All vectors will be lost and must be re-indexed.", collectionName)
			if dropErr := m.client.DropCollection(ctx, client.NewDropCollectionOption(collectionName)); dropErr != nil {
				log.Errorf("[Milvus] Failed to drop outdated collection %s: %v", collectionName, dropErr)
				return fmt.Errorf("failed to drop outdated collection: %w", dropErr)
			}
			hasCollection = false
			log.Infof("[Milvus] Dropped outdated collection %s, will recreate with BM25 schema", collectionName)
		}
	}

	if !hasCollection {
		log.Infof("[Milvus] Creating collection %s for KB %s with dimension %d", collectionName, knowledgeBaseID, dimension)

		// Define schema — 严格对齐 docs/milvus_collection 中 chunk 类集合的权威 schema。
		// id(VarChar 64, PK) / embedding(FloatVector) / chunk_id(VarChar 64) /
		// knowledge_id(VarChar 64) / knowledge_base_id(VarChar 64) /
		// tag_id(Array<VarChar>) / file_name(VarChar 255) / is_enabled(Bool) /
		// content(VarChar 65535, BM25 输入) / content_sparse(SparseFloatVector, BM25 输出)。
		schema := &entity.Schema{
			CollectionName: collectionName,
			Description:    fmt.Sprintf("WeKnora embeddings collection for KB %s, dimension %d", knowledgeBaseID, dimension),
			AutoID:         false,
			Fields: []*entity.Field{
				entity.NewField().
					WithName(fieldID).
					WithDataType(entity.FieldTypeVarChar).
					WithIsPrimaryKey(true).
					WithMaxLength(64),
				entity.NewField().
					WithName(fieldEmbedding).
					WithDataType(entity.FieldTypeFloatVector).
					WithDim(int64(dimension)),
				entity.NewField().
					WithName(fieldContent).
					WithDataType(entity.FieldTypeVarChar).
					WithMaxLength(65535).
					WithEnableAnalyzer(true).
					WithEnableMatch(true),
				entity.NewField().
					WithName(fieldContentSparse).
					WithDataType(entity.FieldTypeSparseVector),
				entity.NewField().
					WithName(fieldChunkID).
					WithDataType(entity.FieldTypeVarChar).
					WithMaxLength(64),
				entity.NewField().
					WithName(fieldKnowledgeID).
					WithDataType(entity.FieldTypeVarChar).
					WithMaxLength(64),
				entity.NewField().
					WithName(fieldKnowledgeBaseID).
					WithDataType(entity.FieldTypeVarChar).
					WithMaxLength(64),
				// tag_id 是 Array<VarChar>，平铺存储从当前 tag 到祖先 root 的 id_knowledge_tag 链。
				entity.NewField().
					WithName(fieldTagID).
					WithDataType(entity.FieldTypeArray).
					WithElementType(entity.FieldTypeVarChar).
					WithMaxCapacity(1024).
					WithMaxLength(64).
					WithNullable(true),
				entity.NewField().
					WithName(fieldIsEnabled).
					WithDataType(entity.FieldTypeBool),
				entity.NewField().
					WithName(fieldFileName).
					WithDataType(entity.FieldTypeVarChar).
					WithMaxLength(255),
			},
		}

		// Add BM25 function for content sparse vector
		// ref: https://milvus.io/docs/zh/full-text-search.md
		schema.WithFunction(entity.NewFunction().
			WithName("text_bm25_emb").
			WithInputFields(fieldContent).
			WithOutputFields(fieldContentSparse).
			WithType(entity.FunctionTypeBM25))

		indexOpts := make([]client.CreateIndexOption, 0)
		// hnsw index for embedding field
		indexOpts = append(indexOpts, client.NewCreateIndexOption(collectionName, fieldEmbedding, index.NewHNSWIndex(m.metricType, 16, 128)))
		indexOpts = append(indexOpts, client.NewCreateIndexOption(collectionName, fieldContentSparse, index.NewAutoIndex(entity.BM25)))
		// Scalar payload 索引用 AUTOINDEX；tag_id 为 Array 类型使用 INVERTED 索引以支持 ARRAY_CONTAINS_ANY。
		indexFields := []string{fieldChunkID, fieldKnowledgeID, fieldKnowledgeBaseID, fieldIsEnabled}
		for _, fieldName := range indexFields {
			indexOpts = append(indexOpts, client.NewCreateIndexOption(collectionName, fieldName, index.NewAutoIndex(entity.IP)))
		}
		indexOpts = append(indexOpts, client.NewCreateIndexOption(collectionName, fieldTagID, index.NewInvertedIndex()))

		// Create collection
		err = m.client.CreateCollection(ctx, client.NewCreateCollectionOption(collectionName, schema).WithIndexOptions(indexOpts...))
		if err != nil {
			log.Errorf("[Milvus] Failed to create collection: %v", err)
			return fmt.Errorf("failed to create collection: %w", err)
		}

		log.Infof("[Milvus] Successfully created collection %s", collectionName)
	}

	loadTask, err := m.client.LoadCollection(ctx, client.NewLoadCollectionOption(collectionName))
	if err != nil {
		log.Errorf("[Milvus] Failed to load collection: %v", err)
		return fmt.Errorf("failed to load collection: %w", err)
	}
	if err := loadTask.Await(ctx); err != nil {
		log.Errorf("[Milvus] Failed to await load collection: %v", err)
		return fmt.Errorf("failed to await load collection: %w", err)
	}

	// Mark as initialized
	m.initializedCollections.Store(collectionName, true)
	return nil
}

// EnsureCollection eagerly provisions the per-KB Milvus collection so that personal/public/enterprise
// collections appear immediately after CreateKnowledgeBase, instead of being lazy-created on first write.
func (m *milvusRepository) EnsureCollection(ctx context.Context, knowledgeBaseID string, dimension int) error {
	return m.ensureCollection(ctx, knowledgeBaseID, dimension)
}

// DropKnowledgeBaseCollection removes a KB's footprint from Milvus.
// For enterprise KBs (one-collection-per-KB), it drops the entire collection.
// For personal/public KBs (shared collections), it instead deletes only the
// entities belonging to this KB so other KBs sharing the same collection are
// not affected.
func (m *milvusRepository) DropKnowledgeBaseCollection(ctx context.Context, knowledgeBaseID string) error {
	log := logger.GetLogger(ctx)
	if knowledgeBaseID == "" {
		return nil
	}
	info, err := m.resolveKBInfo(ctx, knowledgeBaseID)
	if err != nil {
		return fmt.Errorf("DropKnowledgeBaseCollection: resolve kb info: %w", err)
	}
	collectionName, err := EmbeddingCollectionByMeta(info)
	if err != nil {
		return fmt.Errorf("DropKnowledgeBaseCollection: resolve collection name: %w", err)
	}
	has, err := m.client.HasCollection(ctx, client.NewHasCollectionOption(collectionName))
	if err != nil {
		return fmt.Errorf("failed to check collection existence: %w", err)
	}
	if !has {
		return nil
	}
	if info.Category == types.KnowledgeBaseCategoryEnterprise {
		if err := m.client.DropCollection(ctx, client.NewDropCollectionOption(collectionName)); err != nil {
			log.Errorf("[Milvus] Failed to drop collection %s: %v", collectionName, err)
			return fmt.Errorf("failed to drop collection: %w", err)
		}
		m.initializedCollections.Delete(collectionName)
		m.kbInfoCache.Delete(knowledgeBaseID)
		log.Infof("[Milvus] Dropped enterprise collection %s for KB %s", collectionName, knowledgeBaseID)
		return nil
	}
	// Shared collection (personal/public): delete only this KB's entities
	expr := fmt.Sprintf("%s == \"%s\"", fieldKnowledgeBaseID, knowledgeBaseID)
	if _, err := m.client.Delete(ctx, client.NewDeleteOption(collectionName).WithExpr(expr)); err != nil {
		log.Errorf("[Milvus] Failed to delete entities from %s with expr %q: %v", collectionName, expr, err)
		return fmt.Errorf("failed to delete shared-collection entities: %w", err)
	}
	m.kbInfoCache.Delete(knowledgeBaseID)
	log.Infof("[Milvus] Removed entities from shared collection %s for KB %s", collectionName, knowledgeBaseID)
	return nil
}

func (m *milvusRepository) EngineType() types.RetrieverEngineType {
	return types.MilvusRetrieverEngineType
}

// collectionLacksBM25Function returns true when the collection exists but has no BM25 function
// bound to the content_sparse field. This indicates the collection was created before full-text
// search support was added and needs to be dropped and recreated.
func (m *milvusRepository) collectionLacksBM25Function(ctx context.Context, collectionName string) (bool, error) {
	coll, err := m.client.DescribeCollection(ctx, client.NewDescribeCollectionOption(collectionName))
	if err != nil {
		return false, fmt.Errorf("describe collection: %w", err)
	}
	if coll.Schema == nil {
		return true, nil
	}
	for _, fn := range coll.Schema.Functions {
		if fn != nil && fn.Type == entity.FunctionTypeBM25 {
			return false, nil
		}
	}
	return true, nil
}

func (m *milvusRepository) Support() []types.RetrieverType {
	return []types.RetrieverType{types.KeywordsRetrieverType, types.VectorRetrieverType}
}

// EstimateStorageSize calculates the estimated storage size for a list of indices
func (m *milvusRepository) EstimateStorageSize(ctx context.Context,
	indexInfoList []*types.IndexInfo, params map[string]any,
) int64 {
	var totalStorageSize int64
	for _, embedding := range indexInfoList {
		embeddingDB := toMilvusVectorEmbedding(embedding, params)
		totalStorageSize += m.calculateStorageSize(embeddingDB)
	}
	logger.GetLogger(ctx).Infof(
		"[Milvus] Storage size for %d indices: %d bytes", len(indexInfoList), totalStorageSize,
	)
	return totalStorageSize
}

// Save stores a single point in Milvus
func (m *milvusRepository) Save(ctx context.Context,
	embedding *types.IndexInfo,
	additionalParams map[string]any,
) error {
	log := logger.GetLogger(ctx)
	log.Debugf("[Milvus] Saving index for chunk ID: %s", embedding.ChunkID)

	if embedding.KnowledgeBaseID == "" {
		return fmt.Errorf("Save: empty KnowledgeBaseID for chunk %s", embedding.ChunkID)
	}

	// Prime the KB metadata cache with whatever the upper layer attached so we
	// can avoid an extra DB lookup when resolving the target collection name.
	if embedding.KBCategory != "" {
		m.rememberKBInfo(KBCollectionInfo{
			ID:              embedding.KnowledgeBaseID,
			Category:        embedding.KBCategory,
			CreatedAtUnixMs: embedding.KBCreatedAtUnixMs,
		})
	}

	embeddingDB := toMilvusVectorEmbedding(embedding, additionalParams)
	if len(embeddingDB.Embedding) == 0 {
		err := fmt.Errorf("empty embedding vector for chunk ID: %s", embedding.ChunkID)
		log.Errorf("[Milvus] %v", err)
		return err
	}

	dimension := len(embeddingDB.Embedding)
	if err := m.ensureCollection(ctx, embedding.KnowledgeBaseID, dimension); err != nil {
		return err
	}

	collectionName, err := m.getCollectionName(ctx, embedding.KnowledgeBaseID)
	if err != nil {
		return fmt.Errorf("Save: resolve collection name: %w", err)
	}

	embeddingDB.ID = uuid.New().String()
	opts := createUpsert(collectionName, []*MilvusVectorEmbedding{embeddingDB})

	_, err = m.client.Upsert(ctx, opts)
	if err != nil {
		log.Errorf("[Milvus] Failed to save index: %v", err)
		return err
	}

	log.Infof("[Milvus] Successfully saved index for chunk ID: %s", embedding.ChunkID)
	return nil
}

// BatchSave stores multiple points in Milvus using batch insert
func (m *milvusRepository) BatchSave(ctx context.Context,
	embeddingList []*types.IndexInfo, additionalParams map[string]any,
) error {
	log := logger.GetLogger(ctx)
	if len(embeddingList) == 0 {
		log.Warn("[Milvus] Empty list provided to BatchSave, skipping")
		return nil
	}

	log.Infof("[Milvus] Batch saving %d indices", len(embeddingList))

	// Group points by (knowledgeBaseID, dimension) for per-KB collections
	type kbDimKey struct {
		kbID string
		dim  int
	}
	embeddingsByKey := make(map[kbDimKey][]*types.IndexInfo)

	for _, embedding := range embeddingList {
		if embedding.KnowledgeBaseID == "" {
			log.Warnf("[Milvus] Skipping index with empty KnowledgeBaseID for chunk ID: %s", embedding.ChunkID)
			continue
		}
		// Prime KB metadata cache from the upper layer to skip a DB round-trip
		// when resolving the target collection name below.
		if embedding.KBCategory != "" {
			m.rememberKBInfo(KBCollectionInfo{
				ID:              embedding.KnowledgeBaseID,
				Category:        embedding.KBCategory,
				CreatedAtUnixMs: embedding.KBCreatedAtUnixMs,
			})
		}
		embeddingDB := toMilvusVectorEmbedding(embedding, additionalParams)
		if len(embeddingDB.Embedding) == 0 {
			log.Warnf("[Milvus] Skipping empty embedding for chunk ID: %s", embedding.ChunkID)
			continue
		}

		key := kbDimKey{kbID: embedding.KnowledgeBaseID, dim: len(embeddingDB.Embedding)}
		embeddingsByKey[key] = append(embeddingsByKey[key], embedding)
		log.Debugf("[Milvus] Added chunk ID %s to batch request (kb=%s, dim=%d)", embedding.ChunkID, key.kbID, key.dim)
	}

	if len(embeddingsByKey) == 0 {
		log.Warn("[Milvus] No valid points to save after filtering")
		return nil
	}

	// Save points to each (kb, dimension)-specific collection
	totalSaved := 0
	for key, embeddings := range embeddingsByKey {
		if err := m.ensureCollection(ctx, key.kbID, key.dim); err != nil {
			return err
		}

		collectionName, err := m.getCollectionName(ctx, key.kbID)
		if err != nil {
			return fmt.Errorf("BatchSave: resolve collection name: %w", err)
		}
		n := len(embeddings)
		embeddingDBList := make([]*MilvusVectorEmbedding, 0, n)

		for _, embedding := range embeddings {
			embeddingDB := toMilvusVectorEmbedding(embedding, additionalParams)
			embeddingDB.ID = uuid.New().String()
			embeddingDBList = append(embeddingDBList, embeddingDB)
		}
		opts := createUpsert(collectionName, embeddingDBList)
		_, err = m.client.Upsert(ctx, opts)
		if err != nil {
			log.Errorf("[Milvus] Failed to execute batch operation for kb %s dim %d: %v", key.kbID, key.dim, err)
			return fmt.Errorf("failed to batch save (kb %s, dim %d): %w", key.kbID, key.dim, err)
		}
		totalSaved += n
		log.Infof("[Milvus] Saved %d points to collection %s", n, collectionName)
	}

	log.Infof("[Milvus] Successfully batch saved %d indices", totalSaved)
	return nil
}

// DeleteByChunkIDList removes points from the collection based on chunk IDs
func (m *milvusRepository) DeleteByChunkIDList(ctx context.Context, knowledgeBaseID string, chunkIDList []string, dimension int, knowledgeType string) error {
	log := logger.GetLogger(ctx)
	if len(chunkIDList) == 0 {
		log.Warn("[Milvus] Empty chunk ID list provided for deletion, skipping")
		return nil
	}
	if knowledgeBaseID == "" {
		return fmt.Errorf("DeleteByChunkIDList: empty knowledgeBaseID")
	}

	collectionName, err := m.getCollectionName(ctx, knowledgeBaseID)
	if err != nil {
		return fmt.Errorf("DeleteByChunkIDList: resolve collection name: %w", err)
	}
	hasCollection, err := m.client.HasCollection(ctx, client.NewHasCollectionOption(collectionName))
	if err != nil {
		return fmt.Errorf("failed to check collection existence: %w", err)
	}
	if !hasCollection {
		log.Warnf("[Milvus] Collection %s does not exist, skipping delete by chunk IDs", collectionName)
		return nil
	}
	log.Infof("[Milvus] Deleting indices by chunk IDs from %s, count: %d", collectionName, len(chunkIDList))

	deleteOpt := client.NewDeleteOption(collectionName)
	deleteOpt.WithStringIDs(fieldChunkID, chunkIDList)
	_, err = m.client.Delete(ctx, deleteOpt)
	if err != nil {
		log.Errorf("[Milvus] Failed to delete by chunk IDs: %v", err)
		return fmt.Errorf("failed to delete by chunk IDs: %w", err)
	}

	log.Infof("[Milvus] Successfully deleted documents by chunk IDs")
	return nil
}

// DeleteByKnowledgeIDList removes points from the collection based on knowledge IDs
func (m *milvusRepository) DeleteByKnowledgeIDList(ctx context.Context,
	knowledgeBaseID string, knowledgeIDList []string, dimension int, knowledgeType string,
) error {
	log := logger.GetLogger(ctx)
	if len(knowledgeIDList) == 0 {
		log.Warn("[Milvus] Empty knowledge ID list provided for deletion, skipping")
		return nil
	}
	if knowledgeBaseID == "" {
		return fmt.Errorf("DeleteByKnowledgeIDList: empty knowledgeBaseID")
	}

	collectionName, err := m.getCollectionName(ctx, knowledgeBaseID)
	if err != nil {
		return fmt.Errorf("DeleteByKnowledgeIDList: resolve collection name: %w", err)
	}
	hasCollection, err := m.client.HasCollection(ctx, client.NewHasCollectionOption(collectionName))
	if err != nil {
		return fmt.Errorf("failed to check collection existence: %w", err)
	}
	if !hasCollection {
		log.Warnf("[Milvus] Collection %s does not exist, skipping delete by knowledge IDs", collectionName)
		return nil
	}
	log.Infof("[Milvus] Deleting indices by knowledge IDs from %s, count: %d", collectionName, len(knowledgeIDList))

	deleteOpt := client.NewDeleteOption(collectionName)
	deleteOpt.WithStringIDs(fieldKnowledgeID, knowledgeIDList)
	_, err = m.client.Delete(ctx, deleteOpt)
	if err != nil {
		log.Errorf("[Milvus] Failed to delete by knowledge IDs: %v", err)
		return fmt.Errorf("failed to delete by knowledge IDs: %w", err)
	}

	log.Infof("[Milvus] Successfully deleted documents by knowledge IDs")
	return nil
}

// DeleteBySourceIDList removes points from the collection based on source IDs
func (m *milvusRepository) DeleteBySourceIDList(ctx context.Context,
	knowledgeBaseID string, sourceIDList []string, dimension int, knowledgeType string,
) error {
	log := logger.GetLogger(ctx)
	if len(sourceIDList) == 0 {
		log.Warn("[Milvus] Empty source ID list provided for deletion, skipping")
		return nil
	}
	if knowledgeBaseID == "" {
		return fmt.Errorf("DeleteBySourceIDList: empty knowledgeBaseID")
	}

	collectionName, err := m.getCollectionName(ctx, knowledgeBaseID)
	if err != nil {
		return fmt.Errorf("DeleteBySourceIDList: resolve collection name: %w", err)
	}
	hasCollection, err := m.client.HasCollection(ctx, client.NewHasCollectionOption(collectionName))
	if err != nil {
		return fmt.Errorf("failed to check collection existence: %w", err)
	}
	if !hasCollection {
		log.Warnf("[Milvus] Collection %s does not exist, skipping delete by source IDs", collectionName)
		return nil
	}
	log.Infof("[Milvus] Deleting indices by source IDs from %s, count: %d", collectionName, len(sourceIDList))

	deleteOpt := client.NewDeleteOption(collectionName)
	// 新 schema 不再保留 source_id；source 语义已收敛为 chunk_id。
	deleteOpt.WithStringIDs(fieldChunkID, sourceIDList)
	_, err = m.client.Delete(ctx, deleteOpt)
	if err != nil {
		log.Errorf("[Milvus] Failed to delete by source IDs: %v", err)
		return fmt.Errorf("failed to delete by source IDs: %w", err)
	}

	log.Infof("[Milvus] Successfully deleted documents by source IDs")
	return nil
}

// BatchUpdateChunkEnabledStatus updates the enabled status of chunks in batch within the given KB's collection
func (m *milvusRepository) BatchUpdateChunkEnabledStatus(ctx context.Context, knowledgeBaseID string, chunkStatusMap map[string]bool) error {
	log := logger.GetLogger(ctx)
	if len(chunkStatusMap) == 0 {
		log.Warn("[Milvus] Empty chunk status map provided, skipping")
		return nil
	}
	if knowledgeBaseID == "" {
		return fmt.Errorf("BatchUpdateChunkEnabledStatus: empty knowledgeBaseID")
	}

	log.Infof("[Milvus] Batch updating chunk enabled status, count: %d, kb: %s", len(chunkStatusMap), knowledgeBaseID)

	collectionName, err := m.getCollectionName(ctx, knowledgeBaseID)
	if err != nil {
		return fmt.Errorf("BatchUpdateChunkEnabledStatus: resolve collection name: %w", err)
	}
	has, err := m.client.HasCollection(ctx, client.NewHasCollectionOption(collectionName))
	if err != nil {
		return fmt.Errorf("failed to check collection existence: %w", err)
	}
	if !has {
		log.Warnf("[Milvus] Collection %s does not exist, skipping enabled status update", collectionName)
		return nil
	}

	// Group chunks by enabled status for batch updates
	enabledChunkIDs := make([]string, 0)
	disabledChunkIDs := make([]string, 0)

	for chunkID, enabled := range chunkStatusMap {
		if enabled {
			enabledChunkIDs = append(enabledChunkIDs, chunkID)
		} else {
			disabledChunkIDs = append(disabledChunkIDs, chunkID)
		}
	}

	if len(enabledChunkIDs) > 0 {
		enabledEmbeddings, _, err := m.searchByFilter(ctx, collectionName, &universalFilterCondition{
			Field:    fieldChunkID,
			Operator: operatorIn,
			Value:    enabledChunkIDs,
		}, nil, nil)
		if err != nil {
			return fmt.Errorf("failed to search enabled chunks in %s: %w", collectionName, err)
		}
		upsertEmbeddings := make([]*MilvusVectorEmbedding, 0, len(enabledEmbeddings))
		for _, embedding := range enabledEmbeddings {
			embedding.IsEnabled = true
			upsertEmbeddings = append(upsertEmbeddings, &embedding.MilvusVectorEmbedding)
		}
		if len(upsertEmbeddings) > 0 {
			enabledReq := createUpsert(collectionName, upsertEmbeddings)
			if _, err := m.client.Upsert(ctx, enabledReq); err != nil {
				return fmt.Errorf("failed to update enabled chunks in %s: %w", collectionName, err)
			}
		}
	}

	if len(disabledChunkIDs) > 0 {
		disabledEmbeddings, _, err := m.searchByFilter(ctx, collectionName, &universalFilterCondition{
			Field:    fieldChunkID,
			Operator: operatorIn,
			Value:    disabledChunkIDs,
		}, nil, nil)
		if err != nil {
			return fmt.Errorf("failed to search disabled chunks in %s: %w", collectionName, err)
		}
		upsertEmbeddings := make([]*MilvusVectorEmbedding, 0, len(disabledEmbeddings))
		for _, embedding := range disabledEmbeddings {
			embedding.IsEnabled = false
			upsertEmbeddings = append(upsertEmbeddings, &embedding.MilvusVectorEmbedding)
		}
		if len(upsertEmbeddings) > 0 {
			disabledReq := createUpsert(collectionName, upsertEmbeddings)
			if _, err := m.client.Upsert(ctx, disabledReq); err != nil {
				return fmt.Errorf("failed to update disabled chunks in %s: %w", collectionName, err)
			}
		}
	}

	log.Infof("[Milvus] Batch update chunk enabled status completed for kb %s", knowledgeBaseID)
	return nil
}

func (m *milvusRepository) searchByFilter(ctx context.Context, collectionName string, filter *universalFilterCondition, limit, offset *int) ([]*MilvusVectorEmbeddingWithScore, int, error) {
	params, err := m.filter.Convert(filter)
	if err != nil {
		return nil, 0, err
	}
	queryOpt := client.NewQueryOption(collectionName)
	if params.exprStr != "" {
		queryOpt.WithFilter(params.exprStr)
		for k, v := range params.params {
			queryOpt.WithTemplateParam(k, v)
		}
	}
	queryOpt.WithOutputFields("*")
	if limit != nil {
		queryOpt.WithLimit(*limit)
	}
	if offset != nil {
		queryOpt.WithOffset(*offset)
	}
	resultSet, err := m.client.Query(ctx, queryOpt)
	if err != nil {
		return nil, 0, err
	}
	embeddings, _, err := convertResultSet([]client.ResultSet{resultSet})
	if err != nil {
		return nil, 0, err
	}
	return embeddings, resultSet.ResultCount, nil
}

// BatchUpdateChunkTagID updates the tag ID (and path) of chunks in batch within the given KB
func (m *milvusRepository) BatchUpdateChunkTagID(ctx context.Context, knowledgeBaseID string, chunkTagMap map[string]types.ChunkTagUpdate) error {
	log := logger.GetLogger(ctx)
	if len(chunkTagMap) == 0 {
		log.Warn("[Milvus] Empty chunk tag map provided, skipping")
		return nil
	}
	if knowledgeBaseID == "" {
		return fmt.Errorf("BatchUpdateChunkTagID: empty knowledgeBaseID")
	}

	log.Infof("[Milvus] Batch updating chunk tag ID, count: %d, kb: %s", len(chunkTagMap), knowledgeBaseID)

	collectionName, err := m.getCollectionName(ctx, knowledgeBaseID)
	if err != nil {
		return fmt.Errorf("BatchUpdateChunkTagID: resolve collection name: %w", err)
	}
	has, err := m.client.HasCollection(ctx, client.NewHasCollectionOption(collectionName))
	if err != nil {
		return fmt.Errorf("failed to check collection existence: %w", err)
	}
	if !has {
		log.Warnf("[Milvus] Collection %s does not exist, skipping tag id update", collectionName)
		return nil
	}

	// Group chunks by target tag chain key for batch updates. 同一祖先链的 chunk 一批上。
	type tagGroupValue struct {
		tagIDs   []string
		chunkIDs []string
	}
	tagGroups := make(map[string]*tagGroupValue)
	for chunkID, upd := range chunkTagMap {
		key := strings.Join(upd.TagIDs, "\x00")
		g, ok := tagGroups[key]
		if !ok {
			g = &tagGroupValue{tagIDs: upd.TagIDs}
			tagGroups[key] = g
		}
		g.chunkIDs = append(g.chunkIDs, chunkID)
	}

	for _, g := range tagGroups {
		embeddings, _, err := m.searchByFilter(ctx, collectionName, &universalFilterCondition{
			Field:    fieldChunkID,
			Operator: operatorIn,
			Value:    g.chunkIDs,
		}, nil, nil)
		if err != nil {
			return fmt.Errorf("failed to search chunks in %s: %w", collectionName, err)
		}
		upsertEmbeddings := make([]*MilvusVectorEmbedding, 0, len(embeddings))
		for _, embedding := range embeddings {
			embedding.TagIDs = g.tagIDs
			upsertEmbeddings = append(upsertEmbeddings, &embedding.MilvusVectorEmbedding)
		}
		if len(upsertEmbeddings) > 0 {
			req := createUpsert(collectionName, upsertEmbeddings)
			if _, err := m.client.Upsert(ctx, req); err != nil {
				return fmt.Errorf("failed to update chunks in %s: %w", collectionName, err)
			}
		}
	}

	log.Infof("[Milvus] Batch update chunk tag ID completed for kb %s", knowledgeBaseID)
	return nil
}

func (m *milvusRepository) getBaseFilterForQuery(params types.RetrieveParams) (string, map[string]any, error) {
	filters := make([]*universalFilterCondition, 0)
	if len(params.KnowledgeBaseIDs) > 0 {
		filters = append(filters, &universalFilterCondition{
			Field:    fieldKnowledgeBaseID,
			Operator: operatorIn,
			Value:    params.KnowledgeBaseIDs,
		})
	}
	if len(params.KnowledgeIDs) > 0 {
		filters = append(filters, &universalFilterCondition{
			Field:    fieldKnowledgeID,
			Operator: operatorIn,
			Value:    params.KnowledgeIDs,
		})
	}
	if len(params.TagIDs) > 0 {
		// tag_id 是 Array 类型，使用 ARRAY_CONTAINS_ANY 命中任意祖先 / 自身。
		filters = append(filters, &universalFilterCondition{
			Field:    fieldTagID,
			Operator: operatorArrayContainsAny,
			Value:    params.TagIDs,
		})
	}
	if len(params.ExcludeKnowledgeIDs) > 0 {
		filters = append(filters, &universalFilterCondition{
			Field:    fieldKnowledgeID,
			Operator: operatorNotIn,
			Value:    params.ExcludeKnowledgeIDs,
		})
	}
	if len(params.ExcludeChunkIDs) > 0 {
		filters = append(filters, &universalFilterCondition{
			Field:    fieldChunkID,
			Operator: operatorNotIn,
			Value:    params.ExcludeChunkIDs,
		})
	}
	filters = append(filters, &universalFilterCondition{
		Field:    fieldIsEnabled,
		Operator: operatorEqual,
		Value:    true,
	})
	if len(filters) == 0 {
		return "", nil, nil
	}
	f, err := m.filter.Convert(&universalFilterCondition{
		Operator: operatorAnd,
		Value:    filters,
	})
	if err != nil {
		return "", nil, err
	}
	return f.exprStr, f.params, nil
}

// Retrieve dispatches the retrieval operation to the appropriate method based on retriever type
func (m *milvusRepository) Retrieve(ctx context.Context,
	params types.RetrieveParams,
) ([]*types.RetrieveResult, error) {
	log := logger.GetLogger(ctx)
	log.Debugf("[Milvus] Processing retrieval request of type: %s", params.RetrieverType)

	switch params.RetrieverType {
	case types.VectorRetrieverType:
		return m.VectorRetrieve(ctx, params)
	case types.KeywordsRetrieverType:
		return m.KeywordsRetrieve(ctx, params)
	}

	err := fmt.Errorf("invalid retriever type: %v", params.RetrieverType)
	log.Errorf("[Milvus] %v", err)
	return nil, err
}

// VectorRetrieve performs vector similarity search across the knowledge bases' collections
func (m *milvusRepository) VectorRetrieve(ctx context.Context,
	params types.RetrieveParams,
) ([]*types.RetrieveResult, error) {
	log := logger.GetLogger(ctx)
	dimension := len(params.Embedding)
	log.Infof("[Milvus] Vector retrieval: dim=%d, topK=%d, threshold=%.4f, kbIDs=%v",
		dimension, params.TopK, params.Threshold, params.KnowledgeBaseIDs)

	if len(params.KnowledgeBaseIDs) == 0 {
		log.Warn("[Milvus] VectorRetrieve called without KnowledgeBaseIDs, returning empty results")
		return buildRetrieveResult(nil, types.VectorRetrieverType), nil
	}

	// Prime KB metadata cache from RetrieveParams to skip DB lookups when
	// resolving collection names per kbID below.
	for _, kb := range params.KnowledgeBases {
		if kb != nil {
			m.rememberKBInfo(FromKnowledgeBase(kb))
		}
	}

	var sp *index.CustomAnnParam
	if params.Threshold > 0 {
		ann := index.NewCustomAnnParam()
		ann.WithRadius(params.Threshold)
		sp = &ann
	}

	// KnowledgeBaseIDs 已在 expr 中确保过滤 (每个 collection 单独查询时 expr 效果一致）
	expr, paramsMap, err := m.getBaseFilterForQuery(params)
	if err != nil {
		log.Errorf("[Milvus] Failed to build base filter: %v", err)
		return nil, fmt.Errorf("failed to build filter: %w", err)
	}

	var allResults []*types.IndexWithScore
	for _, kbID := range params.KnowledgeBaseIDs {
		collectionName, err := m.getCollectionName(ctx, kbID)
		if err != nil {
			log.Errorf("[Milvus] Failed to resolve collection for kb %s: %v", kbID, err)
			continue
		}
		has, err := m.client.HasCollection(ctx, client.NewHasCollectionOption(collectionName))
		if err != nil {
			log.Errorf("[Milvus] Failed to check collection existence %s: %v", collectionName, err)
			continue
		}
		if !has {
			log.Warnf("[Milvus] Collection %s does not exist, skipping", collectionName)
			continue
		}

		searchOption := client.NewSearchOption(collectionName, params.TopK, []entity.Vector{entity.FloatVector(params.Embedding)})
		searchOption.WithANNSField(fieldEmbedding)
		if sp != nil {
			searchOption.WithAnnParam(sp)
		}
		if expr != "" {
			searchOption.WithFilter(expr)
			for k, v := range paramsMap {
				searchOption.WithTemplateParam(k, v)
			}
		}
		searchOption.WithOutputFields("*")
		resultSet, err := m.client.Search(ctx, searchOption)
		if err != nil {
			log.Errorf("[Milvus] Vector search failed on %s: %v", collectionName, err)
			continue
		}
		sets, scores, err := convertResultSet(resultSet)
		if err != nil {
			log.Errorf("[Milvus] Failed to convert result set from %s: %v", collectionName, err)
			continue
		}
		for i, set := range sets {
			set.Score = scores[i]
			allResults = append(allResults, fromMilvusVectorEmbedding(set.ID, set, types.MatchTypeEmbedding))
		}
	}

	// Sort by score (desc) and limit to topK
	if len(allResults) > 1 {
		sortIndexByScoreDesc(allResults)
	}
	if len(allResults) > params.TopK {
		allResults = allResults[:params.TopK]
	}
	if len(allResults) == 0 {
		log.Warnf("[Milvus] No vector matches found that meet threshold %.4f", params.Threshold)
	} else {
		log.Infof("[Milvus] Vector retrieval found %d results", len(allResults))
		log.Debugf("[Milvus] Top result score: %.4f", allResults[0].Score)
	}
	return buildRetrieveResult(allResults, types.VectorRetrieverType), nil
}

// KeywordsRetrieve performs keyword-based search in document content across KB collections
func (m *milvusRepository) KeywordsRetrieve(ctx context.Context,
	params types.RetrieveParams,
) ([]*types.RetrieveResult, error) {
	log := logger.GetLogger(ctx)
	log.Infof("[Milvus] Performing keywords retrieval with query: %s, topK: %d, kbIDs: %v", params.Query, params.TopK, params.KnowledgeBaseIDs)

	if len(params.KnowledgeBaseIDs) == 0 {
		log.Warn("[Milvus] KeywordsRetrieve called without KnowledgeBaseIDs, returning empty results")
		return buildRetrieveResult(nil, types.KeywordsRetrieverType), nil
	}

	// Prime KB metadata cache from RetrieveParams to avoid DB lookups when
	// resolving collection names per kbID below.
	for _, kb := range params.KnowledgeBases {
		if kb != nil {
			m.rememberKBInfo(FromKnowledgeBase(kb))
		}
	}

	expr, paramsMap, err := m.getBaseFilterForQuery(params)
	if err != nil {
		log.Errorf("[Milvus] Failed to build base filter: %v", err)
		return nil, fmt.Errorf("failed to build filter: %w", err)
	}

	var allResults []*types.IndexWithScore
	for _, kbID := range params.KnowledgeBaseIDs {
		collectionName, err := m.getCollectionName(ctx, kbID)
		if err != nil {
			log.Errorf("[Milvus] Failed to resolve collection for kb %s: %v", kbID, err)
			continue
		}
		has, err := m.client.HasCollection(ctx, client.NewHasCollectionOption(collectionName))
		if err != nil {
			log.Errorf("[Milvus] Failed to check collection existence %s: %v", collectionName, err)
			continue
		}
		if !has {
			log.Warnf("[Milvus] Collection %s does not exist, skipping", collectionName)
			continue
		}
		searchOpt := client.NewSearchOption(collectionName, params.TopK, []entity.Vector{entity.Text(params.Query)})
		searchOpt.WithANNSField(fieldContentSparse)
		if expr != "" {
			searchOpt.WithFilter(expr)
			for k, v := range paramsMap {
				searchOpt.WithTemplateParam(k, v)
			}
		}
		searchOpt.WithOutputFields("*")
		resultSet, err := m.client.Search(ctx, searchOpt)
		if err != nil {
			log.Errorf("[Milvus] Keywords search failed on %s: %v", collectionName, err)
			continue
		}
		sets, _, err := convertResultSet(resultSet)
		if err != nil {
			log.Errorf("[Milvus] Failed to convert result set from %s: %v", collectionName, err)
			continue
		}
		for _, set := range sets {
			set.Score = 1.0
			allResults = append(allResults, fromMilvusVectorEmbedding(set.ID, set, types.MatchTypeKeywords))
		}
	}

	// Limit results to topK
	if len(allResults) > params.TopK {
		allResults = allResults[:params.TopK]
	}

	if len(allResults) == 0 {
		log.Warnf("[Milvus] No keyword matches found for query: %s", params.Query)
	} else {
		log.Infof("[Milvus] Keywords retrieval found %d results", len(allResults))
	}

	return buildRetrieveResult(allResults, types.KeywordsRetrieverType), nil
}

// sortIndexByScoreDesc sorts IndexWithScore list by Score descending (stable)
func sortIndexByScoreDesc(list []*types.IndexWithScore) {
	for i := 1; i < len(list); i++ {
		for j := i; j > 0 && list[j].Score > list[j-1].Score; j-- {
			list[j], list[j-1] = list[j-1], list[j]
		}
	}
}

// CopyIndices copies index data from source knowledge base to target knowledge base
func (m *milvusRepository) CopyIndices(ctx context.Context,
	sourceKnowledgeBaseID string,
	sourceToTargetKBIDMap map[string]string,
	sourceToTargetChunkIDMap map[string]string,
	targetKnowledgeBaseID string,
	dimension int,
	knowledgeType string,
) error {
	log := logger.GetLogger(ctx)
	log.Infof(
		"[Milvus] Copying indices from source knowledge base %s to target knowledge base %s, count: %d, dimension: %d",
		sourceKnowledgeBaseID, targetKnowledgeBaseID, len(sourceToTargetChunkIDMap), dimension,
	)

	if len(sourceToTargetChunkIDMap) == 0 {
		log.Warn("[Milvus] Empty mapping, skipping copy")
		return nil
	}
	if sourceKnowledgeBaseID == "" || targetKnowledgeBaseID == "" {
		return fmt.Errorf("CopyIndices: empty source/target knowledgeBaseID")
	}

	sourceCollection, err := m.getCollectionName(ctx, sourceKnowledgeBaseID)
	if err != nil {
		return fmt.Errorf("failed to resolve source collection: %w", err)
	}
	targetCollection, err := m.getCollectionName(ctx, targetKnowledgeBaseID)
	if err != nil {
		return fmt.Errorf("failed to resolve target collection: %w", err)
	}

	// Verify source collection exists
	hasSource, err := m.client.HasCollection(ctx, client.NewHasCollectionOption(sourceCollection))
	if err != nil {
		return fmt.Errorf("failed to check source collection existence: %w", err)
	}
	if !hasSource {
		log.Warnf("[Milvus] Source collection %s does not exist, nothing to copy", sourceCollection)
		return nil
	}

	// Ensure target collection exists
	if err := m.ensureCollection(ctx, targetKnowledgeBaseID, dimension); err != nil {
		return err
	}

	batchSize := 64
	totalCopied := 0
	var offset *int
	for {
		sourceEmbeddings, count, err := m.searchByFilter(ctx, sourceCollection, &universalFilterCondition{
			Field:    fieldKnowledgeBaseID,
			Operator: operatorEqual,
			Value:    sourceKnowledgeBaseID,
		}, &batchSize, offset)
		if err != nil {
			log.Errorf("[Milvus] Failed to query source points: %v", err)
			return err
		}
		if len(sourceEmbeddings) == 0 {
			break
		}
		targetEmbeddings := make([]*MilvusVectorEmbedding, 0, len(sourceEmbeddings))
		for _, sourceEmbedding := range sourceEmbeddings {
			sourceChunkID := sourceEmbedding.ChunkID
			sourceKnowledgeID := sourceEmbedding.KnowledgeID

			targetChunkID, ok := sourceToTargetChunkIDMap[sourceChunkID]
			if !ok {
				log.Warnf("[Milvus] Source chunk %s not found in target mapping, skipping", sourceChunkID)
				continue
			}
			targetKnowledgeID, ok := sourceToTargetKBIDMap[sourceKnowledgeID]
			if !ok {
				log.Warnf("[Milvus] Source knowledge %s not found in target mapping, skipping", sourceKnowledgeID)
				continue
			}
			targetEmbedding := &MilvusVectorEmbedding{
				ID:              uuid.New().String(),
				Content:         sourceEmbedding.Content,
				ChunkID:         targetChunkID,
				KnowledgeID:     targetKnowledgeID,
				KnowledgeBaseID: targetKnowledgeBaseID,
				TagIDs:          sourceEmbedding.TagIDs,
				FileName:        sourceEmbedding.FileName,
				Embedding:       sourceEmbedding.Embedding,
				IsEnabled:       sourceEmbedding.IsEnabled,
			}
			targetEmbeddings = append(targetEmbeddings, targetEmbedding)
		}
		if len(targetEmbeddings) > 0 {
			opts := createUpsert(targetCollection, targetEmbeddings)
			_, err := m.client.Upsert(ctx, opts)
			if err != nil {
				log.Errorf("[Milvus] Failed to batch upsert target points: %v", err)
				return err
			}
			totalCopied += len(targetEmbeddings)
			log.Infof("[Milvus] Successfully copied batch, batch size: %d, total copied: %d",
				len(targetEmbeddings), totalCopied)
		}

		if count < batchSize {
			break
		}
		if offset == nil {
			offset = new(int)
		}
		*offset += count
	}

	log.Infof("[Milvus] Index copy completed, total copied: %d", totalCopied)
	return nil
}

func buildRetrieveResult(results []*types.IndexWithScore, retrieverType types.RetrieverType) []*types.RetrieveResult {
	return []*types.RetrieveResult{
		{
			Results:             results,
			RetrieverEngineType: types.MilvusRetrieverEngineType,
			RetrieverType:       retrieverType,
			Error:               nil,
		},
	}
}

func (m *milvusRepository) calculateStorageSize(embedding *MilvusVectorEmbedding) int64 {
	// Payload fields
	payloadSizeBytes := int64(0)
	payloadSizeBytes += int64(len(embedding.Content))         // content string
	payloadSizeBytes += int64(len(embedding.ChunkID))         // chunk_id string
	payloadSizeBytes += int64(len(embedding.KnowledgeID))     // knowledge_id string
	payloadSizeBytes += int64(len(embedding.KnowledgeBaseID)) // knowledge_base_id string
	payloadSizeBytes += int64(len(embedding.FileName))        // file_name string
	// tag_id Array<VarChar>：按祖先链平铺后的字节总和估算
	for _, t := range embedding.TagIDs {
		payloadSizeBytes += int64(len(t))
	}
	payloadSizeBytes += 1 // is_enabled bool

	// Vector storage and index
	var vectorSizeBytes int64 = 0
	var indexBytes int64 = 0
	if embedding.Embedding != nil {
		dimensions := int64(len(embedding.Embedding))
		vectorSizeBytes = dimensions * 4

		// IVF_FLAT index: vectors are duplicated in inverted lists (grouped by cluster),
		// plus a small per-vector overhead for cluster assignment and list management.
		// The centroid table (nlist × dim × 4) is a shared structure and should NOT
		// be counted per-vector.
		indexBytes = vectorSizeBytes + 16
	}

	// ID tracker and metadata: ~32 bytes per vector
	const metadataBytes int64 = 32

	totalSizeBytes := payloadSizeBytes + vectorSizeBytes + indexBytes + metadataBytes
	return totalSizeBytes
}

// toMilvusVectorEmbedding converts IndexInfo to Milvus format
func toMilvusVectorEmbedding(embedding *types.IndexInfo, additionalParams map[string]interface{}) *MilvusVectorEmbedding {
	vector := &MilvusVectorEmbedding{
		Content:         embedding.Content,
		ChunkID:         embedding.ChunkID,
		KnowledgeID:     embedding.KnowledgeID,
		KnowledgeBaseID: embedding.KnowledgeBaseID,
		TagIDs:          embedding.TagIDs,
		FileName:        embedding.FileName,
		IsEnabled:       embedding.IsEnabled,
	}
	if additionalParams != nil && slices.Contains(slices.Collect(maps.Keys(additionalParams)), fieldEmbedding) {
		if embeddingMap, ok := additionalParams[fieldEmbedding].(map[string][]float32); ok {
			vector.Embedding = embeddingMap[embedding.SourceID]
		}
	}
	return vector
}

// fromMilvusVectorEmbedding converts Milvus result to IndexWithScore domain model
func fromMilvusVectorEmbedding(id string,
	embedding *MilvusVectorEmbeddingWithScore,
	matchType types.MatchType,
) *types.IndexWithScore {
	return &types.IndexWithScore{
		ID:              id,
		SourceID:        embedding.ChunkID,
		SourceType:      types.ChunkSourceType,
		ChunkID:         embedding.ChunkID,
		KnowledgeID:     embedding.KnowledgeID,
		KnowledgeBaseID: embedding.KnowledgeBaseID,
		TagIDs:          embedding.TagIDs,
		Content:         embedding.Content,
		Score:           embedding.Score,
		MatchType:       matchType,
	}
}

func createUpsert(collectionName string, embeddings []*MilvusVectorEmbedding) client.UpsertOption {
	ids := make([]string, 0, len(embeddings))
	embeddingsData := make([][]float32, 0, len(embeddings))
	contents := make([]string, 0, len(embeddings))
	chunkIDs := make([]string, 0, len(embeddings))
	knowledgeIDs := make([]string, 0, len(embeddings))
	knowledgeBaseIDs := make([]string, 0, len(embeddings))
	// tag_id Array<VarChar>：每行一个祖先链数组。nil/空数组表示本行无标签。
	tagIDs := make([][]string, 0, len(embeddings))
	fileNames := make([]string, 0, len(embeddings))
	isEnableds := make([]bool, 0, len(embeddings))
	var dimension int
	for _, embedding := range embeddings {
		ids = append(ids, embedding.ID)
		embeddingsData = append(embeddingsData, embedding.Embedding)
		contents = append(contents, embedding.Content)
		chunkIDs = append(chunkIDs, embedding.ChunkID)
		knowledgeIDs = append(knowledgeIDs, embedding.KnowledgeID)
		knowledgeBaseIDs = append(knowledgeBaseIDs, embedding.KnowledgeBaseID)
		tagIDs = append(tagIDs, embedding.TagIDs)
		fileNames = append(fileNames, embedding.FileName)
		isEnableds = append(isEnableds, embedding.IsEnabled)
		dimension = len(embedding.Embedding)
	}
	opt := client.NewColumnBasedInsertOption(collectionName).
		WithVarcharColumn(fieldID, ids).
		WithFloatVectorColumn(fieldEmbedding, dimension, embeddingsData).
		WithVarcharColumn(fieldContent, contents).
		WithVarcharColumn(fieldChunkID, chunkIDs).
		WithVarcharColumn(fieldKnowledgeID, knowledgeIDs).
		WithVarcharColumn(fieldKnowledgeBaseID, knowledgeBaseIDs).
		WithVarcharColumn(fieldFileName, fileNames).
		WithBoolColumn(fieldIsEnabled, isEnableds).
		WithColumns(column.NewColumnVarCharArray(fieldTagID, tagIDs))
	return opt
}

func convertResultSet(resultSet []client.ResultSet) ([]*MilvusVectorEmbeddingWithScore, []float64, error) {
	var results []*MilvusVectorEmbeddingWithScore
	var scores []float64
	if len(resultSet) == 0 {
		return results, scores, nil
	}
	set := resultSet[0]
	resultLen := set.Len()
	if resultLen == 0 {
		return results, scores, nil
	}

	for _, score := range set.Scores {
		scores = append(scores, float64(score))
	}
	docs := make([]*MilvusVectorEmbeddingWithScore, 0, resultLen)
	for i := 0; i < resultLen; i++ {
		docs = append(docs, &MilvusVectorEmbeddingWithScore{})
	}
	for _, field := range allFields {
		columns := set.GetColumn(field)
		if columns == nil || columns.Len() == 0 {
			continue
		}
		if field == fieldID {
			for i := 0; i < columns.Len(); i++ {
				val, err := columns.GetAsString(i)
				if err != nil {
					return nil, nil, err
				}
				docs[i].ID = val
			}
		}
		if field == fieldContent {
			for i := 0; i < columns.Len(); i++ {
				val, err := columns.GetAsString(i)
				if err != nil {
					return nil, nil, err
				}
				docs[i].Content = val
			}
		}
		if field == fieldChunkID {
			for i := 0; i < columns.Len(); i++ {
				val, err := columns.GetAsString(i)
				if err != nil {
					return nil, nil, err
				}
				docs[i].ChunkID = val
			}
		}
		if field == fieldKnowledgeID {
			for i := 0; i < columns.Len(); i++ {
				val, err := columns.GetAsString(i)
				if err != nil {
					return nil, nil, err
				}
				docs[i].KnowledgeID = val
			}
		}
		if field == fieldKnowledgeBaseID {
			for i := 0; i < columns.Len(); i++ {
				val, err := columns.GetAsString(i)
				if err != nil {
					return nil, nil, err
				}
				docs[i].KnowledgeBaseID = val
			}
		}
		if field == fieldTagID {
			// tag_id 是 Array<VarChar>，使用 ColumnVarCharArray.Value(i) 拿到 []string
			arrCol, ok := columns.(*column.ColumnVarCharArray)
			if !ok {
				continue
			}
			for i := 0; i < arrCol.Len(); i++ {
				val, err := arrCol.Value(i)
				if err != nil {
					return nil, nil, fmt.Errorf("get tag_id array failed: %w", err)
				}
				docs[i].TagIDs = val
			}
		}
		if field == fieldIsEnabled {
			for i := 0; i < columns.Len(); i++ {
				val, err := columns.GetAsBool(i)
				if err != nil {
					return nil, nil, err
				}
				docs[i].IsEnabled = val
			}
		}
		if field == fieldEmbedding {
			vectorColumn, ok := columns.(*column.ColumnDoubleArray)
			if !ok {
				continue
			}
			for i := 0; i < vectorColumn.Len(); i++ {
				val, err := vectorColumn.Value(i)
				if err != nil {
					return nil, nil, fmt.Errorf("get vector failed: %w", err)
				}
				embedding := make([]float32, len(val))
				for j, v := range val {
					embedding[j] = float32(v)
				}
				docs[i].Embedding = embedding
			}
		}
	}
	return docs, scores, nil
}
