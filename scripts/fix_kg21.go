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

	// 1. Fix tag.SeqID at line 5147 (Build response)
	old := "\t// Build response\n\ttagSeqIDMap := make(map[string]int64)\n\tif chunk.TagID != \"\" {\n\t\ttag, tagErr := s.tagRepo.GetByID(ctx, chunk.TagID)\n\t\tif tagErr == nil && tag != nil {\n\t\t\ttagSeqIDMap[tag.ID] = tag.SeqID\n\t\t}\n\t}\n\n\tentry, err := s.chunkToFAQEntry(chunk, kb, tagSeqIDMap)"
	new := "\t// Build response\n\ttagSeqIDMap := make(map[string]int64)\n\tif chunk.TagID != \"\" {\n\t\ttagSeqIDMap[chunk.TagID] = 0\n\t}\n\n\tentry, err := s.chunkToFAQEntry(chunk, kb, tagSeqIDMap)"
	if strings.Contains(src, old) {
		src = strings.Replace(src, old, new, 1)
		count++
	}

	// 2. Fix unused tenantID at line 5219 (UpdateFAQEntryFields)
	old = "\tkb, err := s.validateFAQKnowledgeBase(ctx, kbID)\n\tif err != nil {\n\t\treturn err\n\t}\n\ttenantID := ctx.Value(types.TenantIDContextKey).(uint64)\n\n\tenabledUpdates := make(map[string]bool)"
	new = "\tkb, err := s.validateFAQKnowledgeBase(ctx, kbID)\n\tif err != nil {\n\t\treturn err\n\t}\n\n\tenabledUpdates := make(map[string]bool)"
	if strings.Contains(src, old) {
		src = strings.Replace(src, old, new, 1)
		count++
	}

	// 3. Fix newTagPath usage at lines 5288-5289 (ByTag block) - TagPath removed
	old = "\t\t\tif newTagUUID != nil {\n\t\t\t\tpathVal := \"\"\n\t\t\t\tif newTagPath != nil {\n\t\t\t\t\tpathVal = *newTagPath\n\t\t\t\t}\n\t\t\t\tfor _, id := range affectedIDs {\n\t\t\t\t\ttagUpdates[id] = types.ChunkTagUpdate{TagID: *newTagUUID, TagPath: pathVal}\n\t\t\t\t}\n\t\t\t}"
	new = "\t\t\tif newTagUUID != nil {\n\t\t\t\tfor _, id := range affectedIDs {\n\t\t\t\t\ttagUpdates[id] = types.ChunkTagUpdate{TagID: *newTagUUID}\n\t\t\t\t}\n\t\t\t}"
	if strings.Contains(src, old) {
		src = strings.Replace(src, old, new, 1)
		count++
	}

	// 4. Fix ByID section: *update.TagID > 0 → *update.TagID != "" + newTag.Path
	old = "\t\t\t// Handle TagID (convert seq_id to UUID)\n\t\t\tif update.TagID != nil {\n\t\t\t\tvar newTagID, newTagPath string\n\t\t\t\tif *update.TagID > 0 {\n\t\t\t\t\tnewTag, err := s.tagRepo.GetByID(ctx, *update.TagID)\n\t\t\t\t\tif err != nil {\n\t\t\t\t\t\treturn werrors.NewNotFoundError(fmt.Sprintf(\"标签 %d 不存在\", *update.TagID))\n\t\t\t\t\t}\n\t\t\t\t\tnewTagID = newTag.ID\n\t\t\t\t\tnewTagPath = newTag.Path\n\t\t\t\t}\n\t\t\t\tif chunk.TagID != newTagID {\n\t\t\t\t\tchunk.TagID = newTagID\n\t\t\t\t\ttagUpdates[chunk.ID] = types.ChunkTagUpdate{TagID: newTagID, TagPath: newTagPath}\n\t\t\t\t\tneedUpdate = true\n\t\t\t\t}\n\t\t\t}"
	new = "\t\t\t// Handle TagID (now UUID string)\n\t\t\tif update.TagID != nil {\n\t\t\t\tnewTagID := *update.TagID\n\t\t\t\tif newTagID != \"\" {\n\t\t\t\t\t_, err := s.tagRepo.GetByID(ctx, newTagID)\n\t\t\t\t\tif err != nil {\n\t\t\t\t\t\treturn werrors.NewNotFoundError(fmt.Sprintf(\"标签 %s 不存在\", newTagID))\n\t\t\t\t\t}\n\t\t\t\t}\n\t\t\t\tif chunk.TagID != newTagID {\n\t\t\t\t\tchunk.TagID = newTagID\n\t\t\t\t\ttagUpdates[chunk.ID] = types.ChunkTagUpdate{TagID: newTagID}\n\t\t\t\t\tneedUpdate = true\n\t\t\t\t}\n\t\t\t}"
	if strings.Contains(src, old) {
		src = strings.Replace(src, old, new, 1)
		count++
	}

	// 5. Fix unused tenantID at line 5416 (UpdateKnowledgeTag)
	old = "func (s *knowledgeService) UpdateKnowledgeTag(ctx context.Context, knowledgeID string, tagID *string) error {\n\ttenantID := ctx.Value(types.TenantIDContextKey).(uint64)\n\tknowledge, err := s.repo.GetKnowledgeByID(ctx, knowledgeID)"
	new = "func (s *knowledgeService) UpdateKnowledgeTag(ctx context.Context, knowledgeID string, tagID *string) error {\n\tknowledge, err := s.repo.GetKnowledgeByID(ctx, knowledgeID)"
	if strings.Contains(src, old) {
		src = strings.Replace(src, old, new, 1)
		count++
	}

	// 6. Fix unused tenantID at line 5449 (BatchUpdateKnowledgeTags - search for it)
	// Actually line 5449 might be in a different function, let me be more careful
	// 5538 is in UpdateFAQEntryTag
	old = "func (s *knowledgeService) UpdateFAQEntryTag(ctx context.Context, kbID string, entryID string, tagID *string) error {\n\tkb, err := s.validateFAQKnowledgeBase(ctx, kbID)\n\tif err != nil {\n\t\treturn err\n\t}\n\ttenantID := ctx.Value(types.TenantIDContextKey).(uint64)\n\tchunk, err := s.chunkRepo.GetChunkByID(ctx, entryID)"
	new = "func (s *knowledgeService) UpdateFAQEntryTag(ctx context.Context, kbID string, entryID string, tagID *string) error {\n\tkb, err := s.validateFAQKnowledgeBase(ctx, kbID)\n\tif err != nil {\n\t\treturn err\n\t}\n\tchunk, err := s.chunkRepo.GetChunkByID(ctx, entryID)"
	if strings.Contains(src, old) {
		src = strings.Replace(src, old, new, 1)
		count++
	}

	// 7. Fix tag.Path at line 5557 (UpdateFAQEntryTag)
	old = "\tvar resolvedTagID, resolvedTagPath string\n\tif tagID != nil && *tagID != \"\" {\n\t\ttag, err := s.tagRepo.GetByID(ctx, *tagID)\n\t\tif err != nil {\n\t\t\treturn err\n\t\t}\n\t\tif tag.KnowledgeBaseID != kb.ID {\n\t\t\treturn werrors.NewBadRequestError(\"标签不属于当前知识库\")\n\t\t}\n\t\tresolvedTagID = tag.ID\n\t\tresolvedTagPath = tag.Path\n\t}"
	new = "\tvar resolvedTagID string\n\tif tagID != nil && *tagID != \"\" {\n\t\ttag, err := s.tagRepo.GetByID(ctx, *tagID)\n\t\tif err != nil {\n\t\t\treturn err\n\t\t}\n\t\tif tag.KnowledgeBaseID != kb.ID {\n\t\t\treturn werrors.NewBadRequestError(\"标签不属于当前知识库\")\n\t\t}\n\t\tresolvedTagID = tag.ID\n\t}"
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
