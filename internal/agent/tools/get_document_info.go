package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/Tencent/WeKnora/internal/utils"
)

var getDocumentInfoTool = BaseTool{
	name: ToolGetDocumentInfo,
	description: `Retrieve detailed metadata information about documents.

## When to Use

Use this tool when:
- Need to understand document basic information (title, type, size, etc.)
- Check if document exists and is available
- Batch query metadata for multiple documents
- Understand document processing status

Do not use when:
- Need document content (use knowledge_search)
- Need specific text chunks (search results already contain full content)


## Returned Information

- Basic info: title, description, source type
- File info: filename, type, size
- Processing status: whether processed, chunk count
- Metadata: custom tags and properties


## Notes

- Concurrent query for multiple documents provides better performance
- Returns complete document metadata, not just title
- Can check document processing status (parse_status)`,
	schema: utils.GenerateSchema[GetDocumentInfoInput](),
}

// GetDocumentInfoInput defines the input parameters for get document info tool
type GetDocumentInfoInput struct {
	KnowledgeIDs []string `json:"knowledge_ids" jsonschema:"REQUIRED JSON array of document/knowledge ID strings (e.g. [\"abc\",\"def\"]), NOT a single string. Even when querying only one document you MUST wrap the id in an array: [\"abc\"]. Obtain ids from the knowledge_id field returned by search/select tools."`
}

// GetDocumentInfoTool retrieves detailed information about a document/knowledge
type GetDocumentInfoTool struct {
	BaseTool
	knowledgeService interfaces.KnowledgeService
	chunkService     interfaces.ChunkService
	searchTargets    types.SearchTargets // Pre-computed unified search targets with KB-tenant mapping
}

// NewGetDocumentInfoTool creates a new get document info tool
func NewGetDocumentInfoTool(
	knowledgeService interfaces.KnowledgeService,
	chunkService interfaces.ChunkService,
	searchTargets types.SearchTargets,
) *GetDocumentInfoTool {
	return &GetDocumentInfoTool{
		BaseTool:         getDocumentInfoTool,
		knowledgeService: knowledgeService,
		chunkService:     chunkService,
		searchTargets:    searchTargets,
	}
}

// Execute retrieves document information with concurrent processing
func (t *GetDocumentInfoTool) Execute(ctx context.Context, args json.RawMessage) (*types.ToolResult, error) {
	// 先按实际期望的结构解析。如果失败，尝试容错：
	// LLM 偶尔会把 knowledge_ids 当成单个字符串或逗号分隔的字符串传进来，
	// 这里把它规范化成 []string，避免单次 tool call 直接作废。
	var input GetDocumentInfoInput
	if err := json.Unmarshal(args, &input); err != nil {
		normalized, nerr := normalizeGetDocumentInfoArgs(args)
		if nerr != nil {
			return &types.ToolResult{
				Success: false,
				Error:   fmt.Sprintf("Failed to parse args: %v", err),
			}, err
		}
		input = normalized
	}

	// Extract knowledge_ids array
	knowledgeIDs := input.KnowledgeIDs
	if len(knowledgeIDs) == 0 {
		return &types.ToolResult{
			Success: false,
			Error:   "knowledge_ids is required and must be a non-empty array",
		}, fmt.Errorf("knowledge_ids is required")
	}

	// Concurrently get info for each knowledge ID
	type docInfo struct {
		knowledge  *types.Knowledge
		chunkCount int
		err        error
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	results := make(map[string]*docInfo)

	// Concurrently get info for each knowledge ID
	for _, knowledgeID := range knowledgeIDs {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()

			// Get knowledge metadata without tenant filter to support shared KB
			knowledge, err := t.knowledgeService.GetKnowledgeByIDOnly(ctx, id)
			if err != nil {
				mu.Lock()
				results[id] = &docInfo{
					err: fmt.Errorf("failed to get document info: %v", err),
				}
				mu.Unlock()
				return
			}

			// Verify the knowledge's KB is in searchTargets (permission check)
			if !t.searchTargets.ContainsKB(knowledge.KnowledgeBaseID) {
				mu.Lock()
				results[id] = &docInfo{
					err: fmt.Errorf("knowledge base %s is not accessible", knowledge.KnowledgeBaseID),
				}
				mu.Unlock()
				return
			}

			// Get chunk count for this knowledge
			_, total, err := t.chunkService.GetRepository().
				ListPagedChunksByKnowledgeID(ctx, id, &types.Pagination{
					Page:     1,
					PageSize: 1,
				}, []types.ChunkType{"text"}, "", "", "", "", "")
			if err != nil {
				mu.Lock()
				results[id] = &docInfo{
					err: fmt.Errorf("failed to get document info: %v", err),
				}
				mu.Unlock()
				return
			}
			chunkCount := int(total)

			mu.Lock()
			results[id] = &docInfo{
				knowledge:  knowledge,
				chunkCount: chunkCount,
			}
			mu.Unlock()
		}(knowledgeID)
	}

	wg.Wait()

	// Collect successful results and errors
	successDocs := make([]*docInfo, 0)
	var errors []string

	for _, knowledgeID := range knowledgeIDs {
		result := results[knowledgeID]
		if result.err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", knowledgeID, result.err))
		} else if result.knowledge != nil {
			successDocs = append(successDocs, result)
		}
	}

	if len(successDocs) == 0 {
		return &types.ToolResult{
			Success: false,
			Error:   fmt.Sprintf("Failed to retrieve any document info. Errors: %v", errors),
		}, fmt.Errorf("all document retrievals failed")
	}

	// Format output
	output := "=== Document Info ===\n\n"
	output += fmt.Sprintf("Successfully retrieved %d / %d documents\n\n", len(successDocs), len(knowledgeIDs))

	if len(errors) > 0 {
		output += "=== Partial Failures ===\n"
		for _, errMsg := range errors {
			output += fmt.Sprintf("  - %s\n", errMsg)
		}
		output += "\n"
	}

	formattedDocs := make([]map[string]interface{}, 0, len(successDocs))
	for i, doc := range successDocs {
		k := doc.knowledge

		output += fmt.Sprintf("[Document #%d]\n", i+1)
		output += fmt.Sprintf("  ID:           %s\n", k.ID)
		output += fmt.Sprintf("  Title:        %s\n", k.Title)

		if k.Description != "" {
			output += fmt.Sprintf("  Description:  %s\n", k.Description)
		}

		output += fmt.Sprintf("  Type:         %s\n", k.Type)

		if k.FileName != "" {
			output += fmt.Sprintf("  File Name:    %s\n", k.FileName)
			output += fmt.Sprintf("  File Type:    %s\n", k.FileType)
			output += fmt.Sprintf("  File Size:    %s\n", formatFileSize(k.FileSize))
		}

		output += fmt.Sprintf("  Parse Status: %s\n", formatParseStatus(k.ParseStatus))
		output += fmt.Sprintf("  Chunk Count:  %d\n", doc.chunkCount)

		output += "\n"

		formattedDocs = append(formattedDocs, map[string]interface{}{
			"knowledge_id": k.ID,
			"title":        k.Title,
			"description":  k.Description,
			"type":         k.Type,
			"file_name":    k.FileName,
			"file_type":    k.FileType,
			"file_size":    k.FileSize,
			"parse_status": k.ParseStatus,
			"chunk_count":  doc.chunkCount,
		})
	}

	// Extract first document title for summary
	var firstTitle string
	if len(successDocs) > 0 && successDocs[0].knowledge != nil {
		firstTitle = successDocs[0].knowledge.Title
	}

	return &types.ToolResult{
		Success: true,
		Output:  output,
		Data: map[string]interface{}{
			"documents":    formattedDocs,
			"total_docs":   len(successDocs),
			"requested":    len(knowledgeIDs),
			"errors":       errors,
			"display_type": "document_info",
			"title":        firstTitle, // For frontend summary display
		},
	}, nil
}

func formatSource(knowledgeType, source string) string {
	switch knowledgeType {
	case "file":
		return "File Upload"
	case "url":
		return fmt.Sprintf("URL: %s", source)
	case "passage":
		return "Text Input"
	default:
		return knowledgeType
	}
}

func formatFileSize(size int64) string {
	if size == 0 {
		return "Unknown"
	}
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(size)/float64(div), "KMGTPE"[exp])
}

func formatParseStatus(status string) string {
	switch status {
	case "pending":
		return "Pending"
	case "processing":
		return "Processing"
	case "completed", "success":
		return "Completed"
	case "failed":
		return "Failed"
	default:
		return status
	}
}

// normalizeGetDocumentInfoArgs 容错地解析 get_document_info 的入参。
// 主要处理 LLM 把 knowledge_ids 传成单个字符串（或逗号分隔字符串）的情况，
// 把它规范化成 []string。用于严格结构解析失败时的降级通道。
func normalizeGetDocumentInfoArgs(args json.RawMessage) (GetDocumentInfoInput, error) {
	var input GetDocumentInfoInput
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(args, &raw); err != nil {
		return input, err
	}
	idsRaw, ok := raw["knowledge_ids"]
	if !ok {
		return input, fmt.Errorf("knowledge_ids is required")
	}

	// 先试标准数组
	var ids []string
	if err := json.Unmarshal(idsRaw, &ids); err == nil {
		input.KnowledgeIDs = ids
		return input, nil
	}

	// 再试单个字符串
	var single string
	if err := json.Unmarshal(idsRaw, &single); err != nil {
		return input, fmt.Errorf("knowledge_ids must be array of strings or a single string")
	}
	// 允许逗号分隔的字符串，拆成数组。
	parts := strings.Split(single, ",")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			input.KnowledgeIDs = append(input.KnowledgeIDs, p)
		}
	}
	return input, nil
}
