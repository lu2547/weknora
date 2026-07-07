//go:build ignore

package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {
	filePath := "/Users/frankie/Documents/work/workspace/ai-app/WeKnora/internal/application/service/knowledge.go"
	data, err := os.ReadFile(filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read: %v\n", err)
		os.Exit(1)
	}
	content := string(data)
	type R struct{ o, n string }
	reps := []R{
		{`s.repo.ListKnowledgeByKnowledgeBaseID(ctx, ctx.Value(types.TenantIDContextKey).(uint64), kbID)`, `s.repo.ListKnowledgeByKnowledgeBaseID(ctx, kbID)`},
		{`s.repo.ListKnowledgeByKnowledgeBaseID(ctx, tenantID, kbID)`, `s.repo.ListKnowledgeByKnowledgeBaseID(ctx, kbID)`},
		{`s.repo.ListKnowledgeByKnowledgeBaseID(ctx, srcKB.TenantID, srcKB.ID)`, `s.repo.ListKnowledgeByKnowledgeBaseID(ctx, srcKB.ID)`},
		{`s.repo.ListKnowledgeByKnowledgeBaseID(ctx, kb.TenantID, kb.ID)`, `s.repo.ListKnowledgeByKnowledgeBaseID(ctx, kb.ID)`},
		{`s.repo.GetKnowledgeByID(gctx, srcKB.TenantID, knowledge)`, `s.repo.GetKnowledgeByID(gctx, knowledge)`},
		{`func (s *knowledgeService) isKnowledgeDeleting(ctx context.Context, tenantID uint64, knowledgeID string) bool {`, `func (s *knowledgeService) isKnowledgeDeleting(ctx context.Context, knowledgeID string) bool {`},
		{`s.isKnowledgeDeleting(ctx, knowledge.TenantID, knowledge.ID)`, `s.isKnowledgeDeleting(ctx, knowledge.ID)`},
		{`s.repo.DeleteKnowledge(ctx, ctx.Value(types.TenantIDContextKey).(uint64), id)`, `s.repo.DeleteKnowledge(ctx, id)`},
		{`s.repo.DeleteKnowledgeList(ctx, tenantID, knowledgeIDs)`, `s.repo.DeleteKnowledgeList(ctx, knowledgeIDs)`},
		{`s.chunkRepo.FAQChunkDiff(ctx, srcKB.TenantID, srcKB.ID, dstKB.TenantID, dstKB.ID)`, `s.chunkRepo.FAQChunkDiff(ctx, srcKB.ID, dstKB.ID)`},
		{`s.chunkRepo.ListChunksByID(ctx, srcKB.TenantID, batchIDs)`, `s.chunkRepo.ListChunksByID(ctx, batchIDs)`},
		{`enableMultimodelValue = kb.IsMultimodalEnabled()`, `enableMultimodelValue = false`},
		{`s.modelService.GetEmbeddingModel(ctx, kb.EmbeddingModelID)`, `s.modelService.GetEmbeddingModel(ctx, knowledge.EmbeddingModelID)`},
		{`attribute.Int("tenant_id", int(knowledge.TenantID)),`, `// tenant_id tracing removed`},
		{`attribute.String("embedding_model_id", kb.EmbeddingModelID),`, `attribute.String("embedding_model_id", knowledge.EmbeddingModelID),`},
		{`s.resolveFileService(ctx, kb).SaveFile(ctx, file, knowledge.TenantID, knowledge.ID)`, `s.resolveFileService(ctx, kb).SaveFile(ctx, file, 0, knowledge.ID)`},
	}
	count := 0
	for _, r := range reps {
		if strings.Contains(content, r.o) {
			n := strings.Count(content, r.o)
			content = strings.ReplaceAll(content, r.o, r.n)
			count += n
			fmt.Printf("  [%d] %s\n", n, r.o[:min(70, len(r.o))])
		}
	}
	if count == 0 {
		fmt.Println("No replacements")
		return
	}
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "write: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("\nDone: %d replacements\n", count)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
