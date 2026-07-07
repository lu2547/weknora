//go:build ignore

package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {
	path := "internal/application/service/knowledge.go"
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read: %v\n", err)
		os.Exit(1)
	}
	src := string(data)
	count := 0

	// 1. Fix updateChunkVector: sourceKB.EmbeddingModelID → use default embedding model
	old := `func (s *knowledgeService) updateChunkVector(ctx context.Context, kbID string, chunks []*types.Chunk) error {
	// Get embedding model from knowledge base
	sourceKB, err := s.kbService.GetKnowledgeBaseByID(ctx, kbID)
	if err != nil {
		return err
	}
	embeddingModel, err := s.modelService.GetEmbeddingModel(ctx, sourceKB.EmbeddingModelID)`
	new := `func (s *knowledgeService) updateChunkVector(ctx context.Context, kbID string, chunks []*types.Chunk) error {
	// Get default embedding model
	embeddingModelID, err := s.getDefaultEmbeddingModelID(ctx)
	if err != nil {
		return err
	}
	embeddingModel, err := s.modelService.GetEmbeddingModel(ctx, embeddingModelID)`
	if strings.Contains(src, old) {
		src = strings.Replace(src, old, new, 1)
		count++
	}

	// 2. Fix findFAQKnowledge call - remove effectiveTenantID
	old = "s.findFAQKnowledge(ctx, effectiveTenantID, kb.ID)"
	new = "s.findFAQKnowledge(ctx, kb.ID)"
	if strings.Contains(src, old) {
		src = strings.Replace(src, old, new, 1)
		count++
	}

	// 3. Fix ListFAQEntries signature: tagSeqID int64 → tagID string
	old = `func (s *knowledgeService) ListFAQEntries(ctx context.Context,
	kbID string, page *types.Pagination, tagSeqID int64, keyword string, searchField string, sortOrder string,
) (*types.PageResult, error) {`
	new = `func (s *knowledgeService) ListFAQEntries(ctx context.Context,
	kbID string, page *types.Pagination, tagID string, keyword string, searchField string, sortOrder string,
) (*types.PageResult, error) {`
	if strings.Contains(src, old) {
		src = strings.Replace(src, old, new, 1)
		count++
	}

	// 4. Fix tag lookup: was converting tagSeqID→tagID, now tagID is already a string
	old = `	// Convert tagSeqID to tagID (UUID)
	var tagID string
	if tagSeqID > 0 {
		tag, err := s.tagRepo.GetByID(ctx, tagSeqID)
		if err != nil {
			return nil, werrors.NewNotFoundError("标签不存在")
		}
		tagID = tag.ID
	}`
	new = `	// tagID is already a UUID string, no conversion needed`
	if strings.Contains(src, old) {
		src = strings.Replace(src, old, new, 1)
		count++
	}

	// 5. Fix ListPagedChunksByKnowledgeID call - remove effectiveTenantID
	old = "s.chunkRepo.ListPagedChunksByKnowledgeID(\n\t\tctx, effectiveTenantID, faqKnowledge.ID, page, chunkType, tagID, keyword, searchField, sortOrder, types.KnowledgeTypeFAQ,"
	new = "s.chunkRepo.ListPagedChunksByKnowledgeID(\n\t\tctx, faqKnowledge.ID, page, chunkType, tagID, keyword, searchField, sortOrder, types.KnowledgeTypeFAQ,"
	if strings.Contains(src, old) {
		src = strings.Replace(src, old, new, 1)
		count++
	}

	// 6. Fix GetByIDs call - remove effectiveTenantID
	old = "s.tagRepo.GetByIDs(ctx, effectiveTenantID, tagIDs)"
	new = "s.tagRepo.GetByIDs(ctx, tagIDs)"
	if strings.Contains(src, old) {
		src = strings.ReplaceAll(src, old, new)
		count++
	}

	// 7. Fix tag.SeqID → use tag.ID as key in tagSeqIDMap (remove SeqID usage)
	// Replace the tagSeqIDMap with a simple tagNameMap approach
	old = `	// Build tag ID to name and seq_id mapping for all unique tag IDs (batch query)
	tagNameMap := make(map[string]string)
	tagSeqIDMap := make(map[string]int64)
	tagIDs := make([]string, 0)
	tagIDSet := make(map[string]struct{})
	for _, chunk := range chunks {
		if chunk.TagID != "" {
			if _, exists := tagIDSet[chunk.TagID]; !exists {
				tagIDSet[chunk.TagID] = struct{}{}
				tagIDs = append(tagIDs, chunk.TagID)
			}
		}
	}
	if len(tagIDs) > 0 {
		tags, err := s.tagRepo.GetByIDs(ctx, tagIDs)
		if err == nil {
			for _, tag := range tags {
				tagNameMap[tag.ID] = tag.Name
				tagSeqIDMap[tag.ID] = tag.SeqID
			}
		}
	}`
	new = `	// Build tag ID to name mapping for all unique tag IDs (batch query)
	tagNameMap := make(map[string]string)
	tagSeqIDMap := make(map[string]int64) // kept for API compat, always 0
	tagIDs := make([]string, 0)
	tagIDSet := make(map[string]struct{})
	for _, chunk := range chunks {
		if chunk.TagID != "" {
			if _, exists := tagIDSet[chunk.TagID]; !exists {
				tagIDSet[chunk.TagID] = struct{}{}
				tagIDs = append(tagIDs, chunk.TagID)
			}
		}
	}
	if len(tagIDs) > 0 {
		tags, err := s.tagRepo.GetByIDs(ctx, tagIDs)
		if err == nil {
			for _, tag := range tags {
				tagNameMap[tag.ID] = tag.Name
				tagSeqIDMap[tag.ID] = 0 // SeqID removed, placeholder
			}
		}
	}`
	if strings.Contains(src, old) {
		src = strings.Replace(src, old, new, 1)
		count++
	}

	// 8. Fix unused tenantID in saveFAQImportResultToDatabase
	old = "func (s *knowledgeService) saveFAQImportResultToDatabase(ctx context.Context,\n\tpayload *types.FAQImportPayload, progress *types.FAQImportProgress, originalTotalEntries int,\n) error {\n\t// 获取FAQ知识库实例\n\ttenantID := ctx.Value(types.TenantIDContextKey).(uint64)\n\tknowledge, err := s.repo.GetKnowledgeByID(ctx, payload.KnowledgeID)"
	new = "func (s *knowledgeService) saveFAQImportResultToDatabase(ctx context.Context,\n\tpayload *types.FAQImportPayload, progress *types.FAQImportProgress, originalTotalEntries int,\n) error {\n\t// 获取FAQ知识库实例\n\tknowledge, err := s.repo.GetKnowledgeByID(ctx, payload.KnowledgeID)"
	if strings.Contains(src, old) {
		src = strings.Replace(src, old, new, 1)
		count++
	}

	// 9. Fix calculateAppendOperations signature - remove tenantID param
	old = "func (s *knowledgeService) calculateAppendOperations(ctx context.Context,\n\ttenantID uint64, kbID string, entries []types.FAQEntryPayload,"
	new = "func (s *knowledgeService) calculateAppendOperations(ctx context.Context,\n\tkbID string, entries []types.FAQEntryPayload,"
	if strings.Contains(src, old) {
		src = strings.Replace(src, old, new, 1)
		count++
	}

	// 10. Fix calculateAppendOperations call - remove tenantID arg
	old = "s.calculateAppendOperations(ctx, tenantID, kb.ID, payload.Entries)"
	new = "s.calculateAppendOperations(ctx, kb.ID, payload.Entries)"
	if strings.Contains(src, old) {
		src = strings.Replace(src, old, new, 1)
		count++
	}

	// 11. Fix tag.SeqID at line ~4586 in FAQ success entry building
	old = "if tag, err := s.tagRepo.GetByID(ctx, chunk.TagID); err == nil && tag != nil {\n\t\t\t\t\ttagID = tag.SeqID\n\t\t\t\t\ttagName = tag.Name"
	new = "if tag, err := s.tagRepo.GetByID(ctx, chunk.TagID); err == nil && tag != nil {\n\t\t\t\t\t_ = tag // SeqID removed\n\t\t\t\t\ttagName = tag.Name"
	if strings.Contains(src, old) {
		src = strings.Replace(src, old, new, 1)
		count++
	}

	// 12. Fix var tagID int64 → string for the FAQ success entry
	old = "\t\t\tvar tagID int64\n\t\t\ttagName := \"\"\n\t\t\tif chunk.TagID != \"\" {\n\t\t\t\tif tag, err := s.tagRepo.GetByID(ctx, chunk.TagID); err == nil && tag != nil {\n\t\t\t\t\t_ = tag // SeqID removed\n\t\t\t\t\ttagName = tag.Name"
	new = "\t\t\tvar tagIDVal string\n\t\t\ttagName := \"\"\n\t\t\tif chunk.TagID != \"\" {\n\t\t\t\ttagIDVal = chunk.TagID\n\t\t\t\tif tag, err := s.tagRepo.GetByID(ctx, chunk.TagID); err == nil && tag != nil {\n\t\t\t\t\ttagName = tag.Name"
	if strings.Contains(src, old) {
		src = strings.Replace(src, old, new, 1)
		count++
	}

	if count == 0 {
		fmt.Println("No replacements made")
		os.Exit(1)
	}

	err = os.WriteFile(path, []byte(src), 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "write: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Done: %d replacements\n", count)
}
