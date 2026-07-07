package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {
	path := "/Users/frankie/Documents/work/workspace/ai-app/WeKnora/internal/application/service/knowledge.go"
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Printf("Error reading file: %v\n", err)
		os.Exit(1)
	}
	content := string(data)
	original := content

	// Fix 1: CreateKnowledgeFromPassage - remove channel parameter
	content = strings.Replace(content,
		"func (s *knowledgeService) CreateKnowledgeFromPassage(ctx context.Context,\n\tkbID string, passage []string, channel string,\n) (*types.Knowledge, error) {",
		"func (s *knowledgeService) CreateKnowledgeFromPassage(ctx context.Context,\n\tkbID string, passage []string,\n) (*types.Knowledge, error) {",
		1)

	// Fix 2: CreateKnowledgeFromPassageSync - remove channel parameter
	content = strings.Replace(content,
		"func (s *knowledgeService) CreateKnowledgeFromPassageSync(ctx context.Context,\n\tkbID string, passage []string, channel string,\n) (*types.Knowledge, error) {",
		"func (s *knowledgeService) CreateKnowledgeFromPassageSync(ctx context.Context,\n\tkbID string, passage []string,\n) (*types.Knowledge, error) {",
		1)

	// Fix 3: title variable conflict (line 832 area)
	// The parameter is now 'title string' but there's `title := safeTitle` which conflicts
	content = strings.Replace(content,
		"\tnow := time.Now()\n\ttitle := safeTitle\n\tif title == \"\" {\n\t\ttitle = fmt.Sprintf(\"Knowledge-%s\", now.Format(\"20060102-150405\"))\n\t}",
		"\tnow := time.Now()\n\teffTitle := safeTitle\n\tif effTitle == \"\" {\n\t\teffTitle = fmt.Sprintf(\"Knowledge-%s\", now.Format(\"20060102-150405\"))\n\t}",
		1)

	// Fix references to title after it becomes effTitle in that function
	// fileName uses title
	content = strings.Replace(content,
		"\tfileName := ensureManualFileName(title)\n",
		"\tfileName := ensureManualFileName(effTitle)\n",
		1)

	// Knowledge struct Title field
	content = strings.Replace(content,
		"\t\tType:            types.KnowledgeTypeManual,\n\t\tTitle:           title,",
		"\t\tType:            types.KnowledgeTypeManual,\n\t\tTitle:           effTitle,",
		1)

	// Fix 4: unused kb in HandleSummaryGeneration (line 2123 area)
	content = strings.Replace(content,
		"\t// Get knowledge base\n\tkb, err := s.kbService.GetKnowledgeBaseByID(ctx, payload.KnowledgeBaseID)\n\tif err != nil {\n\t\tlogger.Errorf(ctx, \"Failed to get knowledge base: %v\", err)\n\t\treturn nil\n\t}\n\n\tif \"\" == \"\" {\n\t\tlogger.Warn(ctx, \"Knowledge base summary model ID is empty, skipping summary generation\")\n\t\treturn nil\n\t}",
		"\t// Get knowledge base (validate it exists)\n\tif _, err := s.kbService.GetKnowledgeBaseByID(ctx, payload.KnowledgeBaseID); err != nil {\n\t\tlogger.Errorf(ctx, \"Failed to get knowledge base: %v\", err)\n\t\treturn nil\n\t}",
		1)

	// Fix 5: unused kb in HandleQuestionGeneration (line 2356 area)
	content = strings.Replace(content,
		"\t// Get knowledge base\n\tkb, err := s.kbService.GetKnowledgeBaseByID(ctx, payload.KnowledgeBaseID)\n\tif err != nil {\n\t\tlogger.Errorf(ctx, \"Failed to get knowledge base: %v\", err)\n\t\treturn nil\n\t}\n\n\t// Get knowledge\n\tknowledge, err := s.repo.GetKnowledgeByID(ctx, payload.KnowledgeID)",
		"\t// Get knowledge base (validate it exists)\n\tif _, err := s.kbService.GetKnowledgeBaseByID(ctx, payload.KnowledgeBaseID); err != nil {\n\t\tlogger.Errorf(ctx, \"Failed to get knowledge base: %v\", err)\n\t\treturn nil\n\t}\n\n\t// Get knowledge\n\tknowledge, err := s.repo.GetKnowledgeByID(ctx, payload.KnowledgeID)",
		1)

	// Fix 6: unused tenantID in GetKnowledgeFile (line 2584)
	content = strings.Replace(content,
		"func (s *knowledgeService) GetKnowledgeFile(ctx context.Context, id string) (io.ReadCloser, string, error) {\n\t// Get knowledge record\n\ttenantID := ctx.Value(types.TenantIDContextKey).(uint64)\n\tknowledge, err := s.repo.GetKnowledgeByID(ctx, id)",
		"func (s *knowledgeService) GetKnowledgeFile(ctx context.Context, id string) (io.ReadCloser, string, error) {\n\t// Get knowledge record\n\tknowledge, err := s.repo.GetKnowledgeByID(ctx, id)",
		1)

	if content == original {
		fmt.Println("WARNING: No changes made!")
		os.Exit(1)
	}

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		fmt.Printf("Error writing file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully applied fixes. File size: %d -> %d bytes\n", len(original), len(content))
}
