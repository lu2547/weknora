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

	// 1. Fix createKnowledgeFromPassageInternal calls - remove channel arg
	old := "return s.createKnowledgeFromPassageInternal(ctx, kbID, passage, false, channel)"
	new := "return s.createKnowledgeFromPassageInternal(ctx, kbID, passage, false)"
	if strings.Contains(src, old) {
		src = strings.Replace(src, old, new, 1)
		count++
	}

	old = "return s.createKnowledgeFromPassageInternal(ctx, kbID, passage, true, channel)"
	new = "return s.createKnowledgeFromPassageInternal(ctx, kbID, passage, true)"
	if strings.Contains(src, old) {
		src = strings.Replace(src, old, new, 1)
		count++
	}

	// 2. Fix createKnowledgeFromPassageInternal signature - remove channel param
	old = "kbID string, passage []string, syncMode bool, channel string,"
	new = "kbID string, passage []string, syncMode bool,"
	if strings.Contains(src, old) {
		src = strings.Replace(src, old, new, 1)
		count++
	}

	// 3. Fix unused tenantID at line ~2662 (UpdateManualKnowledge)
	old = "\ttenantID := ctx.Value(types.TenantIDContextKey).(uint64)\n\texisting, err := s.repo.GetKnowledgeByID(ctx, knowledgeID)\n\tif err != nil {\n\t\tlogger.Errorf(ctx, \"Failed to load knowledge: %v\", err)\n\t\treturn nil, err\n\t}\n\tif !existing.IsManual() {"
	new = "\texisting, err := s.repo.GetKnowledgeByID(ctx, knowledgeID)\n\tif err != nil {\n\t\tlogger.Errorf(ctx, \"Failed to load knowledge: %v\", err)\n\t\treturn nil, err\n\t}\n\tif !existing.IsManual() {"
	if strings.Contains(src, old) {
		src = strings.Replace(src, old, new, 1)
		count++
	}

	// 4. Fix unused kb at line ~2672 (in UpdateManualKnowledge)
	old = "\tkb, err := s.kbService.GetKnowledgeBaseByID(ctx, existing.KnowledgeBaseID)\n\tif err != nil {\n\t\tlogger.Errorf(ctx, \"Failed to get knowledge base for manual update: %v\", err)\n\t\treturn nil, err\n\t}"
	new = "\t_, err = s.kbService.GetKnowledgeBaseByID(ctx, existing.KnowledgeBaseID)\n\tif err != nil {\n\t\tlogger.Errorf(ctx, \"Failed to get knowledge base for manual update: %v\", err)\n\t\treturn nil, err\n\t}"
	if strings.Contains(src, old) {
		src = strings.Replace(src, old, new, 1)
		count++
	}

	// 5. Fix unused tenantID at line ~2769 (ReparseKnowledge)
	old = "\ttenantID := ctx.Value(types.TenantIDContextKey).(uint64)\n\texisting, err := s.repo.GetKnowledgeByID(ctx, knowledgeID)\n\tif err != nil {\n\t\tlogger.Errorf(ctx, \"Failed to load knowledge: %v\", err)\n\t\treturn nil, err\n\t}\n\n\t// Get knowledge base configuration"
	new = "\texisting, err := s.repo.GetKnowledgeByID(ctx, knowledgeID)\n\tif err != nil {\n\t\tlogger.Errorf(ctx, \"Failed to load knowledge: %v\", err)\n\t\treturn nil, err\n\t}\n\n\t// Get knowledge base configuration"
	if strings.Contains(src, old) {
		src = strings.Replace(src, old, new, 1)
		count++
	}

	// 6. Fix unused tenantID in dead code block (file_url && false)
	old = "\tif existing.Type == \"file_url\" && false {\n\t\ttenantID := ctx.Value(types.TenantIDContextKey).(uint64)\n\n\t\tenableMultimodel := false"
	new = "\tif existing.Type == \"file_url\" && false {\n\t\tenableMultimodel := false"
	if strings.Contains(src, old) {
		src = strings.Replace(src, old, new, 1)
		count++
	}

	// 7. Fix unused tenantID in dead code block (url && false)
	old = "\tif existing.Type == \"url\" && false {\n\t\ttenantID := ctx.Value(types.TenantIDContextKey).(uint64)\n\n\t\tenableMultimodel := false"
	new = "\tif existing.Type == \"url\" && false {\n\t\tenableMultimodel := false"
	if strings.Contains(src, old) {
		src = strings.Replace(src, old, new, 1)
		count++
	}

	// 8. Fix unused srcKB in CloneKnowledgeBase
	old = "\tsrcKB, dstKB, err := s.kbService.CopyKnowledgeBase(ctx, srcID, dstID)"
	new = "\t_, dstKB, err := s.kbService.CopyKnowledgeBase(ctx, srcID, dstID)"
	if strings.Contains(src, old) {
		src = strings.Replace(src, old, new, 1)
		count++
	}

	// 9. Fix untyped nil assignments in CloneKnowledgeBase
	old = "\taddKnowledge, err := nil, nil\n\tif err != nil {\n\t\tlogger.Errorf(ctx, \"Failed to get knowledge: %v\", err)\n\t\treturn err\n\t}\n\n\tdelKnowledge, err := nil, nil\n\tif err != nil {\n\t\tlogger.Errorf(ctx, \"Failed to get knowledge: %v\", err)\n\t\treturn err\n\t}"
	new = "\t// TODO: implement knowledge diff logic\n\tvar addKnowledge []string\n\tvar delKnowledge []string"
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
