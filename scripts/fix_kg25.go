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

	// 1. Fix chunkToFAQEntry: tagSeqID int64 → string
	old := "\tvar tagSeqID int64\n\tif chunk.TagID != \"\" && tagSeqIDMap != nil {\n\t\ttagSeqID = tagSeqIDMap[chunk.TagID]\n\t}\n\n\tentry := &types.FAQEntry{\n\t\tID:                chunk.SeqID,\n\t\tChunkID:           chunk.ID,\n\t\tKnowledgeID:       chunk.KnowledgeID,\n\t\tKnowledgeBaseID:   chunk.KnowledgeBaseID,\n\t\tTagID:             tagSeqID,"
	new := "\t// TagID is now UUID string - use chunk's tag ID directly\n\ttagIDStr := chunk.TagID\n\n\tentry := &types.FAQEntry{\n\t\tID:                chunk.SeqID,\n\t\tChunkID:           chunk.ID,\n\t\tKnowledgeID:       chunk.KnowledgeID,\n\t\tKnowledgeBaseID:   chunk.KnowledgeBaseID,\n\t\tTagID:             tagIDStr,"
	if strings.Contains(src, old) {
		src = strings.Replace(src, old, new, 1)
		count++
	}

	// 2. Fix buildStorageConfig: kb.StorageConfig → use tenant storage config
	old = "func (s *knowledgeService) buildStorageConfig(ctx context.Context, kb *types.KnowledgeBase) *types.DocParserStorageConfig {\n\tprovider := \"\"\n\tif provider == \"\" {\n\t\tprovider = \"local\"\n\t}\n\n\t// Backward compatibility: if legacy cos_config has full params for the chosen provider, use them.\n\tsc := &kb.StorageConfig\n\thasKBFull := false\n\tswitch provider {\n\tcase \"cos\":\n\t\thasKBFull = sc.SecretID != \"\" && sc.BucketName != \"\"\n\tcase \"minio\":\n\t\thasKBFull = sc.BucketName != \"\"\n\tcase \"local\":\n\t\thasKBFull = false\n\t}"
	new = "func (s *knowledgeService) buildStorageConfig(ctx context.Context, kb *types.KnowledgeBase) *types.DocParserStorageConfig {\n\t// StorageConfig removed from KB; use tenant default\n\tprovider := \"local\"\n\ttenantInfo, _ := ctx.Value(types.TenantInfoContextKey).(*types.Tenant)\n\tif tenantInfo != nil && tenantInfo.StorageEngineConfig.DefaultProvider != \"\" {\n\t\tprovider = tenantInfo.StorageEngineConfig.DefaultProvider\n\t}\n\n\thasKBFull := false"
	if strings.Contains(src, old) {
		src = strings.Replace(src, old, new, 1)
		count++
	}

	// 3. Fix unused tenantID in GetFAQImportTaskStatus (line 8435)
	old = "\tif progress.Status == types.FAQImportStatusCompleted && progress.KnowledgeID != \"\" {\n\t\ttenantID := ctx.Value(types.TenantIDContextKey).(uint64)\n\t\tknowledge, err := s.repo.GetKnowledgeByID(ctx, progress.KnowledgeID)"
	new = "\tif progress.Status == types.FAQImportStatusCompleted && progress.KnowledgeID != \"\" {\n\t\tknowledge, err := s.repo.GetKnowledgeByID(ctx, progress.KnowledgeID)"
	if strings.Contains(src, old) {
		src = strings.Replace(src, old, new, 1)
		count++
	}

	// 4. Fix unused tenantID in UpdateLastFAQImportResultDisplayStatus (line 8459)
	old = "\t// 获取当前租户ID\n\ttenantID := ctx.Value(types.TenantIDContextKey).(uint64)\n\n\t// 查找FAQ类型的knowledge"
	new = "\t// 查找FAQ类型的knowledge"
	if strings.Contains(src, old) {
		src = strings.Replace(src, old, new, 1)
		count++
	}

	// 5. Fix nil assignments in HandleKnowledgeBaseSync (same pattern as CloneKnowledgeBase)
	old = "\t// Document type: use Knowledge-level diff based on file_hash\n\taddKnowledge, err := nil, nil\n\tif err != nil {\n\t\tlogger.Errorf(ctx, \"Failed to get knowledge to add: %v\", err)\n\t\thandleError(progress, err, \"Failed to calculate knowledge difference\")\n\t\treturn err\n\t}\n\n\tdelKnowledge, err := nil, nil\n\tif err != nil {\n\t\tlogger.Errorf(ctx, \"Failed to get knowledge to delete: %v\", err)\n\t\thandleError(progress, err, \"Failed to calculate knowledge difference\")\n\t\treturn err\n\t}"
	new = "\t// TODO: implement knowledge diff logic for sync\n\tvar addKnowledge []string\n\tvar delKnowledge []string"
	if strings.Contains(src, old) {
		src = strings.Replace(src, old, new, 1)
		count++
	}

	// 6. Fix dstKB.EmbeddingModelID (line 8733) → use default embedding model
	old = "\t// Get embedding model\n\tembeddingModel, err := s.modelService.GetEmbeddingModel(ctx, dstKB.EmbeddingModelID)"
	new = "\t// Get default embedding model\n\tdefEmbModelIDSync, err := s.getDefaultEmbeddingModelIDFromModels(ctx)\n\tif err != nil {\n\t\tlogger.Errorf(ctx, \"Failed to get default embedding model: %v\", err)\n\t\thandleError(progress, err, \"Failed to get embedding model\")\n\t\treturn err\n\t}\n\tembeddingModel, err := s.modelService.GetEmbeddingModel(ctx, defEmbModelIDSync)"
	if strings.Contains(src, old) {
		src = strings.Replace(src, old, new, 1)
		count++
	}

	// 7. Fix sourceKB.EmbeddingModelID / targetKB.EmbeddingModelID comparison (line 9056-9057)
	// Remove the embedding model check entirely since KB no longer stores it
	old = "\tif sourceKB.EmbeddingModelID != targetKB.EmbeddingModelID {\n\t\terr := fmt.Errorf(\"embedding model mismatch: source=%s, target=%s\", sourceKB.EmbeddingModelID, targetKB.EmbeddingModelID)\n\t\thandleError(progress, err, \"Source and target must use the same embedding model\")\n\t\treturn err\n\t}"
	new = "\t// EmbeddingModelID removed from KB - system uses default embedding model for all KBs"
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
