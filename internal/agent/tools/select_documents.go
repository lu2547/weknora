package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// selectDocumentsToolMeta 是 select_documents 工具的共享定义与 schema。
var selectDocumentsToolMeta = BaseTool{
	name: ToolSelectDocuments,
	description: `选文档（文档级筛选）工具。

本工具在全局「知识摘要」集合（weknora_summary）上按多种条件筛选 TopK 篇候选文档，
只返回文档级元信息（knowledge_id + 标题 + 摘要 + 得分），**不返回分片正文**。

## 何时使用本工具
当用户的问题是「在一批文档里定位要看哪几份」时，优先调用本工具拿到候选 knowledge_id，
再传给 knowledge_search / grep_chunks / list_knowledge_chunks 做分片级检索。
典型触发场景（**务必识别并使用**）：
- 按部门/组织/业务线定位：例如「人设部最近3年的报告」「风控部今年的周报」——
  部门名映射到 tag_path_prefixes，时间映射到 created_after / created_before。
- 按时间范围定位：例如「2024 年之后的产品手册」「最近半年的会议纪要」——走时间范围过滤。
- 按文件名/文件类型定位：例如「所有财报 PDF」「包含 Q3 的 xlsx」——用 filename_keywords / file_types。
- 按主题语义定位：例如「跟合规相关的政策文件」——走 query 语义向量。
- 上述条件的任意组合。

## 工作原理
1. 用 query 做稠密向量检索（语义相关性打分）；
2. 在向量检索的同时，叠加标量过滤：知识库、标签、文件名、文件类型、时间范围；
3. 返回 TopK 文档（按得分降序）。

## 参数组合建议
- query：始终提供一句最能描述主题的自然语言（即使用户说的是部门/时间，也要提炼出一个语义短句，
  例如「人设部最近3年的报告」→ query="报告" + tag_ids=[<人设部标签的 id_knowledge_tag>] + 时间范围）；
- knowledge_base_ids / tag_ids：把用户已明确指定的范围下压到过滤器；
tag_ids 是标签的 id_knowledge_tag 列表，传任意层级（祖先或叶子）都能命中该节点及子孙；
- filename_keywords：在文件名里模糊匹配（例如 "Q3"、"财报"、"2024"）。

## 本工具不会做的事
- 不会返回分片正文，不会直接回答用户问题；
- 不会做关键词精确命中（关键词匹配请用 grep_chunks）；
- 不做 file_type / created_at 过滤（如有需要应在 service 层先查 PG 拿 knowledge_ids 后再调用本工具）。

## 输出
按得分降序的候选文档列表，字段含 knowledge_id / 标题 / 摘要 / 得分。`,
	schema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "query": {
      "type": "string",
      "description": "【必填】一句描述要找的文档主题的自然语言查询。即便用户问题偏条件筛选（部门/时间/文件名），也要提炼一个语义短句作为 query。"
    },
    "knowledge_base_ids": {
      "type": "array",
      "description": "可选：将筛选范围限定在这些知识库 ID 内。",
      "items": {"type": "string"},
      "minItems": 0,
      "maxItems": 10
    },
    "tag_ids": {
      "type": "array",
      "description": "可选：标签 id_knowledge_tag 列表。传任意层级（祖先或叶子）都能命中该节点及其子孙（Milvus ARRAY_CONTAINS_ANY）。部门/业务线定位优先用此项。",
      "items": {"type": "string"},
      "minItems": 0,
      "maxItems": 20
    },
    "filename_keywords": {
      "type": "array",
      "description": "可选：文件名模糊包含匹配关键词（例如 ['Q3','财报','2024']）。多个关键词之间取并集（OR）。",
      "items": {"type": "string"},
      "minItems": 0,
      "maxItems": 10
    },
    "top_k": {
      "type": "integer",
      "description": "可选：返回文档数量，默认 5，最大 20。",
      "minimum": 1,
      "maximum": 20
    }
  },
  "required": ["query"]
}`),
}

// SelectDocumentsInput 定义 select_documents 工具的入参。
type SelectDocumentsInput struct {
	Query            string   `json:"query"`
	KnowledgeBaseIDs []string `json:"knowledge_base_ids,omitempty"`
	TagIDs           []string `json:"tag_ids,omitempty"`
	FileNameKeywords []string `json:"filename_keywords,omitempty"`
	TopK             int      `json:"top_k,omitempty"`
}

// SelectDocumentsTool 基于摘要向量相似度 + 标量过滤对知识文档进行筛选。
type SelectDocumentsTool struct {
	BaseTool
	knowledgeBaseService interfaces.KnowledgeBaseService
	searchTargets        types.SearchTargets
	// defaultFilter 作为来自上下文（例如用户在 UI 中预选的标签/时间范围）的
	// 默认过滤条件。只有当入参对应字段为空时才生效，入参始终具有更高优先级。
	defaultFilter types.SummaryFilter
}

// NewSelectDocumentsTool 构造一个新的 select_documents 工具实例。
// defaultFilter 可传零值表示没有上下文预选条件。
func NewSelectDocumentsTool(
	knowledgeBaseService interfaces.KnowledgeBaseService,
	searchTargets types.SearchTargets,
	defaultFilter types.SummaryFilter,
) *SelectDocumentsTool {
	return &SelectDocumentsTool{
		BaseTool:             selectDocumentsToolMeta,
		knowledgeBaseService: knowledgeBaseService,
		searchTargets:        searchTargets,
		defaultFilter:        defaultFilter,
	}
}

// Execute 执行 select_documents 工具。
func (t *SelectDocumentsTool) Execute(ctx context.Context, args json.RawMessage) (*types.ToolResult, error) {
	logger.Infof(ctx, "[Tool][SelectDocuments] 开始执行")

	var input SelectDocumentsInput
	if err := json.Unmarshal(args, &input); err != nil {
		logger.Errorf(ctx, "[Tool][SelectDocuments] 参数解析失败: %v", err)
		return &types.ToolResult{
			Success: false,
			Error:   fmt.Sprintf("参数解析失败: %v", err),
		}, err
	}

	query := strings.TrimSpace(input.Query)
	if query == "" {
		return &types.ToolResult{
			Success: false,
			Error:   "query 参数必填",
		}, fmt.Errorf("empty query")
	}

	// 确定本次要检索的知识库集合。
	searchTargets := t.searchTargets
	if len(input.KnowledgeBaseIDs) > 0 {
		want := make(map[string]bool, len(input.KnowledgeBaseIDs))
		for _, id := range input.KnowledgeBaseIDs {
			want[id] = true
		}
		var filtered types.SearchTargets
		for _, st := range t.searchTargets {
			if want[st.KnowledgeBaseID] {
				filtered = append(filtered, st)
			}
		}
		searchTargets = filtered
	}
	if len(searchTargets) == 0 {
		return &types.ToolResult{
			Success: false,
			Error:   "没有可用的知识库用于 select_documents",
		}, fmt.Errorf("no search targets")
	}

	// 从检索目标中抽取生效的 KB ID 集合（此处忽略知识级目标——
	// select_documents 工作在文档粒度）。
	effectiveKBIDs := searchTargets.GetAllKnowledgeBaseIDs()
	if len(effectiveKBIDs) == 0 {
		return &types.ToolResult{
			Success: false,
			Error:   "没有可用的知识库用于 select_documents",
		}, fmt.Errorf("no kb ids")
	}

	topK := input.TopK
	if topK <= 0 {
		topK = 5
	}
	if topK > 20 {
		topK = 20
	}

	filter := types.SummaryFilter{
		KnowledgeBaseIDs: effectiveKBIDs,
		TagIDs:           firstNonEmpty(input.TagIDs, t.defaultFilter.TagIDs),
		FileNameKeywords: firstNonEmpty(input.FileNameKeywords, t.defaultFilter.FileNameKeywords),
	}

	// 用第一个 KB 作为 embedding 参考模型。检索范围内的所有 KB
	// 预期共享同一个 embedding 模型（单租户假设）。
	referenceKBID := effectiveKBIDs[0]

	hits, err := t.knowledgeBaseService.SearchKnowledgeSummaries(ctx, referenceKBID, query, topK, filter)
	if err != nil {
		logger.Warnf(ctx, "[Tool][SelectDocuments] 摘要检索失败: %v", err)
		return &types.ToolResult{
			Success: false,
			Error:   fmt.Sprintf("摘要检索失败: %v", err),
		}, err
	}

	// 按得分降序排序（SearchSummaries 可能已经排过，此处再保证一次以得到确定性输出）。
	sort.Slice(hits, func(i, j int) bool {
		return hits[i].Score > hits[j].Score
	})

	return t.formatOutput(ctx, query, effectiveKBIDs, hits)
}

// formatOutput 把 TopK 的命中结果渲染为工具返回值。
func (t *SelectDocumentsTool) formatOutput(
	ctx context.Context,
	query string,
	kbIDs []string,
	hits []*types.SummaryHit,
) (*types.ToolResult, error) {
	if len(hits) == 0 {
		output := fmt.Sprintf(
			"在 %d 个知识库中未找到与查询 %q 相关的文档。\n\n"+
				"可以尝试放宽 query 表达，或移除标签过滤条件。",
			len(kbIDs), query,
		)
		return &types.ToolResult{
			Success: true,
			Output:  output,
			Data: map[string]interface{}{
				"query":              query,
				"knowledge_base_ids": kbIDs,
				"results":            []interface{}{},
				"count":              0,
				"display_type":       "document_shortlist",
			},
		}, nil
	}

	var b strings.Builder
	b.WriteString("=== 文档候选清单 ===\n")
	b.WriteString(fmt.Sprintf("查询：%s\n", query))
	b.WriteString(fmt.Sprintf("共找到 %d 篇候选文档\n\n", len(hits)))

	formatted := make([]map[string]interface{}, 0, len(hits))
	for i, h := range hits {
		title := h.FileName
		if title == "" {
			title = h.KnowledgeID
		}

		b.WriteString(fmt.Sprintf("[%d] %s\n", i+1, title))
		b.WriteString(fmt.Sprintf("    knowledge_id：%s\n", h.KnowledgeID))
		if len(h.TagIDs) > 0 {
			b.WriteString(fmt.Sprintf("    tag_ids：%s\n", strings.Join(h.TagIDs, ",")))
		}
		b.WriteString(fmt.Sprintf("    得分：%.4f\n", h.Score))
		if h.Content != "" {
			b.WriteString(fmt.Sprintf("    摘要：%s\n", truncateForDisplay(h.Content, 400)))
		}
		b.WriteString("\n")

		formatted = append(formatted, map[string]interface{}{
			"rank":              i + 1,
			"knowledge_id":      h.KnowledgeID,
			"knowledge_base_id": h.KnowledgeBaseID,
			"title":             title,
			"file_name":         h.FileName,
			"tag_ids":           h.TagIDs,
			"summary":           h.Content,
			"score":             h.Score,
		})
	}

	b.WriteString("=== 后续步骤 ===\n")
	b.WriteString("- 将选中的 knowledge_id 传给 knowledge_search / grep_chunks / list_knowledge_chunks，用于下钻到分片。\n")
	b.WriteString("- 本工具本身不会直接回答用户的问题。\n")

	knowledgeIDs := make([]string, 0, len(hits))
	for _, h := range hits {
		knowledgeIDs = append(knowledgeIDs, h.KnowledgeID)
	}

	return &types.ToolResult{
		Success: true,
		Output:  b.String(),
		Data: map[string]interface{}{
			"query":              query,
			"knowledge_base_ids": kbIDs,
			"results":            formatted,
			"count":              len(formatted),
			"knowledge_ids":      knowledgeIDs,
			"display_type":       "document_shortlist",
		},
	}, nil
}

// truncateForDisplay 将字符串按 rune 截断到最多 maxRunes 个，超出则在末尾追加省略号。
// 基于 rune 截断以避免破坏 UTF-8 多字节字符。
func truncateForDisplay(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes]) + "..."
}

// firstNonEmpty 返回第一个非空字符串切片。用于在入参为空时回落到 defaultFilter 对应字段。
func firstNonEmpty(primary, fallback []string) []string {
	if len(primary) > 0 {
		return primary
	}
	return fallback
}
