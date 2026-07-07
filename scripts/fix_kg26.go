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

	// 1. Remove dead code block `if hasKBFull { ... }` that references removed `sc` variable
	old := "\thasKBFull := false\n\n\tif hasKBFull {\n\t\tlogger.Infof(ctx, \"[storage] buildStorageConfig use legacy kb config: kb=%s provider=%s bucket=%s path_prefix=%s\",\n\t\t\tkb.ID, provider, sc.BucketName, sc.PathPrefix)\n\t\treturn &types.DocParserStorageConfig{\n\t\t\tProvider:        strings.ToUpper(provider),\n\t\t\tRegion:          sc.Region,\n\t\t\tBucketName:      sc.BucketName,\n\t\t\tAccessKeyID:     sc.SecretID,\n\t\t\tSecretAccessKey: sc.SecretKey,\n\t\t\tAppID:           sc.AppID,\n\t\t\tPathPrefix:      sc.PathPrefix,\n\t\t}\n\t}"
	new := ""
	if strings.Contains(src, old) {
		src = strings.Replace(src, old, new, 1)
		count++
	}

	// 2. Remove unused tenantID in moveOneKnowledge
	old = "func (s *knowledgeService) moveOneKnowledge(\n\tctx context.Context,\n\tknowledgeID string,\n\tsourceKB, targetKB *types.KnowledgeBase,\n\tmode string,\n) error {\n\ttenantID := ctx.Value(types.TenantIDContextKey).(uint64)\n"
	new = "func (s *knowledgeService) moveOneKnowledge(\n\tctx context.Context,\n\tknowledgeID string,\n\tsourceKB, targetKB *types.KnowledgeBase,\n\tmode string,\n) error {\n"
	if strings.Contains(src, old) {
		src = strings.Replace(src, old, new, 1)
		count++
	}

	// 3. Remove unused tenantID in moveKnowledgeReuseVectors
	old = ") error {\n\ttenantID := ctx.Value(types.TenantIDContextKey).(uint64)\n\ttenantInfo := ctx.Value(types.TenantInfoContextKey).(*types.Tenant)\n\n\t// 1. Get old chunk IDs for vector index copy mapping\n\toldChunks, err := s.chunkRepo.ListChunksByKnowledgeID(ctx, knowledge.ID)"
	new = ") error {\n\ttenantInfo := ctx.Value(types.TenantInfoContextKey).(*types.Tenant)\n\n\t// 1. Get old chunk IDs for vector index copy mapping\n\toldChunks, err := s.chunkRepo.ListChunksByKnowledgeID(ctx, knowledge.ID)"
	if strings.Contains(src, old) {
		src = strings.Replace(src, old, new, 1)
		count++
	}

	// 4. Remove unused tenantID in moveKnowledgeReparse
	old = "func (s *knowledgeService) moveKnowledgeReparse(\n\tctx context.Context,\n\tknowledge *types.Knowledge,\n\t_, targetKB *types.KnowledgeBase,\n) error {\n\ttenantID := ctx.Value(types.TenantIDContextKey).(uint64)\n"
	new = "func (s *knowledgeService) moveKnowledgeReparse(\n\tctx context.Context,\n\tknowledge *types.Knowledge,\n\t_, targetKB *types.KnowledgeBase,\n) error {\n"
	if strings.Contains(src, old) {
		src = strings.Replace(src, old, new, 1)
		count++
	}

	// 5. Fix knowledge.EmbeddingModelID = targetKB.EmbeddingModelID
	old = "\tknowledge.EmbeddingModelID = targetKB.EmbeddingModelID\n\tknowledge.TagID = \"\" // Clear tag since tags are KB-scoped"
	new = "\tknowledge.TagID = \"\" // Clear tag since tags are KB-scoped"
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
