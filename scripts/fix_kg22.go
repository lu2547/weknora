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

	// 1. Fix remaining newTagPath/TagPath in ByTag block (lines 5282-5290)
	old := `			if newTagUUID != nil {
				pathVal := ""
				if newTagPath != nil {
					pathVal = *newTagPath
				}
				for _, id := range affectedIDs {
					tagUpdates[id] = types.ChunkTagUpdate{TagID: *newTagUUID, TagPath: pathVal}
				}
			}`
	new := `			if newTagUUID != nil {
				for _, id := range affectedIDs {
					tagUpdates[id] = types.ChunkTagUpdate{TagID: *newTagUUID}
				}
			}`
	if strings.Contains(src, old) {
		src = strings.Replace(src, old, new, 1)
		count++
	}

	// 2. Fix unused tenantID in UpdateKnowledgeTagBatch (line 5442)
	old = "\ttenantIDVal := ctx.Value(types.TenantIDContextKey)\n\tif tenantIDVal == nil {\n\t\treturn werrors.NewUnauthorizedError(\"tenant ID not found in context\")\n\t}\n\ttenantID, ok := tenantIDVal.(uint64)\n\tif !ok {\n\t\treturn werrors.NewUnauthorizedError(\"invalid tenant ID in context\")\n\t}"
	new = "\ttenantIDVal := ctx.Value(types.TenantIDContextKey)\n\tif tenantIDVal == nil {\n\t\treturn werrors.NewUnauthorizedError(\"tenant ID not found in context\")\n\t}\n\t_, ok := tenantIDVal.(uint64)\n\tif !ok {\n\t\treturn werrors.NewUnauthorizedError(\"invalid tenant ID in context\")\n\t}"
	if strings.Contains(src, old) {
		src = strings.Replace(src, old, new, 1)
		count++
	}

	// 3. Fix ChunkTagUpdate with TagPath in UpdateFAQEntryTag (line 5571)
	old = "return retrieveEngine.BatchUpdateChunkTagID(ctx, kb.ID, map[string]types.ChunkTagUpdate{chunk.ID: {TagID: resolvedTagID, TagPath: resolvedTagPath}})"
	new = "return retrieveEngine.BatchUpdateChunkTagID(ctx, kb.ID, map[string]types.ChunkTagUpdate{chunk.ID: {TagID: resolvedTagID}})"
	if strings.Contains(src, old) {
		src = strings.Replace(src, old, new, 1)
		count++
	}

	// 4. Fix UpdateFAQEntryTagBatch - the whole tag lookup logic uses int64 seq_ids
	// Change the function signature and body to use string tag IDs
	old = `// UpdateFAQEntryTagBatch updates tags for FAQ entries in batch.
// Key: entry seq_id, Value: tag seq_id (nil to remove tag)
func (s *knowledgeService) UpdateFAQEntryTagBatch(ctx context.Context, kbID string, updates map[int64]*int64) error {`
	new = `// UpdateFAQEntryTagBatch updates tags for FAQ entries in batch.
// Key: entry seq_id, Value: tag UUID (nil to remove tag)
func (s *knowledgeService) UpdateFAQEntryTagBatch(ctx context.Context, kbID string, updates map[int64]*string) error {`
	if strings.Contains(src, old) {
		src = strings.Replace(src, old, new, 1)
		count++
	}

	// 5. Fix tag validation block in UpdateFAQEntryTagBatch
	old = `	// Collect unique tag seq_ids for batch lookup
	tagSeqIDSet := make(map[int64]bool)
	for _, tagSeqID := range updates {
		if tagSeqID != nil && *tagSeqID > 0 {
			tagSeqIDSet[*tagSeqID] = true
		}
	}

	// Validate all tags in batch by seq_id
	tagMap := make(map[int64]*types.KnowledgeTag)
	if len(tagSeqIDSet) > 0 {
		tagSeqIDs := make([]int64, 0, len(tagSeqIDSet))
		for tagSeqID := range tagSeqIDSet {
			tagSeqIDs = append(tagSeqIDs, tagSeqID)
		}
		tags, err := s.tagRepo.GetByIDs(ctx, tagSeqIDs)
		if err != nil {
			return err
		}
		for _, tag := range tags {
			if tag.KnowledgeBaseID != kb.ID {
				return werrors.NewBadRequestError(fmt.Sprintf("标签 %d 不属于当前知识库", tag.SeqID))
			}
			tagMap[tag.SeqID] = tag
		}
	}`
	new = `	// Collect unique tag UUIDs for batch lookup
	tagIDSet := make(map[string]bool)
	for _, tagUUID := range updates {
		if tagUUID != nil && *tagUUID != "" {
			tagIDSet[*tagUUID] = true
		}
	}

	// Validate all tags in batch
	tagMap := make(map[string]*types.KnowledgeTag)
	if len(tagIDSet) > 0 {
		tagIDs := make([]string, 0, len(tagIDSet))
		for tagID := range tagIDSet {
			tagIDs = append(tagIDs, tagID)
		}
		tags, err := s.tagRepo.GetByIDs(ctx, tagIDs)
		if err != nil {
			return err
		}
		for _, tag := range tags {
			if tag.KnowledgeBaseID != kb.ID {
				return werrors.NewBadRequestError(fmt.Sprintf("标签 %s 不属于当前知识库", tag.ID))
			}
			tagMap[tag.ID] = tag
		}
	}`
	if strings.Contains(src, old) {
		src = strings.Replace(src, old, new, 1)
		count++
	}

	// 6. Fix update loop in UpdateFAQEntryTagBatch
	old = `	// Update chunks
	chunksToUpdate := make([]*types.Chunk, 0)
	tagUpdates := make(map[string]types.ChunkTagUpdate)
	for entrySeqID, tagSeqID := range updates {
		chunk, exists := chunkBySeqID[entrySeqID]
		if !exists {
			continue
		}
		if chunk.KnowledgeBaseID != kb.ID || chunk.ChunkType != types.ChunkTypeFAQ {
			continue
		}

		var resolvedTagID, resolvedTagPath string
		if tagSeqID != nil && *tagSeqID > 0 {
			tag, ok := tagMap[*tagSeqID]
			if !ok {
				return werrors.NewBadRequestError(fmt.Sprintf("标签 %d 不存在", *tagSeqID))
			}
			resolvedTagID = tag.ID
			resolvedTagPath = tag.Path
		}

		chunk.TagID = resolvedTagID
		chunk.UpdatedAt = time.Now()
		chunksToUpdate = append(chunksToUpdate, chunk)
		tagUpdates[chunk.ID] = types.ChunkTagUpdate{TagID: resolvedTagID, TagPath: resolvedTagPath}
	}`
	new = `	// Update chunks
	chunksToUpdate := make([]*types.Chunk, 0)
	tagUpdates := make(map[string]types.ChunkTagUpdate)
	for entrySeqID, tagUUID := range updates {
		chunk, exists := chunkBySeqID[entrySeqID]
		if !exists {
			continue
		}
		if chunk.KnowledgeBaseID != kb.ID || chunk.ChunkType != types.ChunkTypeFAQ {
			continue
		}

		var resolvedTagID string
		if tagUUID != nil && *tagUUID != "" {
			_, ok := tagMap[*tagUUID]
			if !ok {
				return werrors.NewBadRequestError(fmt.Sprintf("标签 %s 不存在", *tagUUID))
			}
			resolvedTagID = *tagUUID
		}

		chunk.TagID = resolvedTagID
		chunk.UpdatedAt = time.Now()
		chunksToUpdate = append(chunksToUpdate, chunk)
		tagUpdates[chunk.ID] = types.ChunkTagUpdate{TagID: resolvedTagID}
	}`
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
