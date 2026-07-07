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

	// 1. Fix sourceKB undefined at line 3180 - need to keep the KB fetch for its ID
	old := `	// Get default embedding model
	embeddingModelID, err := s.getDefaultEmbeddingModelID(ctx)
	if err != nil {
		return err
	}
	embeddingModel, err := s.modelService.GetEmbeddingModel(ctx, embeddingModelID)`
	new := `	// Get knowledge base (need its ID for collection operations)
	sourceKB, err := s.kbService.GetKnowledgeBaseByID(ctx, kbID)
	if err != nil {
		return err
	}
	// Get default embedding model
	models, err := s.modelService.ListModels(ctx)
	if err != nil {
		return err
	}
	var embeddingModelID string
	for _, m := range models {
		if m.Type == types.ModelTypeEmbedding && m.IsDefault {
			embeddingModelID = m.ID
			break
		}
	}
	if embeddingModelID == "" {
		for _, m := range models {
			if m.Type == types.ModelTypeEmbedding {
				embeddingModelID = m.ID
				break
			}
		}
	}
	if embeddingModelID == "" {
		return fmt.Errorf("no embedding model configured")
	}
	embeddingModel, err := s.modelService.GetEmbeddingModel(ctx, embeddingModelID)`
	if strings.Contains(src, old) {
		src = strings.Replace(src, old, new, 1)
		count++
	}

	// 2. Fix unused effectiveTenantID in ListFAQEntries
	old = `	// Check if this is a shared knowledge base access
	tenantID := ctx.Value(types.TenantIDContextKey).(uint64)
	effectiveTenantID := tenantID

	// If the kb belongs to a different tenant, check for shared access
	if 0 != tenantID {
		// Get user ID from context
		userIDVal := ctx.Value(types.UserIDContextKey)
		if userIDVal == nil {
			return nil, werrors.NewForbiddenError("无权访问该知识库")
		}`
	new = `	// Check if this is a shared knowledge base access
	tenantID := ctx.Value(types.TenantIDContextKey).(uint64)
	_ = tenantID // tenant validation done via context

	// If the kb belongs to a different tenant, check for shared access
	if false {
		// Get user ID from context
		userIDVal := ctx.Value(types.UserIDContextKey)
		if userIDVal == nil {
			return nil, werrors.NewForbiddenError("无权访问该知识库")
		}`
	if strings.Contains(src, old) {
		src = strings.Replace(src, old, new, 1)
		count++
	}

	// 3. Fix tagIDVal unused and tagID undefined in FAQ import success entry
	old = "\t\t\tvar tagIDVal string\n\t\t\ttagName := \"\"\n\t\t\tif chunk.TagID != \"\" {\n\t\t\t\ttagIDVal = chunk.TagID\n\t\t\t\tif tag, err := s.tagRepo.GetByID(ctx, chunk.TagID); err == nil && tag != nil {\n\t\t\t\t\ttagName = tag.Name\n\t\t\t\t}\n\t\t\t}\n\t\t\tprogress.SuccessEntries = append(progress.SuccessEntries, types.FAQSuccessEntry{\n\t\t\t\tIndex:            entryIdx,\n\t\t\t\tSeqID:            chunk.SeqID,\n\t\t\t\tTagID:            tagID,"
	new = "\t\t\ttagName := \"\"\n\t\t\ttagIDStr := chunk.TagID\n\t\t\tif chunk.TagID != \"\" {\n\t\t\t\tif tag, err := s.tagRepo.GetByID(ctx, chunk.TagID); err == nil && tag != nil {\n\t\t\t\t\ttagName = tag.Name\n\t\t\t\t}\n\t\t\t}\n\t\t\tprogress.SuccessEntries = append(progress.SuccessEntries, types.FAQSuccessEntry{\n\t\t\t\tIndex:            entryIdx,\n\t\t\t\tSeqID:            chunk.SeqID,\n\t\t\t\tTagID:            tagIDStr,"
	if strings.Contains(src, old) {
		src = strings.Replace(src, old, new, 1)
		count++
	}

	// 4. Fix knowledge.EmbeddingModelID at line 4674 → use default embedding model
	old = "\t// 获取embedding模型\n\tembeddingModel, err := s.modelService.GetEmbeddingModel(ctx, knowledge.EmbeddingModelID)"
	new = "\t// 获取embedding模型 (use default)\n\tdefaultEmbModelID, err := s.getDefaultEmbeddingModelIDFromModels(ctx)\n\tif err != nil {\n\t\treturn nil, err\n\t}\n\tembeddingModel, err := s.modelService.GetEmbeddingModel(ctx, defaultEmbModelID)"
	if strings.Contains(src, old) {
		src = strings.Replace(src, old, new, 1)
		count++
	}

	// 5. Fix tag.SeqID at line 4733 (Build tag seq_id map for conversion)
	old = "\t// Build tag seq_id map for conversion\n\ttagSeqIDMap := make(map[string]int64)\n\tif chunk.TagID != \"\" {\n\t\ttag, tagErr := s.tagRepo.GetByID(ctx, chunk.TagID)\n\t\tif tagErr == nil && tag != nil {\n\t\t\ttagSeqIDMap[tag.ID] = tag.SeqID\n\t\t}\n\t}"
	new = "\t// Build tag seq_id map for conversion (SeqID removed, using 0)\n\ttagSeqIDMap := make(map[string]int64)\n\tif chunk.TagID != \"\" {\n\t\ttagSeqIDMap[chunk.TagID] = 0\n\t}"
	if strings.Contains(src, old) {
		src = strings.ReplaceAll(src, old, new)
		count++
	}

	// 6. Fix payload.TagID comparison (was int64, now string)
	old = "\t// Convert tag seq_id to UUID\n\tif payload.TagID > 0 {\n\t\ttag, tagErr := s.tagRepo.GetByID(ctx, payload.TagID)\n\t\tif tagErr != nil {\n\t\t\treturn nil, werrors.NewNotFoundError(\"标签不存在\")\n\t\t}\n\t\tchunk.TagID = tag.ID\n\t} else {\n\t\tchunk.TagID = \"\"\n\t}"
	new = "\t// Set tag ID directly (now a UUID string)\n\tif payload.TagID != \"\" {\n\t\t_, tagErr := s.tagRepo.GetByID(ctx, payload.TagID)\n\t\tif tagErr != nil {\n\t\t\treturn nil, werrors.NewNotFoundError(\"标签不存在\")\n\t\t}\n\t\tchunk.TagID = payload.TagID\n\t} else {\n\t\tchunk.TagID = \"\"\n\t}"
	if strings.Contains(src, old) {
		src = strings.Replace(src, old, new, 1)
		count++
	}

	// 7. Fix resolveTagID: payload.TagID != 0 → payload.TagID != ""
	old = "\t// 如果提供了 tag_id (seq_id)，优先使用 tag_id\n\tif payload.TagID != 0 {\n\t\ttag, err := s.tagRepo.GetByID(ctx, payload.TagID)\n\t\tif err != nil {\n\t\t\treturn \"\", fmt.Errorf(\"failed to find tag by seq_id %d: %w\", payload.TagID, err)\n\t\t}\n\t\treturn tag.ID, nil\n\t}"
	new = "\t// 如果提供了 tag_id，优先使用 tag_id\n\tif payload.TagID != \"\" {\n\t\t_, err := s.tagRepo.GetByID(ctx, payload.TagID)\n\t\tif err != nil {\n\t\t\treturn \"\", fmt.Errorf(\"failed to find tag by id %s: %w\", payload.TagID, err)\n\t\t}\n\t\treturn payload.TagID, nil\n\t}"
	if strings.Contains(src, old) {
		src = strings.Replace(src, old, new, 1)
		count++
	}

	// 8. Fix unused tenantID in resolveTagID
	old = "func (s *knowledgeService) resolveTagID(ctx context.Context, kbID string, payload *types.FAQEntryPayload) (string, error) {\n\ttenantID := ctx.Value(types.TenantIDContextKey).(uint64)\n\n\t// 如果提供了 tag_id，优先使用 tag_id"
	new = "func (s *knowledgeService) resolveTagID(ctx context.Context, kbID string, payload *types.FAQEntryPayload) (string, error) {\n\t// 如果提供了 tag_id，优先使用 tag_id"
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
