# WeKnora Agent 能力详解

## 1. 概述

WeKnora 的 Agent 能力构建在经典的 **ReAct（Reason + Act）** 范式之上，通过 **事件总线（EventBus）驱动的流式执行引擎** 实现了多轮推理-工具调用循环。系统支持两种核心模式：

| 模式                | 标识符            | 说明                                   |
| ------------------- | ----------------- | -------------------------------------- |
| **Quick Answer**    | `quick-answer`    | 传统 RAG 问答，ChatPipeline 插件链处理 |
| **Smart Reasoning** | `smart-reasoning` | ReAct Agent，LLM 自主决策工具使用顺序  |

---

## 2. 数据库表结构

### 2.1 custom_agents 表（000006_custom_agents.up.sql）

```sql
CREATE TABLE IF NOT EXISTS custom_agents (
    id              VARCHAR(64)  NOT NULL,   -- 智能体唯一ID（builtin-* 或 UUID）
    tenant_id       INTEGER      NOT NULL,   -- 多租户隔离
    name            VARCHAR(255) NOT NULL,   -- 显示名称
    description     TEXT,
    avatar          VARCHAR(255),            -- 头像URL
    is_builtin      BOOLEAN      DEFAULT FALSE,
    created_by      VARCHAR(255),
    config          JSONB        NOT NULL DEFAULT '{}', -- 全量配置（CustomAgentConfig）
    created_at      TIMESTAMP    NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMP    NOT NULL DEFAULT NOW(),
    PRIMARY KEY (id, tenant_id)
);
```

**JSONB config 字段字段清单**（对应 `CustomAgentConfig`）：

| 字段                              | 类型     | 说明                                       |
| --------------------------------- | -------- | ------------------------------------------ |
| `agent_mode`                      | string   | `quick-answer` / `smart-reasoning`         |
| `system_prompt`                   | string   | 自定义系统提示词（支持 `{{占位符}}` 语法） |
| `context_template`                | string   | RAG 上下文模板（普通模式）                 |
| `model_id`                        | string   | 主模型 ID                                  |
| `rerank_model_id`                 | string   | 重排模型 ID                                |
| `temperature`                     | float    | 温度（默认 0.7）                           |
| `max_completion_tokens`           | int      | 最大生成 Token 数                          |
| `max_iterations`                  | int      | Agent 最大迭代次数（默认 20）              |
| `allowed_tools`                   | []string | 白名单工具列表                             |
| `reflection_enabled`              | bool     | 是否开启工具调用后反思                     |
| `mcp_selection_mode`              | string   | `all` / `selected` / `none`                |
| `mcp_services`                    | []string | 选定的 MCP 服务 ID 列表                    |
| `skills_selection_mode`           | string   | `all` / `selected` / `none`                |
| `selected_skills`                 | []string | 选定的 Skill 名称列表                      |
| `kb_selection_mode`               | string   | `all` / `selected` / `none`                |
| `knowledge_bases`                 | []string | 绑定知识库 UUID 列表                       |
| `retrieve_kb_only_when_mentioned` | bool     | 仅 @ 提及时检索知识库                      |
| `web_search_enabled`              | bool     | 网络搜索开关                               |
| `web_search_max_results`          | int      | 网络搜索最大结果数                         |
| `multi_turn_enabled`              | bool     | 多轮对话开关                               |
| `history_turns`                   | int      | 保留历史轮数                               |
| `embedding_top_k`                 | int      | 向量召回 TopK                              |
| `rerank_top_k`                    | int      | 重排 TopK                                  |
| `enable_query_expansion`          | bool     | 查询扩展开关                               |
| `enable_rewrite`                  | bool     | 查询改写开关                               |
| `fallback_strategy`               | string   | `fixed` / `model` 兜底策略                 |

### 2.2 内置智能体

系统预置三个内置 Agent（`is_builtin=true`）：

| ID                        | 名称       | 模式              | 说明                      |
| ------------------------- | ---------- | ----------------- | ------------------------- |
| `builtin-quick-answer`    | 快速回答   | `quick-answer`    | 基础 RAG 问答，简洁高效   |
| `builtin-smart-reasoning` | 深度推理   | `smart-reasoning` | ReAct Agent，复杂多步推理 |
| `builtin-data-analyst`    | 数据分析师 | `smart-reasoning` | 特化于数据处理和分析      |

---

## 3. 后端 Agent 架构

### 3.1 AgentEngine 核心结构体

```go
// internal/agent/engine.go
type AgentEngine struct {
    chatModel         chat.Chat             // LLM 接口（支持流式）
    toolRegistry      *tools.Registry       // 工具注册表
    config            *types.AgentConfig    // Agent 配置
    eventBus          *event.EventBus       // 事件总线
    sessionID         string                // 会话 ID
    contextManager    ContextManager        // 多轮上下文管理
    knowledgeBasesInfo []*KnowledgeBaseInfo // 绑定的知识库信息
    selectedDocs      []*SelectedDocumentInfo // @ 提及的文档
    systemPromptTemplate string             // 自定义系统提示词
}
```

### 3.2 AgentConfig 配置结构

```go
// internal/types/agent.go
type AgentConfig struct {
    MaxIterations    int     `json:"max_iterations"`    // 默认 20
    Temperature      float32 `json:"temperature"`       // 默认 0.7
    Thinking         bool    `json:"thinking"`          // 是否启用思考模式
    ReflectionEnabled bool   `json:"reflection_enabled"` // 工具调用后反思
    WebSearchEnabled  bool   `json:"web_search_enabled"`
    AllowedTools      []string `json:"allowed_tools"`
}
```

### 3.3 AgentState 运行时状态

```go
type AgentState struct {
    Query         string          // 用户查询
    CurrentRound  int             // 当前迭代轮次
    RoundSteps    []AgentStep     // 每轮的步骤记录
    FinalAnswer   string          // 最终答案
    IsComplete    bool            // 是否完成
    KnowledgeRefs []KnowledgeRef  // 引用的知识库片段
}

type AgentStep struct {
    Iteration int
    Thought   string        // LLM 思考内容
    ToolCalls []ToolCall    // 本轮工具调用列表
}

type ToolCall struct {
    ID         string            // 工具调用 ID
    Name       string            // 工具名称
    Args       map[string]interface{} // 工具参数
    Result     ToolResult        // 工具返回结果
    Reflection string            // 反思内容（可选）
}
```

---

## 4. ReAct 主循环详解

### 4.1 executeLoop 完整流程

```
┌─────────────────────────────────────────────────────────────┐
│                   executeLoop                                │
│  for state.CurrentRound < MaxIterations (默认 20)            │
│                                                             │
│  ┌──────────────────────────────────────────────────────┐  │
│  │  Phase 1: THINK（流式 LLM 推理）                       │  │
│  │  streamThinkingToEventBus(messages, tools, iteration) │  │
│  │  → 流式输出 EventAgentThought 事件                     │  │
│  │  → 若有 thinking_tool → EventAgentThought (source=    │  │
│  │    thinking_tool)                                      │  │
│  │  → 若有 final_answer tool → EventAgentFinalAnswer     │  │
│  └──────────────────────────────────────────────────────┘  │
│                          ↓                                   │
│  ┌──────────────────────────────────────────────────────┐  │
│  │  Phase 2: CHECK COMPLETION                            │  │
│  │  • FinishReason == "stop" && ToolCalls == nil         │  │
│  │    → state.IsComplete = true; break                   │  │
│  │  • ToolCall.Name == "final_answer"                    │  │
│  │    → state.FinalAnswer = answer; break                │  │
│  └──────────────────────────────────────────────────────┘  │
│                          ↓                                   │
│  ┌──────────────────────────────────────────────────────┐  │
│  │  Phase 3: ACT（并发工具执行）                           │  │
│  │  for each tc in response.ToolCalls:                   │  │
│  │    result = toolRegistry.ExecuteTool(ctx, name, args) │  │
│  │    → 发出 EventAgentToolCall 事件（pending）            │  │
│  │    → 发出 EventAgentToolResult 事件（含结构化数据）      │  │
│  │    → 若 reflection_enabled:                           │  │
│  │        streamReflectionToEventBus → reflection 字符串  │  │
│  └──────────────────────────────────────────────────────┘  │
│                          ↓                                   │
│  ┌──────────────────────────────────────────────────────┐  │
│  │  Phase 4: OBSERVE                                     │  │
│  │  appendToolResults(messages, step)                    │  │
│  │  → 追加 role=assistant (含tool_calls) 消息             │  │
│  │  → 追加 role=tool 结果消息                              │  │
│  │  → 写入 contextManager 持久化                           │  │
│  └──────────────────────────────────────────────────────┘  │
│                          ↓                                   │
│       state.CurrentRound++; 继续下一轮                       │
└─────────────────────────────────────────────────────────────┘
          ↓ (达到 MaxIterations 仍未完成)
   streamFinalAnswerToEventBus → 强制生成最终答案
```

### 4.2 streamThinkingToEventBus 细节

```go
func (e *AgentEngine) streamThinkingToEventBus(...) {
    opts := &chat.ChatOptions{
        Temperature: e.config.Temperature,  // 通常 0.7
        Tools:       tools,                 // 所有注册工具的 function 定义
        Thinking:    e.config.Thinking,     // 是否开启原生思考模式（DeepSeek R1等）
    }

    // 事件路由逻辑
    func(chunk *types.StreamResponse, fullContent string) {
        switch chunk.ResponseType {
        case ResponseTypeToolCall:
            // 新工具调用 → EventAgentToolCall (pending=true)
        case ResponseTypeAnswer:
            // final_answer 工具内容流 → EventAgentFinalAnswer
        case ResponseTypeThinking:
            // thinking 工具内容流 → EventAgentThought (source=thinking_tool)
        default:
            // 普通文本 → EventAgentThought
        }
    }
}
```

### 4.3 消息构建（buildMessagesWithLLMContext）

```go
messages := []chat.Message{
    {Role: "system", Content: systemPrompt},  // 系统提示词（含 KB 列表、技能列表等）
    // 历史多轮上下文（排除 system 消息）
    {Role: "user", Content: msg1},
    {Role: "assistant", Content: resp1, ToolCalls: [...]},
    {Role: "tool", Content: toolResult, ToolCallID: id},
    // ...
    {Role: "user", Content: currentQuery},  // 当前轮查询
}
```

---

## 5. 工具系统详解

### 5.1 工具注册表（tools/registry.go）

```go
type Registry struct {
    tools   map[string]Tool
    nameSet map[string]bool  // first-wins 防止注入攻击
}

// 注册工具（first-wins 策略）
func (r *Registry) Register(tool Tool) {
    if r.nameSet[tool.GetName()] {
        return  // 已存在则忽略，防止同名工具覆盖攻击
    }
    r.tools[tool.GetName()] = tool
    r.nameSet[tool.GetName()] = true
}

// 执行工具
func (r *Registry) ExecuteTool(ctx context.Context, name string, args map[string]interface{}) (*ToolResult, error) {
    tool, exists := r.tools[name]
    if !exists {
        return &ToolResult{
            Success: false,
            Error:   fmt.Sprintf("tool %s not found", name),
        }, nil  // 返回 nil error 避免终止循环
    }
    return tool.Execute(ctx, args)
}
```

### 5.2 内置工具清单（tools/definitions.go）

| 工具名                    | 功能                       | 前端显示名   |
| ------------------------- | -------------------------- | ------------ |
| `thinking`                | 链式思考，记录思维过程     | 思考         |
| `todo_write`              | 创建/更新任务计划列表      | 更新计划     |
| `knowledge_search`        | 语义向量搜索知识库         | 搜索知识库   |
| `grep_chunks`             | 关键词精确匹配知识库切片   | 关键词检索   |
| `list_knowledge_chunks`   | 按 ID 批量获取切片完整内容 | 读取文档内容 |
| `get_document_info`       | 获取文档元信息             | 获取文档信息 |
| `get_related_documents`   | 获取相关文档列表           | 关联文档     |
| `get_document_content`    | 获取文档完整内容           | 文档全文     |
| `web_search`              | 网络搜索（需启用）         | 网络搜索     |
| `web_fetch`               | 抓取网页内容               | 获取网页     |
| `query_knowledge_graph`   | 图谱关系查询               | 知识图谱     |
| `knowledge_graph_extract` | 提取知识图谱               | 图谱提取     |
| `final_answer`            | 提交最终答案（MANDATORY）  | （内部工具） |
| `execute_skill`           | 执行 Skill（动态注册）     | 执行技能     |

### 5.3 工具执行流程

```
LLM 返回 ToolCall
    ↓
toolRegistry.ExecuteTool(ctx, name, args)
    ↓
[名称存在?]
  YES → tool.Execute(ctx, args)
          ↓
        返回 ToolResult{Output, Data, Success, Error}
  NO  → ToolResult{Success: false, Error: "tool not found"}
    ↓
EventAgentToolResult 事件（含 structured Data 供前端渲染）
    ↓
[ReflectionEnabled?]
  YES → streamReflectionToEventBus
          → LLM 评估工具结果，提示下一步
          → EventAgentReflection 事件
    ↓
appendToolResults → 写入对话上下文
```

---

## 6. 系统提示词工程

### 6.1 双模板体系（prompts.go）

**ProgressiveRAGSystemPrompt**（有知识库时使用）：
```
### Role
You are WeKnora, an intelligent retrieval assistant powered by Progressive Agentic RAG.

### Critical Constraints (ABSOLUTE RULES)
1. NO Internal Knowledge
2. Mandatory Deep Read: knowledge_search → list_knowledge_chunks 强制二步检索
3. KB First, Web Second
4. Strict Plan Adherence

### Workflow: The "Reconnaissance-Plan-Execute" Cycle
Phase 1: Preliminary Reconnaissance
Phase 2: Strategic Decision & Planning
Phase 3: Disciplined Execution & Deep Reflection
Phase 4: Final Synthesis

### Core Retrieval Strategy
1. grep_chunks (关键词)
2. knowledge_search (语义)
3. list_knowledge_chunks (MANDATORY 深度读取)
4. query_knowledge_graph (可选图谱)
5. web_search (KB 不足时的兜底)

### Final Output Standards
- 引用格式: <kb doc="..." chunk_id="..." /> 或 <web url="..." title="..." />
- 引用位置: 紧邻事实声明（Proximate Citation Rule）
- 图片渲染: ![description](image_url)

### System Status
Current Time: {{current_time}}
Web Search: {{web_search_status}}

### User Selected Knowledge Bases (via @ mention)
{{knowledge_bases}}
```

**PureAgentSystemPrompt**（纯 Agent 模式，无知识库）：
```
### Role
You are WeKnora, an intelligent assistant powered by ReAct.
Pure Agent mode without attached Knowledge Bases.

### Workflow
1. Analyze → Plan（todo_write）→ Execute → Synthesize（final_answer）

### Tool Guidelines
- web_search / web_fetch: 互联网信息获取
- todo_write: 多步任务管理
- thinking: 规划与反思
- final_answer: MANDATORY 最终动作
```

### 6.2 占位符替换机制

```go
// renderPromptPlaceholdersWithStatus
func renderPromptPlaceholdersWithStatus(
    template string,
    kbs []*KnowledgeBaseInfo,
    webSearchEnabled bool,
    currentTime string,
) string {
    // 替换 {{knowledge_bases}}
    kbContent := formatKnowledgeBaseList(kbs)
    result := strings.ReplaceAll(template, "{{knowledge_bases}}", kbContent)

    // 替换 {{web_search_status}}
    webStatus := "Disabled"
    if webSearchEnabled { webStatus = "Enabled" }
    result = strings.ReplaceAll(result, "{{web_search_status}}", webStatus)

    // 替换 {{current_time}}
    result = strings.ReplaceAll(result, "{{current_time}}", currentTime)

    // 替换 {{skills}}（Level 1 元数据）
    // 由 BuildSystemPromptWithOptions 追加 formatSkillsMetadata
    return result
}
```

### 6.3 BuildSystemPromptWithOptions 完整逻辑

```go
func BuildSystemPromptWithOptions(
    kbs []*KnowledgeBaseInfo,
    webSearchEnabled bool,
    selectedDocs []*SelectedDocumentInfo,
    options *BuildSystemPromptOptions,  // 含 SkillsMetadata
    systemPromptTemplate ...string,
) string {
    // 1. 选择模板（自定义 > PureAgent > ProgressiveRAG）
    template := customOrDefault(kbs)

    // 2. 渲染占位符
    basePrompt := renderPromptPlaceholdersWithStatus(template, kbs, webEnabled, time.Now())

    // 3. 追加 @ 提及文档上下文
    if len(selectedDocs) > 0 {
        basePrompt += formatSelectedDocuments(selectedDocs)
    }

    // 4. 追加 Skills 元数据（Level 1 Progressive Disclosure）
    if options != nil && len(options.SkillsMetadata) > 0 {
        basePrompt += formatSkillsMetadata(options.SkillsMetadata)
    }
    return basePrompt
}
```

### 6.4 知识库列表格式化

系统提示中的 `{{knowledge_bases}}` 展开为 Markdown 表格：

```markdown
## Available Knowledge Bases

| ID        | Name     | Description  | Type     |
| --------- | -------- | ------------ | -------- |
| kb-uuid-1 | 产品手册 | 产品使用文档 | Document |
| kb-uuid-2 | FAQ 库   | 常见问题解答 | FAQ      |
```

---

## 7. 事件驱动流式架构

### 7.1 事件类型完整清单

| 事件类型             | Go 常量                 | 说明                                |
| -------------------- | ----------------------- | ----------------------------------- |
| `agent_thought`      | `EventAgentThought`     | LLM 思考流（含 thinking tool 内容） |
| `agent_tool_call`    | `EventAgentToolCall`    | 工具调用开始（pending 状态）        |
| `agent_tool_result`  | `EventAgentToolResult`  | 工具执行完成（含结构化数据）        |
| `agent_tool`         | `EventAgentTool`        | 工具执行监控（内部）                |
| `agent_reflection`   | `EventAgentReflection`  | 反思内容流                          |
| `agent_final_answer` | `EventAgentFinalAnswer` | 最终答案流（final_answer 工具触发） |
| `agent_complete`     | `EventAgentComplete`    | Agent 执行完成（含所有步骤）        |

### 7.2 AgentCompleteData 结构

```go
type AgentCompleteData struct {
    FinalAnswer     string        // 最终答案文本
    KnowledgeRefs   []interface{} // 引用的知识块
    AgentSteps      []AgentStep   // 全部执行步骤（含 ToolCalls）
    TotalSteps      int
    TotalDurationMs int64         // 后端权威执行时长（毫秒）
    MessageID       string        // 消息 ID（用于消息更新）
}
```

### 7.3 前端 SSE 事件消费

前端通过 SSE 接收事件流，每个事件 JSON 结构：

```json
{
  "id": "thinking-abc123",
  "type": "agent_thought",
  "session_id": "session-xyz",
  "data": {
    "content": "Let me analyze the user's question...",
    "iteration": 0,
    "done": false
  }
}
```

---

## 8. 前端实现详解

### 8.1 AgentStreamDisplay 组件架构

**文件**：`frontend/src/views/chat/components/AgentStreamDisplay.vue`（3045 行）

核心职责：
- 消费 `session.agentEventStream` 事件流
- 将事件分类渲染为可折叠的执行步骤树
- 内联引用（`<kb />`、`<web />`）解析为交互式元素
- Mermaid 图表渲染
- 图片预览

### 8.2 事件流处理管道

```
agentEventStream（响应式数组）
    ↓
buildFullEventList(stream)
  ├── 过滤无效事件
  ├── plan_task_change 注入（todo_write 任务变更时）
  └── 连续 thinking 事件合并（避免碎片化显示）
    ↓
intermediateEvents（中间步骤：思考 + 工具调用）
    ↓
displayEvents（显示事件：答案 or 最终思考）
```

### 8.3 折叠树状 UI 逻辑

```
答案开始流式输出时 → shouldShowCollapsedSteps = true
    ↓
    ┌─────────────────────────────────────────┐
    │  📝 已完成 N 个步骤 · 耗时 X.Xs  [▼]   │  ← tree-root（可点击展开）
    └─────────────────────────────────────────┘
    [展开后] tree-children:
    ├─ plan_task_change: 任务 "查询产品规格"
    ├─ thinking: 分析用户需求... [可展开]
    ├─ tool_call: 🔍 搜索知识库 → 找到 5 个结果
    ├─ tool_call: 📖 读取文档内容 → chunk-xxx
    └─ tool_call: ✅ 提交最终答案

    [答案区域]（始终可见）
    └─ 流式渲染 Markdown 答案
```

**思考事件自动展开策略**：
- 流式推理中：当前 thinking 事件自动展开，并自动滚动到底部
- 后续非 thinking 事件到来时：自动折叠已完成的思考

### 8.4 工具名称国际化映射

```typescript
const TOOL_NAME_KEYS: Record<string, string> = {
  search_knowledge:     'agentStream.tools.searchKnowledge',
  knowledge_search:     'agentStream.tools.searchKnowledge',
  grep_chunks:          'agentStream.tools.grepChunks',
  web_search:           'agentStream.tools.webSearch',
  web_fetch:            'agentStream.tools.webFetch',
  get_document_info:    'agentStream.tools.getDocumentInfo',
  list_knowledge_chunks:'agentStream.tools.listKnowledgeChunks',
  get_related_documents:'agentStream.tools.getRelatedDocuments',
  get_document_content: 'agentStream.tools.getDocumentContent',
  todo_write:           'agentStream.tools.todoWrite',
  knowledge_graph_extract: 'agentStream.tools.knowledgeGraphExtract',
  thinking:             'agentStream.tools.thinking',
};
```

### 8.5 内联引用渲染

Agent 输出中的 `<kb doc="..." chunk_id="..." />` 和 `<web url="..." />` 标签由 `preprocessMarkdown` 函数转换：

```typescript
// KB 引用 → 带 hover 浮层的内联链接
<kb doc="产品说明书" chunk_id="abc-123" kb_id="kb-xyz" />
→
<span class="citation-kb" data-chunk-id="abc-123" data-kb-id="kb-xyz">
  📚 产品说明书
  <span class="citation-tip"><!-- hover 时异步加载 chunk 内容 --></span>
</span>

// Web 引用 → 外链
<web url="https://example.com" title="示例页面" />
→
<a class="citation-web" href="https://example.com">
  🌐 example.com
</a>
```

**KB 引用 hover 流程**：
1. 鼠标 hover → 80ms 防抖定时器
2. 读取 `data-chunk-id`
3. 调用 `getChunkByIdOnly(chunkId)` API
4. 显示浮层（含文档名、所属知识库、标签、内容片段）
5. 点击 → 导航至 `/platform/knowledge-bases/{kbId}`

### 8.6 ToolResultRenderer 结构化结果渲染

**文件**：`frontend/src/views/chat/components/ToolResultRenderer.vue`

根据 `display_type` 字段选择不同渲染组件：

```
tool_result.display_type → 对应渲染组件
├── "knowledge_search"   → KnowledgeSearchResult.vue（卡片列表）
├── "web_search"         → WebSearchResult.vue（链接列表）
├── "grep_chunks"        → GrepChunksResult.vue（高亮代码块）
├── "list_chunks"        → ListChunksResult.vue（切片内容）
├── "document_info"      → DocumentInfoResult.vue
├── "todo_write"         → TodoWriteResult.vue（任务状态卡片）
├── "knowledge_graph"    → KnowledgeGraphResult.vue
└── (无)                → 原始文本输出（fallback）
```

---

## 9. Agent 配置管理（前端）

### 9.1 AgentEditorModal 核心功能

**文件**：`frontend/src/views/agent/AgentEditorModal.vue`（131.4KB）

Tab 结构：
1. **基础设置**：名称、描述、头像、运行模式选择
2. **模型设置**：主模型、温度、最大 Token、重排模型
3. **知识库**：选择模式（全部/指定/不用）、知识库列表、仅提及时检索
4. **工具与 MCP**：允许工具列表、MCP 服务选择
5. **Skills**：技能选择模式和列表
6. **Agent 参数**：最大迭代次数、反思开关、思考模式
7. **网络搜索**：开关、最大结果数
8. **高级配置**：系统提示词（支持占位符插入）、上下文模板、兜底策略、改写提示词

### 9.2 CustomAgentConfig TypeScript 接口

```typescript
// frontend/src/api/agent/index.ts
export interface CustomAgentConfig {
  agent_mode?: 'quick-answer' | 'smart-reasoning';
  system_prompt?: string;
  context_template?: string;
  model_id?: string;
  rerank_model_id?: string;
  temperature?: number;
  max_completion_tokens?: number;
  max_iterations?: number;           // Agent 最大迭代次数
  allowed_tools?: string[];
  reflection_enabled?: boolean;
  mcp_selection_mode?: 'all' | 'selected' | 'none';
  mcp_services?: string[];
  skills_selection_mode?: 'all' | 'selected' | 'none';
  selected_skills?: string[];
  kb_selection_mode?: 'all' | 'selected' | 'none';
  knowledge_bases?: string[];
  retrieve_kb_only_when_mentioned?: boolean;
  supported_file_types?: string[];   // 限制上传文件类型
  web_search_enabled?: boolean;
  web_search_max_results?: number;
  multi_turn_enabled?: boolean;
  history_turns?: number;
  embedding_top_k?: number;
  keyword_threshold?: number;
  vector_threshold?: number;
  rerank_top_k?: number;
  rerank_threshold?: number;
  enable_query_expansion?: boolean;
  enable_rewrite?: boolean;
  rewrite_prompt_system?: string;
  rewrite_prompt_user?: string;
  fallback_strategy?: 'fixed' | 'model';
  fallback_response?: string;
  fallback_prompt?: string;
}
```

### 9.3 Agent API 端点

| 方法   | 路径                          | 说明                          |
| ------ | ----------------------------- | ----------------------------- |
| GET    | `/api/v1/agents`              | 获取全部 Agent 列表（含内置） |
| GET    | `/api/v1/agents/:id`          | 获取单个 Agent 详情           |
| POST   | `/api/v1/agents`              | 创建 Agent                    |
| PUT    | `/api/v1/agents/:id`          | 更新 Agent                    |
| DELETE | `/api/v1/agents/:id`          | 删除 Agent                    |
| POST   | `/api/v1/agents/:id/copy`     | 复制 Agent                    |
| GET    | `/api/v1/agents/placeholders` | 获取占位符定义                |

---

## 10. 占位符系统

### 10.1 支持的占位符（PlaceholdersResponse）

```typescript
interface PlaceholdersResponse {
  all: PlaceholderDefinition[];
  system_prompt: PlaceholderDefinition[];          // 系统提示词中可用
  agent_system_prompt: PlaceholderDefinition[];    // Agent 系统提示词中可用
  context_template: PlaceholderDefinition[];       // 上下文模板中可用
  rewrite_system_prompt: PlaceholderDefinition[];  // 改写系统提示词中可用
  rewrite_prompt: PlaceholderDefinition[];         // 改写用户提示词中可用
  fallback_prompt: PlaceholderDefinition[];        // 兜底提示词中可用
}
```

| 占位符                  | 说明                             | 适用场景                           |
| ----------------------- | -------------------------------- | ---------------------------------- |
| `{{knowledge_bases}}`   | 注入知识库列表 Markdown 表格     | agent_system_prompt                |
| `{{web_search_status}}` | 网络搜索状态（Enabled/Disabled） | system_prompt, agent_system_prompt |
| `{{current_time}}`      | 当前时间（RFC3339 格式）         | 所有                               |
| `{{skills}}`            | 注入 Skills 元数据（Level 1）    | agent_system_prompt                |
| `{{context}}`           | RAG 检索到的上下文切片           | context_template                   |
| `{{query}}`             | 用户查询                         | rewrite_prompt                     |

---

## 11. Reflection（反思）机制

当 `reflection_enabled = true` 时，每次工具调用后触发反思：

```go
// streamReflectionToEventBus
reflectionPrompt := fmt.Sprintf(`
Evaluate the result of calling tool %s and decide the next action.
Tool returned: %s
Think:
1. Does the result satisfy the requirement?
2. What should be done next?
`, toolName, result)

messages := []chat.Message{{Role: "user", Content: reflectionPrompt}}
// 调用 LLM（Temperature=0.5）生成反思
// 流式输出 EventAgentReflection 事件
// 将 reflection 内容存储在 step.ToolCalls[last].Reflection
```

反思内容影响：
- 在 `AgentStep.ToolCalls[n].Reflection` 中记录
- 通过 `EventAgentComplete` 随步骤数据一起传输给前端
- 前端可在步骤详情中展示反思内容

---

## 12. 多轮对话上下文管理

### 12.1 ContextManager 接口

```go
type ContextManager interface {
    GetContext(ctx context.Context, sessionID string) ([]chat.Message, error)
    AddMessage(ctx context.Context, sessionID string, msg chat.Message) error
    ClearContext(ctx context.Context, sessionID string) error
}
```

### 12.2 上下文构建流程

```
GetContext(sessionID) → 历史消息列表（基于 history_turns 截断）
    ↓
buildMessagesWithLLMContext(systemPrompt, query, history)
    ↓
[system] → [history...] → [current_query]
    ↓
每轮 appendToolResults 追加:
    [assistant (tool_calls)] → [tool (result)] → ...
    ↓
同时 AddMessage 写入持久化存储（避免重建上下文时丢失）
```

---

## 13. 达到最大迭代次数处理

当循环结束但 `!state.IsComplete` 时：

```go
// streamFinalAnswerToEventBus 强制生成答案
finalPrompt := `Based on the above tool call results, generate a complete answer...
Requirements:
1. Answer based on the actually retrieved content
2. Clearly cite information sources (chunk_id, document name)
3. Organize the answer in a structured format
4. If information is insufficient, honestly state so
5. IMPORTANT: Respond in the same language as the user's question`

// 构建包含所有工具结果的上下文
for step := range state.RoundSteps {
    messages = append(messages, Message{
        Role:    "user",
        Content: fmt.Sprintf("Tool %s returned: %s", toolCall.Name, toolCall.Result.Output),
    })
}
// LLM 流式生成（Temperature=config.Temperature, Thinking=config.Thinking）
// 输出 EventAgentFinalAnswer 事件
```

---

## 14. 完整 Agent 调用链路图

```
用户发送消息
    ↓
Chat Handler
    ↓
[agent_mode == "smart-reasoning"?]
  YES → AgentEngine.Execute(ctx, query, sessionID, messageID)
          ↓
        BuildSystemPromptWithOptions
          (ProgressiveRAG / PureAgent 模板)
          (注入 KB 列表、Skills 元数据)
          ↓
        buildToolsForLLM
          (toolRegistry → []chat.Tool)
          ↓
        buildMessagesWithLLMContext
          (system + history + query)
          ↓
        ┌─────────────────────────────────┐
        │         executeLoop             │
        │  (MaxIterations 次, 默认 20)    │
        │  Think → Act → Observe         │
        └─────────────────────────────────┘
          ↓
        EventBus.Emit(EventAgentComplete)
          ↓
        SSE 传输给前端
          ↓
        前端 AgentStreamDisplay 渲染
          (折叠树 + 流式答案 + 内联引用)

  NO  → ChatPipeline（Quick Answer / RAG 模式）
```

---

## 15. 默认配置常量

| 常量                       | 值                 | 说明               |
| -------------------------- | ------------------ | ------------------ |
| `DefaultTemperature`       | 0.7                | 默认 LLM 温度      |
| `DefaultMaxIterations`     | 20                 | 默认最大迭代次数   |
| `DefaultReflectionEnabled` | false              | 默认不开启反思     |
| `DefaultThinking`          | false              | 默认不开启思考模式 |
| Reflection Temperature     | 0.5                | 反思 LLM 调用温度  |
| FinalAnswer Temperature    | config.Temperature | 超限强制生成温度   |
