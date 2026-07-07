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

	// 1. Fix the newTagPath block (uses actual tab indentation)
	old := "\t\t\t\tif newTagUUID != nil {\n\t\t\t\t\tpathVal := \"\"\n\t\t\t\t\tif newTagPath != nil {\n\t\t\t\t\t\tpathVal = *newTagPath\n\t\t\t\t\t}\n\t\t\t\t\tfor _, id := range affectedIDs {\n\t\t\t\t\t\ttagUpdates[id] = types.ChunkTagUpdate{TagID: *newTagUUID, TagPath: pathVal}\n\t\t\t\t\t}\n\t\t\t\t}"
	new := "\t\t\t\tif newTagUUID != nil {\n\t\t\t\t\tfor _, id := range affectedIDs {\n\t\t\t\t\t\ttagUpdates[id] = types.ChunkTagUpdate{TagID: *newTagUUID}\n\t\t\t\t\t}\n\t\t\t\t}"
	if strings.Contains(src, old) {
		src = strings.Replace(src, old, new, 1)
		count++
	} else {
		// Try alternate: indentation with 3 tabs for outermost
		old = "\t\t\tif newTagUUID != nil {\n\t\t\t\tpathVal := \"\"\n\t\t\t\tif newTagPath != nil {\n\t\t\t\t\tpathVal = *newTagPath\n\t\t\t\t}\n\t\t\t\tfor _, id := range affectedIDs {\n\t\t\t\t\ttagUpdates[id] = types.ChunkTagUpdate{TagID: *newTagUUID, TagPath: pathVal}\n\t\t\t\t}\n\t\t\t}"
		new = "\t\t\tif newTagUUID != nil {\n\t\t\t\tfor _, id := range affectedIDs {\n\t\t\t\t\ttagUpdates[id] = types.ChunkTagUpdate{TagID: *newTagUUID}\n\t\t\t\t}\n\t\t\t}"
		if strings.Contains(src, old) {
			src = strings.Replace(src, old, new, 1)
			count++
		}
	}

	// 2. Fix the tag validation block in UpdateFAQEntryTagBatch (lines 5602-5627)
	old = "\t// Build tag seq_id set for validation\n\ttagSeqIDSet := make(map[int64]bool)\n\tfor _, tagSeqID := range updates {\n\t\tif tagSeqID != nil && *tagSeqID > 0 {\n\t\t\ttagSeqIDSet[*tagSeqID] = true\n\t\t}\n\t}\n\n\t// Validate all tags in batch by seq_id\n\ttagMap := make(map[int64]*types.KnowledgeTag)\n\tif len(tagSeqIDSet) > 0 {\n\t\ttagSeqIDs := make([]int64, 0, len(tagSeqIDSet))\n\t\tfor tagSeqID := range tagSeqIDSet {\n\t\t\ttagSeqIDs = append(tagSeqIDs, tagSeqID)\n\t\t}\n\t\ttags, err := s.tagRepo.GetByIDs(ctx, tagSeqIDs)\n\t\tif err != nil {\n\t\t\treturn err\n\t\t}\n\t\tfor _, tag := range tags {\n\t\t\tif tag.KnowledgeBaseID != kb.ID {\n\t\t\t\treturn werrors.NewBadRequestError(fmt.Sprintf(\"标签 %d 不属于当前知识库\", tag.SeqID))\n\t\t\t}\n\t\t\ttagMap[tag.SeqID] = tag\n\t\t}\n\t}"
	new = "\t// Build tag UUID set for validation\n\ttagIDSet := make(map[string]bool)\n\tfor _, tagUUID := range updates {\n\t\tif tagUUID != nil && *tagUUID != \"\" {\n\t\t\ttagIDSet[*tagUUID] = true\n\t\t}\n\t}\n\n\t// Validate all tags in batch\n\ttagMap := make(map[string]*types.KnowledgeTag)\n\tif len(tagIDSet) > 0 {\n\t\ttagIDs := make([]string, 0, len(tagIDSet))\n\t\tfor tagID := range tagIDSet {\n\t\t\ttagIDs = append(tagIDs, tagID)\n\t\t}\n\t\ttags, err := s.tagRepo.GetByIDs(ctx, tagIDs)\n\t\tif err != nil {\n\t\t\treturn err\n\t\t}\n\t\tfor _, tag := range tags {\n\t\t\tif tag.KnowledgeBaseID != kb.ID {\n\t\t\t\treturn werrors.NewBadRequestError(fmt.Sprintf(\"标签 %s 不属于当前知识库\", tag.ID))\n\t\t\t}\n\t\t\ttagMap[tag.ID] = tag\n\t\t}\n\t}"
	if strings.Contains(src, old) {
		src = strings.Replace(src, old, new, 1)
		count++
	}

	// 3. Fix tagMap[*tagUUID] which was already map[int64] but now should be map[string]
	// Already handled by step 6 of fix_kg22 (replaced tagMap[*tagSeqID] → tagMap[*tagUUID])
	// But the tagMap type mismatch from step 5 didn't work, let me check...
	// If tagMap is now map[string]*types.KnowledgeTag, the existing code at 5643 should work

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
