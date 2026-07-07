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

	// Fix 1: types.ChannelWeb -> "web"
	content = strings.Replace(content, "types.ChannelWeb", `"web"`, -1)

	// Fix 2: CreateKnowledgeFromFile signature
	content = strings.Replace(content,
		"func (s *knowledgeService) CreateKnowledgeFromFile(ctx context.Context,\n\tkbID string, file *multipart.FileHeader, metadata map[string]string, enableMultimodel *bool, customFileName string, tagID string, channel string,\n) (*types.Knowledge, error) {",
		"func (s *knowledgeService) CreateKnowledgeFromFile(ctx context.Context,\n\tkbID string, file *multipart.FileHeader, customFileName string, tagID string,\n) (*types.Knowledge, error) {",
		1)

	// Fix 3: Remove metadata JSON block
	content = strings.Replace(content,
		"\t// Convert metadata to JSON format if provided\n\tvar metadataJSON types.JSON\n\tif metadata != nil {\n\t\tmetadataBytes, err := json.Marshal(metadata)\n\t\tif err != nil {\n\t\t\tlogger.Errorf(ctx, \"Failed to marshal metadata: %v\", err)\n\t\t\treturn nil, err\n\t\t}\n\t\tmetadataJSON = types.JSON(metadataBytes)\n\t}\n",
		"",
		1)

	// Fix 4: Remove Metadata field from Knowledge struct literal
	content = strings.Replace(content,
		"\t\t// EmbeddingModelID set below via getDefaultEmbeddingModelID\n\t\tMetadata: metadataJSON,",
		"\t\t// EmbeddingModelID set below via getDefaultEmbeddingModelID",
		1)

	// Fix 5: Fix enableMultimodel usage in CreateKnowledgeFromFile
	content = strings.Replace(content,
		"\tenableMultimodelValue := false\n\tif enableMultimodel != nil {\n\t\tenableMultimodelValue = *enableMultimodel\n\t} else {\n\t\tenableMultimodelValue = false\n\t}",
		"\tenableMultimodelValue := false",
		1)

	// Fix 6: CreateKnowledgeFromURL signature
	content = strings.Replace(content,
		"func (s *knowledgeService) CreateKnowledgeFromURL(ctx context.Context,\n\tkbID string, rawURL string, fileName string, fileType string, enableMultimodel *bool, title string, tagID string, channel string,\n) (*types.Knowledge, error) {",
		"func (s *knowledgeService) CreateKnowledgeFromURL(ctx context.Context,\n\tkbID string, rawURL string, fileName string, fileType string, title string, tagID string,\n) (*types.Knowledge, error) {",
		1)

	// Fix 7: Fix call to createKnowledgeFromFileURL
	content = strings.Replace(content,
		"return s.createKnowledgeFromFileURL(ctx, kbID, rawURL, fileName, fileType, enableMultimodel, title, tagID, channel)",
		"return s.createKnowledgeFromFileURL(ctx, kbID, rawURL, fileName, fileType, title, tagID)",
		1)

	// Fix 8: createKnowledgeFromFileURL signature
	content = strings.Replace(content,
		"func (s *knowledgeService) createKnowledgeFromFileURL(\n\tctx context.Context,\n\tkbID string,\n\tfileURL string,\n\tfileName string,\n\tfileType string,\n\tenableMultimodel *bool,\n\ttitle string,\n\ttagID string,\n\tchannel string,\n) (*types.Knowledge, error) {",
		"func (s *knowledgeService) createKnowledgeFromFileURL(\n\tctx context.Context,\n\tkbID string,\n\tfileURL string,\n\tfileName string,\n\tfileType string,\n\ttitle string,\n\ttagID string,\n) (*types.Knowledge, error) {",
		1)

	// Fix 9: unused tenantID (URL hash check)
	content = strings.Replace(content,
		"\t// Check for duplicate (by URL hash)\n\ttenantID := ctx.Value(types.TenantIDContextKey).(uint64)\n\tfileHash := calculateStr(fileURL)",
		"\t// Check for duplicate (by URL hash)\n\tfileHash := calculateStr(fileURL)",
		1)

	// Fix 10: unused tenantID in CreateKnowledgeFromManual
	content = strings.Replace(content,
		"\ttenantID := ctx.Value(types.TenantIDContextKey).(uint64)\n\tnow := time.Now()",
		"\tnow := time.Now()",
		1)

	// Fix 11: Remove NewManualKnowledgeMetadata
	content = strings.Replace(content,
		"\tfileName := ensureManualFileName(title)\n\tmeta := types.NewManualKnowledgeMetadata(cleanContent, status, 1)\n",
		"\tfileName := ensureManualFileName(title)\n",
		1)

	// Fix 12: Remove SetManualMetadata and EnsureManualDefaults
	content = strings.Replace(content,
		"\tif err := knowledge.SetManualMetadata(meta); err != nil {\n\t\tlogger.Errorf(ctx, \"Failed to set manual metadata: %v\", err)\n\t\treturn nil, err\n\t}\n\tknowledge.EnsureManualDefaults()\n",
		"",
		1)

	// Fix 13: unused tenantID in passage processing
	content = strings.Replace(content,
		"\t\tlogger.Info(ctx, \"Enqueuing passage processing task to Asynq\")\n\t\ttenantID := ctx.Value(types.TenantIDContextKey).(uint64)\n",
		"\t\tlogger.Info(ctx, \"Enqueuing passage processing task to Asynq\")\n",
		1)

	// Fix 14: s.repo.GetKnowledgeByIDOnly -> s.repo.GetKnowledgeByID
	content = strings.ReplaceAll(content, "s.repo.GetKnowledgeByIDOnly(", "s.repo.GetKnowledgeByID(")

	// Fix 15: Fix all remaining enableMultimodel patterns in other functions
	content = strings.ReplaceAll(content,
		"\tenableMultimodelValue := false\n\tif enableMultimodel != nil {\n\t\tenableMultimodelValue = *enableMultimodel\n\t}\n",
		"\tenableMultimodelValue := false\n")

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
