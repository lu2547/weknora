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

	// Fix 1: enableMultimodel in CreateKnowledgeFromURL (lines 535-540)
	content = strings.Replace(content,
		"\tenableMultimodelValue := false\n\tif enableMultimodel != nil {\n\t\tenableMultimodelValue = *enableMultimodel\n\t} else {\n\t\tenableMultimodelValue = false\n\t}",
		"\tenableMultimodelValue := false",
		-1) // replace all occurrences

	// Fix 2: unused tenantID line 1107
	content = strings.Replace(content,
		"\t// Collect image URLs before chunks are deleted (ImageInfo references are lost after deletion)\n\ttenantID := ctx.Value(types.TenantIDContextKey).(uint64)\n\tchunkImageInfos",
		"\t// Collect image URLs before chunks are deleted (ImageInfo references are lost after deletion)\n\tchunkImageInfos",
		1)

	// Fix 3: kb.ExtractConfig block (line 1827)
	content = strings.Replace(content,
		"\tif kb.ExtractConfig != nil && kb.ExtractConfig.Enabled {\n\t\tfor _, chunk := range textChunks {\n\t\t\terr := NewChunkExtractTask(ctx, s.task, 0, chunk.ID, \"\")\n\t\t\tif err != nil {\n\t\t\t\tlogger.GetLogger(ctx).WithField(\"error\", err).Errorf(\"processChunks create chunk extract task failed\")\n\t\t\t\tspan.RecordError(err)\n\t\t\t}\n\t\t}\n\t}",
		"\t// TODO: ExtractConfig removed from KB; graph extraction disabled\n\t_ = textChunks",
		1)

	// Fix 4: Add TenantID to QuestionGenerationPayload
	content = strings.Replace(content,
		"\ttenantID := ctx.Value(types.TenantIDContextKey).(uint64)\n\tlang, _ := types.LanguageFromContext(ctx)\n\tpayload := types.QuestionGenerationPayload{\n\t\tKnowledgeBaseID: kbID,\n\t\tKnowledgeID:     knowledgeID,\n\t\tQuestionCount:   questionCount,\n\t\tLanguage:        lang,\n\t}",
		"\ttenantID := ctx.Value(types.TenantIDContextKey).(uint64)\n\tlang, _ := types.LanguageFromContext(ctx)\n\tpayload := types.QuestionGenerationPayload{\n\t\tTenantID:        tenantID,\n\t\tKnowledgeBaseID: kbID,\n\t\tKnowledgeID:     knowledgeID,\n\t\tQuestionCount:   questionCount,\n\t\tLanguage:        lang,\n\t}",
		1)

	// Fix 5: Add TenantID to SummaryGenerationPayload
	content = strings.Replace(content,
		"\ttenantID := ctx.Value(types.TenantIDContextKey).(uint64)\n\tlang, _ := types.LanguageFromContext(ctx)\n\tpayload := types.SummaryGenerationPayload{\n\t\tKnowledgeBaseID: kbID,\n\t\tKnowledgeID:     knowledgeID,\n\t\tLanguage:        lang,\n\t}",
		"\ttenantID := ctx.Value(types.TenantIDContextKey).(uint64)\n\tlang, _ := types.LanguageFromContext(ctx)\n\tpayload := types.SummaryGenerationPayload{\n\t\tTenantID:        tenantID,\n\t\tKnowledgeBaseID: kbID,\n\t\tKnowledgeID:     knowledgeID,\n\t\tLanguage:        lang,\n\t}",
		1)

	// Fix 6: tag.Path -> tag.Name (line 2327)
	content = strings.ReplaceAll(content, "tagPath = tag.Path", "tagPath = tag.Name")

	// Fix 7: CreateKnowledgeFromManual signature change
	content = strings.Replace(content,
		"func (s *knowledgeService) CreateKnowledgeFromManual(ctx context.Context,\n\tkbID string, payload *types.ManualKnowledgePayload, channel string,\n) (*types.Knowledge, error) {\n\tlogger.Info(ctx, \"Start creating manual knowledge entry\")\n\n\tif payload == nil {\n\t\treturn nil, werrors.NewBadRequestError(\"请求内容不能为空\")\n\t}\n\n\tcleanContent := secutils.CleanMarkdown(payload.Content)\n\tif strings.TrimSpace(cleanContent) == \"\" {\n\t\treturn nil, werrors.NewValidationError(\"内容不能为空\")\n\t}\n\tif len([]rune(cleanContent)) > manualContentMaxLength {\n\t\treturn nil, werrors.NewValidationError(fmt.Sprintf(\"内容长度超出限制（最多%d个字符）\", manualContentMaxLength))\n\t}\n\n\tsafeTitle, ok := secutils.ValidateInput(payload.Title)\n\tif !ok {\n\t\treturn nil, werrors.NewValidationError(\"标题包含非法字符或超出长度限制\")\n\t}\n\n\tstatus := strings.ToLower(strings.TrimSpace(payload.Status))\n\tif status == \"\" {\n\t\tstatus = types.ManualKnowledgeStatusDraft\n\t}\n\tif status != types.ManualKnowledgeStatusDraft && status != types.ManualKnowledgeStatusPublish {\n\t\treturn nil, werrors.NewValidationError(\"状态仅支持 draft 或 publish\")\n\t}",
		"func (s *knowledgeService) CreateKnowledgeFromManual(ctx context.Context,\n\tkbID string, title string, content string, tagID string,\n) (*types.Knowledge, error) {\n\tlogger.Info(ctx, \"Start creating manual knowledge entry\")\n\n\tcleanContent := secutils.CleanMarkdown(content)\n\tif strings.TrimSpace(cleanContent) == \"\" {\n\t\treturn nil, werrors.NewValidationError(\"内容不能为空\")\n\t}\n\tif len([]rune(cleanContent)) > manualContentMaxLength {\n\t\treturn nil, werrors.NewValidationError(fmt.Sprintf(\"内容长度超出限制（最多%d个字符）\", manualContentMaxLength))\n\t}\n\n\tsafeTitle, ok := secutils.ValidateInput(title)\n\tif !ok {\n\t\treturn nil, werrors.NewValidationError(\"标题包含非法字符或超出长度限制\")\n\t}\n\n\tstatus := types.ManualKnowledgeStatusPublish",
		1)

	// Fix 8: Remove payload.TagID reference (replaced with tagID param)
	content = strings.Replace(content,
		"\t\tTagID:    payload.TagID, // 设置分类ID，用于知识分类管理",
		"\t\tTagID:    tagID, // 设置分类ID，用于知识分类管理",
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
