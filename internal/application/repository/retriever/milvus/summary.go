package milvus

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/milvus-io/milvus/client/v2/column"
	"github.com/milvus-io/milvus/client/v2/entity"
	"github.com/milvus-io/milvus/client/v2/index"
	client "github.com/milvus-io/milvus/client/v2/milvusclient"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

// ---------------------------------------------------------------------------
// Global summary_knowledge_base collection.
//
// 严格对齐 docs/milvus_collection 中的权威 summary schema：
//   id (VarChar 64, PK)                    — 独立 UUID，便于幂等 upsert
//   embedding (FloatVector dim)            — content 的 dense embedding
//   metadata_embedding (FloatVector dim)   — metadata 的 dense embedding, HNSW(IP)
//   knowledge_id (VarChar 64)              — 普通索引字段
//   knowledge_base_id (VarChar 64)         — 普通索引字段
//   tag_id (Array<VarChar 64>)             — 标签祖先链平铺数组
//   file_name (VarChar 255)                — 必填
//   is_enabled (Bool)                      — 启用状态过滤
//   content (VarChar 65535)                — 全文，参与 BM25 + 稠密向量
//   metadata (VarChar 65535)               — JSON {"tag_name":[...],"title":"..."}  参与 BM25
// 自动派生：
//   content_sparse (SparseFloatVector)     — 由内置 BM25 function 从 content 生成
//   metadata_sparse (SparseFloatVector)    — 由内置 BM25 function 从 metadata 生成
//
// title / file_type / created_at 不写入 Milvus，仅存 PG。
// ---------------------------------------------------------------------------

const (
	summaryCollectionName = SummaryCollectionName

	sfID              = "id"
	sfEmbedding       = "embedding"
	sfMetaEmbedding   = "metadata_embedding"
	sfKnowledgeID     = "knowledge_id"
	sfKnowledgeBaseID = "knowledge_base_id"
	sfTagID           = "tag_id"
	sfFileName        = "file_name"
	sfIsEnabled       = "is_enabled"
	sfContent         = "content"
	sfContentSparse   = "content_sparse"
	sfMetadata        = "metadata"
	sfMetadataSparse  = "metadata_sparse"
)

var summaryOutputFields = []string{
	sfID, sfKnowledgeID, sfKnowledgeBaseID, sfTagID, sfFileName, sfIsEnabled, sfContent, sfMetadata,
}

// ensureSummaryCollection creates the global summary collection on first use.
func (m *milvusRepository) ensureSummaryCollection(ctx context.Context, dimension int) error {
	if dimension <= 0 {
		return fmt.Errorf("ensureSummaryCollection: invalid dimension %d", dimension)
	}

	log := logger.GetLogger(ctx)

	if _, ok := m.initializedCollections.Load(summaryCollectionName); ok {
		return nil
	}

	has, err := m.client.HasCollection(ctx, client.NewHasCollectionOption(summaryCollectionName))
	if err != nil {
		return fmt.Errorf("failed to check summary collection existence: %w", err)
	}

	if !has {
		log.Infof("[Milvus] Creating global summary collection %s with dim=%d", summaryCollectionName, dimension)

		schema := &entity.Schema{
			CollectionName: summaryCollectionName,
			Description:    fmt.Sprintf("WeKnora global knowledge summaries, dim %d", dimension),
			AutoID:         false,
			Fields: []*entity.Field{
				entity.NewField().
					WithName(sfID).
					WithDataType(entity.FieldTypeVarChar).
					WithIsPrimaryKey(true).
					WithMaxLength(64),
				entity.NewField().
					WithName(sfEmbedding).
					WithDataType(entity.FieldTypeFloatVector).
					WithDim(int64(dimension)),
				entity.NewField().
					WithName(sfMetaEmbedding).
					WithDataType(entity.FieldTypeFloatVector).
					WithDim(int64(dimension)),
				entity.NewField().
					WithName(sfContent).
					WithDataType(entity.FieldTypeVarChar).
					WithMaxLength(65535).
					WithEnableAnalyzer(true).
					WithEnableMatch(true),
				entity.NewField().
					WithName(sfContentSparse).
					WithDataType(entity.FieldTypeSparseVector),
				entity.NewField().
					WithName(sfMetadata).
					WithDataType(entity.FieldTypeVarChar).
					WithMaxLength(65535).
					WithEnableAnalyzer(true).
					WithEnableMatch(true),
				entity.NewField().
					WithName(sfMetadataSparse).
					WithDataType(entity.FieldTypeSparseVector),
				entity.NewField().
					WithName(sfKnowledgeID).
					WithDataType(entity.FieldTypeVarChar).
					WithMaxLength(64),
				entity.NewField().
					WithName(sfKnowledgeBaseID).
					WithDataType(entity.FieldTypeVarChar).
					WithMaxLength(64),
				// tag_id 平铺存储从当前 tag 到祖先 root 的 id_knowledge_tag 链。
				entity.NewField().
					WithName(sfTagID).
					WithDataType(entity.FieldTypeArray).
					WithElementType(entity.FieldTypeVarChar).
					WithMaxCapacity(1024).
					WithMaxLength(64).
					WithNullable(true),
				entity.NewField().
					WithName(sfFileName).
					WithDataType(entity.FieldTypeVarChar).
					WithMaxLength(255),
				entity.NewField().
					WithName(sfIsEnabled).
					WithDataType(entity.FieldTypeBool),
			},
		}

		// BM25 内置 function：content -> content_sparse
		schema.WithFunction(entity.NewFunction().
			WithName("text_bm25_emb").
			WithInputFields(sfContent).
			WithOutputFields(sfContentSparse).
			WithType(entity.FunctionTypeBM25))

		// BM25 内置 function：metadata -> metadata_sparse
		schema.WithFunction(entity.NewFunction().
			WithName("metadata_bm25_emb").
			WithInputFields(sfMetadata).
			WithOutputFields(sfMetadataSparse).
			WithType(entity.FunctionTypeBM25))

		indexOpts := []client.CreateIndexOption{
			client.NewCreateIndexOption(summaryCollectionName, sfEmbedding, index.NewHNSWIndex(m.metricType, 16, 128)),
			client.NewCreateIndexOption(summaryCollectionName, sfMetaEmbedding, index.NewHNSWIndex(entity.IP, 16, 128)),
			client.NewCreateIndexOption(summaryCollectionName, sfContentSparse, index.NewAutoIndex(entity.BM25)),
			client.NewCreateIndexOption(summaryCollectionName, sfMetadataSparse, index.NewAutoIndex(entity.BM25)),
		}
		for _, f := range []string{sfKnowledgeID, sfKnowledgeBaseID, sfIsEnabled} {
			indexOpts = append(indexOpts, client.NewCreateIndexOption(summaryCollectionName, f, index.NewAutoIndex(entity.IP)))
		}
		// tag_id 为 Array 字段，使用 INVERTED 索引支持 ARRAY_CONTAINS_ANY
		indexOpts = append(indexOpts, client.NewCreateIndexOption(summaryCollectionName, sfTagID, index.NewInvertedIndex()))

		if err := m.client.CreateCollection(ctx,
			client.NewCreateCollectionOption(summaryCollectionName, schema).WithIndexOptions(indexOpts...),
		); err != nil {
			return fmt.Errorf("failed to create summary collection: %w", err)
		}
		log.Infof("[Milvus] Created summary collection %s", summaryCollectionName)
	}

	loadTask, err := m.client.LoadCollection(ctx, client.NewLoadCollectionOption(summaryCollectionName))
	if err != nil {
		return fmt.Errorf("failed to load summary collection: %w", err)
	}
	if err := loadTask.Await(ctx); err != nil {
		return fmt.Errorf("failed to await load summary collection: %w", err)
	}

	m.initializedCollections.Store(summaryCollectionName, true)
	return nil
}

// checkSummaryVectorDimension returns (dim, true, nil) when the collection exists.
func (m *milvusRepository) checkSummaryVectorDimension(ctx context.Context) (int, bool, error) {
	has, err := m.client.HasCollection(ctx, client.NewHasCollectionOption(summaryCollectionName))
	if err != nil {
		return 0, false, err
	}
	if !has {
		return 0, false, nil
	}
	coll, err := m.client.DescribeCollection(ctx, client.NewDescribeCollectionOption(summaryCollectionName))
	if err != nil {
		return 0, false, err
	}
	for _, f := range coll.Schema.Fields {
		if f.Name == sfEmbedding && f.DataType == entity.FieldTypeFloatVector {
			if dimStr, ok := f.TypeParams["dim"]; ok {
				var d int
				if _, err := fmt.Sscanf(dimStr, "%d", &d); err == nil {
					return d, true, nil
				}
			}
		}
	}
	return 0, true, nil
}

// UpsertKnowledgeSummary inserts or replaces one summary row.
func (m *milvusRepository) UpsertKnowledgeSummary(ctx context.Context, item *types.SummaryItem) error {
	log := logger.GetLogger(ctx)
	if item == nil {
		return nil
	}
	if item.KnowledgeID == "" {
		return fmt.Errorf("UpsertKnowledgeSummary: empty KnowledgeID")
	}
	if len(item.Vector) == 0 {
		log.Warnf("[Milvus] Skip UpsertKnowledgeSummary for %s: empty vector", item.KnowledgeID)
		return nil
	}

	// 维度校验
	existingDim, exists, err := m.checkSummaryVectorDimension(ctx)
	if err != nil {
		return fmt.Errorf("failed to inspect summary collection: %w", err)
	}
	if exists && existingDim > 0 && existingDim != len(item.Vector) {
		log.Warnf(
			"[Milvus] Skip UpsertKnowledgeSummary for %s: vector dim %d != existing summary collection dim %d. "+
				"All KBs must share one embedding model for the global summary collection.",
			item.KnowledgeID, len(item.Vector), existingDim,
		)
		return nil
	}

	if err := m.ensureSummaryCollection(ctx, len(item.Vector)); err != nil {
		return err
	}

	// id 独立 UUID；若上层传入则复用，便于幂等
	id := item.ID
	if id == "" {
		id = uuid.New().String()
	}
	// tag_id 始终是 [][]string；nil 写入会按 nullable 处理为 null。
	tagIDs := [][]string{item.TagIDs}

	opt := client.NewColumnBasedInsertOption(summaryCollectionName).
		WithVarcharColumn(sfID, []string{id}).
		WithFloatVectorColumn(sfEmbedding, len(item.Vector), [][]float32{item.Vector}).
		WithFloatVectorColumn(sfMetaEmbedding, len(item.Vector), [][]float32{metadataVector(item)}).
		WithVarcharColumn(sfContent, []string{truncate(item.Content, 65000)}).
		WithVarcharColumn(sfMetadata, []string{truncate(item.Metadata, 65000)}).
		WithVarcharColumn(sfKnowledgeID, []string{item.KnowledgeID}).
		WithVarcharColumn(sfKnowledgeBaseID, []string{item.KnowledgeBaseID}).
		WithVarcharColumn(sfFileName, []string{truncate(item.FileName, 250)}).
		WithBoolColumn(sfIsEnabled, []bool{item.IsEnabled}).
		WithColumns(column.NewColumnVarCharArray(sfTagID, tagIDs))

	if _, err := m.client.Upsert(ctx, opt); err != nil {
		log.Errorf("[Milvus] Failed to upsert summary for knowledge %s: %v", item.KnowledgeID, err)
		return fmt.Errorf("failed to upsert summary: %w", err)
	}
	log.Infof("[Milvus] Upserted summary id=%s knowledge=%s (kb %s)", id, item.KnowledgeID, item.KnowledgeBaseID)
	return nil
}

// DeleteKnowledgeSummary removes all summary rows of one knowledge.
// 因为 PK 是独立 ID，按 knowledge_id 字段过滤删除。
func (m *milvusRepository) DeleteKnowledgeSummary(ctx context.Context, knowledgeID string) error {
	if knowledgeID == "" {
		return nil
	}
	log := logger.GetLogger(ctx)

	has, err := m.client.HasCollection(ctx, client.NewHasCollectionOption(summaryCollectionName))
	if err != nil {
		return fmt.Errorf("failed to check summary collection existence: %w", err)
	}
	if !has {
		return nil
	}

	expr := fmt.Sprintf(`%s == "%s"`, sfKnowledgeID, escapeMilvusString(knowledgeID))
	deleteOpt := client.NewDeleteOption(summaryCollectionName).WithExpr(expr)
	if _, err := m.client.Delete(ctx, deleteOpt); err != nil {
		log.Errorf("[Milvus] Failed to delete summary for knowledge %s: %v", knowledgeID, err)
		return fmt.Errorf("failed to delete summary: %w", err)
	}
	return nil
}

// DeleteKnowledgeBaseSummaries removes all summary rows for a KB.
func (m *milvusRepository) DeleteKnowledgeBaseSummaries(ctx context.Context, knowledgeBaseID string) error {
	if knowledgeBaseID == "" {
		return nil
	}
	log := logger.GetLogger(ctx)

	has, err := m.client.HasCollection(ctx, client.NewHasCollectionOption(summaryCollectionName))
	if err != nil {
		return fmt.Errorf("failed to check summary collection existence: %w", err)
	}
	if !has {
		return nil
	}

	expr := fmt.Sprintf(`%s == "%s"`, sfKnowledgeBaseID, escapeMilvusString(knowledgeBaseID))
	deleteOpt := client.NewDeleteOption(summaryCollectionName).WithExpr(expr)
	if _, err := m.client.Delete(ctx, deleteOpt); err != nil {
		log.Errorf("[Milvus] Failed to delete summaries for kb %s: %v", knowledgeBaseID, err)
		return fmt.Errorf("failed to delete summaries: %w", err)
	}
	log.Infof("[Milvus] Deleted summaries for kb %s", knowledgeBaseID)
	return nil
}

// SearchSummaries performs a dense vector search over the summary collection.
func (m *milvusRepository) SearchSummaries(
	ctx context.Context,
	queryVector []float32,
	topK int,
	filter types.SummaryFilter,
) ([]*types.SummaryHit, error) {
	log := logger.GetLogger(ctx)
	if len(queryVector) == 0 {
		return nil, fmt.Errorf("SearchSummaries: empty query vector")
	}
	if topK <= 0 {
		topK = 10
	}

	has, err := m.client.HasCollection(ctx, client.NewHasCollectionOption(summaryCollectionName))
	if err != nil {
		return nil, fmt.Errorf("failed to check summary collection existence: %w", err)
	}
	if !has {
		log.Info("[Milvus] Summary collection does not exist yet, returning empty results")
		return nil, nil
	}

	if _, ok := m.initializedCollections.Load(summaryCollectionName); !ok {
		if loadTask, err := m.client.LoadCollection(ctx, client.NewLoadCollectionOption(summaryCollectionName)); err == nil {
			_ = loadTask.Await(ctx)
			m.initializedCollections.Store(summaryCollectionName, true)
		}
	}

	expr := buildSummaryFilterExpr(filter)
	searchOpt := client.NewSearchOption(summaryCollectionName, topK, []entity.Vector{entity.FloatVector(queryVector)})
	searchOpt.WithANNSField(sfEmbedding)
	if expr != "" {
		searchOpt.WithFilter(expr)
	}
	searchOpt.WithOutputFields(summaryOutputFields...)

	resultSet, err := m.client.Search(ctx, searchOpt)
	if err != nil {
		return nil, fmt.Errorf("summary search failed: %w", err)
	}

	hits := convertSummaryResultSet(resultSet)
	log.Infof("[Milvus] Summary search returned %d hits (filter=%q)", len(hits), expr)
	return hits, nil
}

// DropSummaryCollection deletes the entire global summary collection.
func (m *milvusRepository) DropSummaryCollection(ctx context.Context) error {
	log := logger.GetLogger(ctx)
	has, err := m.client.HasCollection(ctx, client.NewHasCollectionOption(summaryCollectionName))
	if err != nil {
		return fmt.Errorf("failed to check summary collection existence: %w", err)
	}
	if !has {
		return nil
	}
	if err := m.client.DropCollection(ctx, client.NewDropCollectionOption(summaryCollectionName)); err != nil {
		return fmt.Errorf("failed to drop summary collection: %w", err)
	}
	m.initializedCollections.Delete(summaryCollectionName)
	log.Infof("[Milvus] Dropped summary collection %s", summaryCollectionName)
	return nil
}

// InspectSummaryCollection reports whether the summary collection exists and its dim.
func (m *milvusRepository) InspectSummaryCollection(ctx context.Context) (bool, int, error) {
	has, err := m.client.HasCollection(ctx, client.NewHasCollectionOption(summaryCollectionName))
	if err != nil {
		return false, 0, fmt.Errorf("failed to check summary collection existence: %w", err)
	}
	if !has {
		return false, 0, nil
	}
	coll, err := m.client.DescribeCollection(ctx, client.NewDescribeCollectionOption(summaryCollectionName))
	if err != nil {
		return true, 0, fmt.Errorf("failed to describe summary collection: %w", err)
	}
	if coll.Schema == nil {
		return true, 0, nil
	}
	for _, f := range coll.Schema.Fields {
		if f == nil || f.Name != sfEmbedding {
			continue
		}
		if dimStr, ok := f.TypeParams["dim"]; ok {
			var dim int
			_, scanErr := fmt.Sscanf(dimStr, "%d", &dim)
			if scanErr == nil {
				return true, dim, nil
			}
		}
	}
	return true, 0, nil
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func truncate(s string, max int) string {
	if max <= 0 {
		return s
	}
	if len(s) <= max {
		return s
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max])
}

func escapeMilvusString(s string) string {
	return strings.ReplaceAll(s, `"`, `\"`)
}

// buildSummaryFilterExpr 根据 SummaryFilter 拼出 Milvus 的布尔表达式。
// 多个子句之间用 AND 连接；同一字段的多个值内部用 OR。
func buildSummaryFilterExpr(f types.SummaryFilter) string {
	var clauses []string

	if len(f.KnowledgeBaseIDs) > 0 {
		clauses = append(clauses, inClause(sfKnowledgeBaseID, f.KnowledgeBaseIDs))
	}
	if len(f.KnowledgeIDs) > 0 {
		clauses = append(clauses, inClause(sfKnowledgeID, f.KnowledgeIDs))
	}
	if len(f.TagIDs) > 0 {
		// tag_id 是 Array<VarChar>，使用 ARRAY_CONTAINS_ANY 命中任意祖先/自身。
		quoted := make([]string, 0, len(f.TagIDs))
		for _, t := range f.TagIDs {
			quoted = append(quoted, fmt.Sprintf(`"%s"`, escapeMilvusString(t)))
		}
		clauses = append(clauses, fmt.Sprintf("ARRAY_CONTAINS_ANY(%s, [%s])", sfTagID, strings.Join(quoted, ",")))
	}
	if len(f.FileNameKeywords) > 0 {
		var likes []string
		for _, kw := range f.FileNameKeywords {
			kw = strings.TrimSpace(kw)
			if kw == "" {
				continue
			}
			likes = append(likes, fmt.Sprintf(`%s like "%%%s%%"`, sfFileName, escapeMilvusString(kw)))
		}
		if len(likes) > 0 {
			clauses = append(clauses, "("+strings.Join(likes, " or ")+")")
		}
	}
	return strings.Join(clauses, " and ")
}

func inClause(field string, values []string) string {
	quoted := make([]string, 0, len(values))
	for _, v := range values {
		quoted = append(quoted, fmt.Sprintf(`"%s"`, escapeMilvusString(v)))
	}
	return fmt.Sprintf("%s in [%s]", field, strings.Join(quoted, ","))
}

// metadataVector returns the MetadataVector if non-empty, otherwise falls back
// to a zero vector matching the content embedding dimension (Milvus requires all
// FloatVector columns to have the same length within one upsert batch).
func metadataVector(item *types.SummaryItem) []float32 {
	if len(item.MetadataVector) > 0 {
		return item.MetadataVector
	}
	// fallback: zero-filled vector with same dimension as content embedding
	return make([]float32, len(item.Vector))
}

// convertSummaryResultSet maps the Milvus search response into SummaryHit slice.
// 仅装填 Milvus schema 中存在的字段；title/file_type/created_at 等富字段由调用方按
// KnowledgeID 回 PG 查询补全。
func convertSummaryResultSet(resultSet []client.ResultSet) []*types.SummaryHit {
	if len(resultSet) == 0 {
		return nil
	}
	set := resultSet[0]
	n := set.Len()
	if n == 0 {
		return nil
	}
	hits := make([]*types.SummaryHit, n)
	for i := 0; i < n; i++ {
		hits[i] = &types.SummaryHit{}
	}
	for i, score := range set.Scores {
		if i < n {
			hits[i].Score = float64(score)
		}
	}
	readString := func(field string, setter func(i int, v string)) {
		col := set.GetColumn(field)
		if col == nil {
			return
		}
		for i := 0; i < col.Len() && i < n; i++ {
			if v, err := col.GetAsString(i); err == nil {
				setter(i, v)
			}
		}
	}
	readString(sfID, func(i int, v string) { hits[i].ID = v })
	readString(sfKnowledgeID, func(i int, v string) { hits[i].KnowledgeID = v })
	readString(sfKnowledgeBaseID, func(i int, v string) { hits[i].KnowledgeBaseID = v })
	readString(sfFileName, func(i int, v string) { hits[i].FileName = v })
	readString(sfContent, func(i int, v string) { hits[i].Content = v })
	readString(sfMetadata, func(i int, v string) { hits[i].Metadata = v })

	// tag_id Array<VarChar>
	if col := set.GetColumn(sfTagID); col != nil {
		if arrCol, ok := col.(*column.ColumnVarCharArray); ok {
			for i := 0; i < arrCol.Len() && i < n; i++ {
				if v, err := arrCol.Value(i); err == nil {
					hits[i].TagIDs = v
				}
			}
		}
	}

	return hits
}
