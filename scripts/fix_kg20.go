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

	// 1. Fix effectiveTenantID at line 3563 - just remove the assignment
	old := `		// Use the source tenant ID for data access
		sourceTenantID, err := s.kbShareService.GetKBSourceTenant(ctx, kbID)
		if err != nil {
			return nil, werrors.NewForbiddenError("无权访问该知识库")
		}
		effectiveTenantID = sourceTenantID
	}`
	new := `		// Use the source tenant ID for data access
		_, err = s.kbShareService.GetKBSourceTenant(ctx, kbID)
		if err != nil {
			return nil, werrors.NewForbiddenError("无权访问该知识库")
		}
	}`
	if strings.Contains(src, old) {
		src = strings.Replace(src, old, new, 1)
		count++
	}

	// 2. Fix knowledge.EmbeddingModelID at line 4949 → faqKnowledge + default model
	old = "\tembeddingModel, err := s.modelService.GetEmbeddingModel(ctx, knowledge.EmbeddingModelID)\n\tif err != nil {\n\t\treturn nil, err\n\t}\n\n\t// 增量索引优化：只对变化的内容进行索引操作"
	new = "\tdefEmbModelID, err := s.getDefaultEmbeddingModelIDFromModels(ctx)\n\tif err != nil {\n\t\treturn nil, err\n\t}\n\tembeddingModel, err := s.modelService.GetEmbeddingModel(ctx, defEmbModelID)\n\tif err != nil {\n\t\treturn nil, err\n\t}\n\n\t// 增量索引优化：只对变化的内容进行索引操作"
	if strings.Contains(src, old) {
		src = strings.Replace(src, old, new, 1)
		count++
	}

	// 3. Fix tag.SeqID at line 5073
	old = "\tif len(newQuestions) == 0 {\n\t\t// No new questions to add, return current entry\n\t\ttagSeqIDMap := make(map[string]int64)\n\t\tif chunk.TagID != \"\" {\n\t\t\ttag, tagErr := s.tagRepo.GetByID(ctx, chunk.TagID)\n\t\t\tif tagErr == nil && tag != nil {\n\t\t\t\ttagSeqIDMap[tag.ID] = tag.SeqID\n\t\t\t}\n\t\t}\n\t\treturn s.chunkToFAQEntry(chunk, kb, tagSeqIDMap)\n\t}"
	new = "\tif len(newQuestions) == 0 {\n\t\t// No new questions to add, return current entry\n\t\ttagSeqIDMap := make(map[string]int64)\n\t\tif chunk.TagID != \"\" {\n\t\t\ttagSeqIDMap[chunk.TagID] = 0\n\t\t}\n\t\treturn s.chunkToFAQEntry(chunk, kb, tagSeqIDMap)\n\t}"
	if strings.Contains(src, old) {
		src = strings.Replace(src, old, new, 1)
		count++
	}

	// 4. Fix knowledge.EmbeddingModelID at line 5115
	old = "\t// Index new similar questions\n\tfaqKnowledge, err := s.repo.GetKnowledgeByID(ctx, chunk.KnowledgeID)\n\tif err != nil {\n\t\treturn nil, err\n\t}\n\n\tembeddingModel, err := s.modelService.GetEmbeddingModel(ctx, knowledge.EmbeddingModelID)"
	new = "\t// Index new similar questions\n\tfaqKnowledge, err := s.repo.GetKnowledgeByID(ctx, chunk.KnowledgeID)\n\tif err != nil {\n\t\treturn nil, err\n\t}\n\n\tdefEmbID2, err := s.getDefaultEmbeddingModelIDFromModels(ctx)\n\tif err != nil {\n\t\treturn nil, err\n\t}\n\tembeddingModel, err := s.modelService.GetEmbeddingModel(ctx, defEmbID2)"
	if strings.Contains(src, old) {
		src = strings.Replace(src, old, new, 1)
		count++
	}

	// 5. Fix tag.SeqID at line 5143
	old = "tagSeqIDMap := make(map[string]int64)\n\tif chunk.TagID != \"\" {\n\t\ttag, tagErr := s.tagRepo.GetByID(ctx, chunk.TagID)\n\t\tif tagErr == nil && tag != nil {\n\t\t\ttagSeqIDMap[tag.ID] = tag.SeqID\n\t\t}\n\t}\n\n\t// 转换为FAQEntry返回\n\tentry, err := s.chunkToFAQEntry(chunk, kb, tagSeqIDMap)"
	// This pattern might appear in multiple places, replace all
	new = "tagSeqIDMap := make(map[string]int64)\n\tif chunk.TagID != \"\" {\n\t\ttagSeqIDMap[chunk.TagID] = 0\n\t}\n\n\t// 转换为FAQEntry返回\n\tentry, err := s.chunkToFAQEntry(chunk, kb, tagSeqIDMap)"
	if strings.Contains(src, old) {
		src = strings.ReplaceAll(src, old, new)
		count++
	}

	// 6. Fix unused tenantID at line 5170
	old = "func (s *knowledgeService) UpdateFAQEntryStatus(ctx context.Context,\n\tkbID string, entryID string, isEnabled bool,\n) error {\n\tkb, err := s.validateFAQKnowledgeBase(ctx, kbID)\n\tif err != nil {\n\t\treturn err\n\t}\n\ttenantID := ctx.Value(types.TenantIDContextKey).(uint64)\n\tchunk, err := s.chunkRepo.GetChunkByID(ctx, entryID)"
	new = "func (s *knowledgeService) UpdateFAQEntryStatus(ctx context.Context,\n\tkbID string, entryID string, isEnabled bool,\n) error {\n\tkb, err := s.validateFAQKnowledgeBase(ctx, kbID)\n\tif err != nil {\n\t\treturn err\n\t}\n\tchunk, err := s.chunkRepo.GetChunkByID(ctx, entryID)"
	if strings.Contains(src, old) {
		src = strings.Replace(src, old, new, 1)
		count++
	}

	// 7. Fix ByTag loop: tagSeqID is now string (map key is string)
	old = "\t// Handle ByTag updates first (by tag seq_id)\n\tif len(req.ByTag) > 0 {\n\t\tfor tagSeqID, update := range req.ByTag {\n\t\t\t// Convert tag seq_id to UUID\n\t\t\ttag, err := s.tagRepo.GetByID(ctx, tagSeqID)\n\t\t\tif err != nil {\n\t\t\t\treturn werrors.NewNotFoundError(fmt.Sprintf(\"标签 %d 不存在\", tagSeqID))\n\t\t\t}"
	new = "\t// Handle ByTag updates first (by tag UUID)\n\tif len(req.ByTag) > 0 {\n\t\tfor tagUUID, update := range req.ByTag {\n\t\t\t// Validate tag exists\n\t\t\t_, err := s.tagRepo.GetByID(ctx, tagUUID)\n\t\t\tif err != nil {\n\t\t\t\treturn werrors.NewNotFoundError(fmt.Sprintf(\"标签 %s 不存在\", tagUUID))\n\t\t\t}"
	if strings.Contains(src, old) {
		src = strings.Replace(src, old, new, 1)
		count++
	}

	// 8. Fix update.TagID (now *string) and newTag.Path
	old = "\t\t\t// Convert new tag seq_id to UUID if provided\n\t\t\tvar newTagUUID *string\n\t\t\tvar newTagPath *string\n\t\t\tif update.TagID != nil {\n\t\t\t\tif *update.TagID > 0 {\n\t\t\t\t\tnewTag, err := s.tagRepo.GetByID(ctx, *update.TagID)\n\t\t\t\t\tif err != nil {\n\t\t\t\t\t\treturn werrors.NewNotFoundError(fmt.Sprintf(\"标签 %d 不存在\", *update.TagID))\n\t\t\t\t\t}\n\t\t\t\t\tnewTagUUID = &newTag.ID\n\t\t\t\t\tnewTagPath = &newTag.Path\n\t\t\t\t} else {\n\t\t\t\t\temptyStr := \"\"\n\t\t\t\t\tnewTagUUID = &emptyStr\n\t\t\t\t\tnewTagPath = &emptyStr\n\t\t\t\t}\n\t\t\t}"
	new = "\t\t\t// Set new tag UUID if provided\n\t\t\tvar newTagUUID *string\n\t\t\tif update.TagID != nil {\n\t\t\t\tif *update.TagID != \"\" {\n\t\t\t\t\t_, err := s.tagRepo.GetByID(ctx, *update.TagID)\n\t\t\t\t\tif err != nil {\n\t\t\t\t\t\treturn werrors.NewNotFoundError(fmt.Sprintf(\"标签 %s 不存在\", *update.TagID))\n\t\t\t\t\t}\n\t\t\t\t\tnewTagUUID = update.TagID\n\t\t\t\t} else {\n\t\t\t\t\temptyStr := \"\"\n\t\t\t\t\tnewTagUUID = &emptyStr\n\t\t\t\t}\n\t\t\t}"
	if strings.Contains(src, old) {
		src = strings.Replace(src, old, new, 1)
		count++
	}

	// 9. Fix UpdateChunkFieldsByTagID call - remove tenantID, remove newTagPath
	old = "s.chunkRepo.UpdateChunkFieldsByTagID(\n\t\t\t\tctx, tenantID, kb.ID, tag.ID,\n\t\t\t\tupdate.IsEnabled, setFlags, clearFlags, newTagUUID, excludeUUIDs,"
	new = "s.chunkRepo.UpdateChunkFieldsByTagID(\n\t\t\t\tctx, kb.ID, tagUUID,\n\t\t\t\tupdate.IsEnabled, setFlags, clearFlags, newTagUUID, excludeUUIDs,"
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
