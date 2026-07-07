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

	// 1. Fix unused tenantID in UpdateFAQEntryTagBatch (line 5580)
	old := "\tkb, err := s.validateFAQKnowledgeBase(ctx, kbID)\n\tif err != nil {\n\t\treturn err\n\t}\n\ttenantID := ctx.Value(types.TenantIDContextKey).(uint64)\n\n\t// Get all chunks in batch by seq_id"
	new := "\tkb, err := s.validateFAQKnowledgeBase(ctx, kbID)\n\tif err != nil {\n\t\treturn err\n\t}\n\n\t// Get all chunks in batch by seq_id"
	if strings.Contains(src, old) {
		src = strings.Replace(src, old, new, 1)
		count++
	}

	// 2. Fix unused tenantID in SearchFAQEntries (line 5695)
	old = "\ttenantID := ctx.Value(types.TenantIDContextKey).(uint64)\n\n\t// Convert tag seq_ids to UUIDs\n\tvar firstPriorityTagUUIDs, secondPriorityTagUUIDs []string\n\tfirstPrioritySeqIDSet := make(map[int64]struct{})\n\tsecondPrioritySeqIDSet := make(map[int64]struct{})\n\n\tif len(req.FirstPriorityTagIDs) > 0 {\n\t\ttags, err := s.tagRepo.GetByIDs(ctx, req.FirstPriorityTagIDs)\n\t\tif err == nil {\n\t\t\tfirstPriorityTagUUIDs = make([]string, 0, len(tags))\n\t\t\tfor _, tag := range tags {\n\t\t\t\tfirstPriorityTagUUIDs = append(firstPriorityTagUUIDs, tag.ID)\n\t\t\t\tfirstPrioritySeqIDSet[tag.SeqID] = struct{}{}\n\t\t\t}\n\t\t}\n\t}\n\tif len(req.SecondPriorityTagIDs) > 0 {\n\t\ttags, err := s.tagRepo.GetByIDs(ctx, req.SecondPriorityTagIDs)\n\t\tif err == nil {\n\t\t\tsecondPriorityTagUUIDs = make([]string, 0, len(tags))\n\t\t\tfor _, tag := range tags {\n\t\t\t\tsecondPriorityTagUUIDs = append(secondPriorityTagUUIDs, tag.ID)\n\t\t\t\tsecondPrioritySeqIDSet[tag.SeqID] = struct{}{}\n\t\t\t}\n\t\t}\n\t}"
	new = "\t// Tag IDs are now UUIDs directly from request\n\tfirstPriorityTagUUIDs := req.FirstPriorityTagIDs\n\tsecondPriorityTagUUIDs := req.SecondPriorityTagIDs\n\n\t// Build sets for priority matching\n\tfirstPriorityIDSet := make(map[string]struct{})\n\tfor _, id := range firstPriorityTagUUIDs {\n\t\tfirstPriorityIDSet[id] = struct{}{}\n\t}\n\tsecondPriorityIDSet := make(map[string]struct{})\n\tfor _, id := range secondPriorityTagUUIDs {\n\t\tsecondPriorityIDSet[id] = struct{}{}\n\t}"
	if strings.Contains(src, old) {
		src = strings.Replace(src, old, new, 1)
		count++
	}

	// 3. Fix tag.SeqID at line 5862 (tagSeqIDMap[tag.ID] = tag.SeqID)
	old = "\tif len(tagIDs) > 0 {\n\t\ttags, err := s.tagRepo.GetByIDs(ctx, tagIDs)\n\t\tif err == nil {\n\t\t\tfor _, tag := range tags {\n\t\t\t\ttagSeqIDMap[tag.ID] = tag.SeqID\n\t\t\t}\n\t\t}\n\t}"
	new = "\tif len(tagIDs) > 0 {\n\t\ttags, err := s.tagRepo.GetByIDs(ctx, tagIDs)\n\t\tif err == nil {\n\t\t\tfor _, tag := range tags {\n\t\t\t\ttagSeqIDMap[tag.ID] = 0 // SeqID removed\n\t\t\t}\n\t\t}\n\t}"
	if strings.Contains(src, old) {
		src = strings.ReplaceAll(src, old, new)
		count++
	}

	// 4. Fix tag name lookup block (lines 5967-5998) that uses int64 tagSeqIDs
	old = "\t// 批量查询TagName并补充到结果中\n\tif len(entries) > 0 {\n\t\t// 收集所有需要查询的TagID (seq_id)\n\t\ttagSeqIDs := make([]int64, 0)\n\t\ttagSeqIDSet := make(map[int64]struct{})\n\t\tfor _, entry := range entries {\n\t\t\tif entry.TagID != 0 {\n\t\t\t\tif _, exists := tagSeqIDSet[entry.TagID]; !exists {\n\t\t\t\t\ttagSeqIDs = append(tagSeqIDs, entry.TagID)\n\t\t\t\t\ttagSeqIDSet[entry.TagID] = struct{}{}\n\t\t\t\t}\n\t\t\t}\n\t\t}\n\n\t\t// 批量查询标签\n\t\tif len(tagSeqIDs) > 0 {\n\t\t\ttags, err := s.tagRepo.GetByIDs(ctx, tagSeqIDs)\n\t\t\tif err != nil {\n\t\t\t\tlogger.Warnf(ctx, \"Failed to batch query tags: %v\", err)\n\t\t\t} else {\n\t\t\t\t// 构建TagSeqID到TagName的映射\n\t\t\t\ttagNameMap := make(map[int64]string)\n\t\t\t\tfor _, tag := range tags {\n\t\t\t\t\ttagNameMap[tag.SeqID] = tag.Name\n\t\t\t\t}"
	new = "\t// 批量查询TagName并补充到结果中\n\tif len(entries) > 0 {\n\t\t// 收集所有需要查询的TagID (UUID)\n\t\ttagIDsForName := make([]string, 0)\n\t\ttagIDSetForName := make(map[string]struct{})\n\t\tfor _, entry := range entries {\n\t\t\tif entry.TagID != \"\" {\n\t\t\t\tif _, exists := tagIDSetForName[entry.TagID]; !exists {\n\t\t\t\t\ttagIDsForName = append(tagIDsForName, entry.TagID)\n\t\t\t\t\ttagIDSetForName[entry.TagID] = struct{}{}\n\t\t\t\t}\n\t\t\t}\n\t\t}\n\n\t\t// 批量查询标签\n\t\tif len(tagIDsForName) > 0 {\n\t\t\ttags, err := s.tagRepo.GetByIDs(ctx, tagIDsForName)\n\t\t\tif err != nil {\n\t\t\t\tlogger.Warnf(ctx, \"Failed to batch query tags: %v\", err)\n\t\t\t} else {\n\t\t\t\t// 构建TagID到TagName的映射\n\t\t\t\ttagNameMap := make(map[string]string)\n\t\t\t\tfor _, tag := range tags {\n\t\t\t\t\ttagNameMap[tag.ID] = tag.Name\n\t\t\t\t}"
	if strings.Contains(src, old) {
		src = strings.Replace(src, old, new, 1)
		count++
	}

	// 5. Fix the tag name assignment after the batch query (entry.TagID != 0 → != "")
	old = "\t\t\t// 补充TagName\n\t\t\t\tfor _, entry := range entries {\n\t\t\t\t\tif entry.TagID != 0 {"
	new = "\t\t\t// 补充TagName\n\t\t\t\tfor _, entry := range entries {\n\t\t\t\t\tif entry.TagID != \"\" {"
	if strings.Contains(src, old) {
		src = strings.Replace(src, old, new, 1)
		count++
	}

	// 6. Fix unused tenantID at line 6020 (DeleteFAQEntries)
	old = "\ttenantID := ctx.Value(types.TenantIDContextKey).(uint64)\n\tvar faqKnowledge *types.Knowledge\n\tchunksToRemove := make([]*types.Chunk, 0, len(entrySeqIDs))"
	new = "\tvar faqKnowledge *types.Knowledge\n\tchunksToRemove := make([]*types.Chunk, 0, len(entrySeqIDs))"
	if strings.Contains(src, old) {
		src = strings.Replace(src, old, new, 1)
		count++
	}

	// 7. Fix firstPrioritySeqIDSet → firstPriorityIDSet references
	src = strings.ReplaceAll(src, "firstPrioritySeqIDSet", "firstPriorityIDSet")
	src = strings.ReplaceAll(src, "secondPrioritySeqIDSet", "secondPriorityIDSet")
	count++

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
