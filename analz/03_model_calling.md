# WeKnora 模型调用能力详解

## 1. 模型调用架构概述

WeKnora 构建了一套完整的多供应商模型调用体系，通过统一接口抽象屏蔽了不同 LLM 服务商的 API 差异，支持**对话模型（Chat）**、**Embedding 向量模型**、**Rerank 重排序模型**和**视觉语言模型（VLM）**四类模型。

```
┌───────────────────────────────────────────────────────┐
│                     模型调用层                          │
├─────────────┬──────────────┬────────────┬─────────────┤
│  Chat (对话) │ Embedding(向量)│ Rerank(重排) │  VLM(视觉)  │
├─────────────┴──────────────┴────────────┴─────────────┤
│              Provider 注册表（20+ 供应商）               │
│  OpenAI│DeepSeek│Aliyun│Zhipu│SiliconFlow│Volcengine... │
└───────────────────────────────────────────────────────┘
```

---

## 2. Provider 注册表体系

文件：`internal/models/provider/provider.go`

### 2.1 支持的供应商（20+）

| Provider 常量         | 说明                         |
| --------------------- | ---------------------------- |
| `ProviderOpenAI`      | OpenAI 官方                  |
| `ProviderAliyun`      | 阿里云 DashScope（通义千问） |
| `ProviderZhipu`       | 智谱 AI（GLM 系列）          |
| `ProviderDeepSeek`    | DeepSeek（R1/V3）            |
| `ProviderGemini`      | Google Gemini                |
| `ProviderVolcengine`  | 火山引擎 Ark                 |
| `ProviderHunyuan`     | 腾讯混元                     |
| `ProviderMiniMax`     | MiniMax（M2）                |
| `ProviderMoonshot`    | 月之暗面（Kimi）             |
| `ProviderModelScope`  | 魔搭 ModelScope              |
| `ProviderQianfan`     | 百度千帆                     |
| `ProviderQiniu`       | 七牛云                       |
| `ProviderSiliconFlow` | 硅基流动                     |
| `ProviderJina`        | Jina AI                      |
| `ProviderOpenRouter`  | OpenRouter 聚合              |
| `ProviderGPUStack`    | GPUStack 私有化部署          |
| `ProviderLongCat`     | 美团 LongCat AI              |
| `ProviderLKEAP`       | 腾讯云 LKEAP                 |
| `ProviderMimo`        | 小米 Mimo                    |
| `ProviderGeneric`     | 自定义 OpenAI 兼容接口       |

### 2.2 供应商自动检测

根据 BaseURL 自动识别供应商：

```go
func DetectProvider(baseURL string) ProviderName {
    switch {
    case containsAny(baseURL, "dashscope.aliyuncs.com"):    return ProviderAliyun
    case containsAny(baseURL, "api.deepseek.com"):          return ProviderDeepSeek
    case containsAny(baseURL, "generativelanguage.googleapis.com"): return ProviderGemini
    case containsAny(baseURL, "volces.com", "volcengine"):  return ProviderVolcengine
    case containsAny(baseURL, "hunyuan.cloud.tencent.com"): return ProviderHunyuan
    case containsAny(baseURL, "lkeap.cloud.tencent.com"):   return ProviderLKEAP
    // ... 20 个 case
    default: return ProviderGeneric // 自定义 OpenAI 兼容
    }
}
```

### 2.3 ProviderInfo 元数据

```go
type ProviderInfo struct {
    Name         ProviderName
    DisplayName  string
    Description  string
    DefaultURLs  map[types.ModelType]string  // 按模型类型区分默认 BaseURL
    ModelTypes   []types.ModelType           // 支持的模型类型
    RequiresAuth bool
    ExtraFields  []ExtraFieldConfig          // UI 额外配置字段
}
```

---

## 3. Chat 对话模型调用

### 3.1 Chat 接口定义

```go
// internal/models/chat/chat.go
type Chat interface {
    Chat(ctx context.Context, messages []Message, opts *ChatOptions) (*types.ChatResponse, error)
    ChatStream(ctx context.Context, messages []Message, opts *ChatOptions) (<-chan types.StreamResponse, error)
    GetModelName() string
    GetModelID() string
}
```

### 3.2 ChatOptions 参数

```go
type ChatOptions struct {
    Temperature         float64         // 温度（默认0.7，推理任务用0.3）
    TopP                float64         // Top-P 采样
    Seed                int             // 随机种子
    MaxTokens           int             // 最大 token（旧字段）
    MaxCompletionTokens int             // 最大完成 token（新字段，优先）
    FrequencyPenalty    float64         // 频率惩罚
    PresencePenalty     float64         // 存在惩罚
    Thinking            *bool           // 是否启用 Thinking（指针，区分 nil/false/true）
    Tools               []Tool          // Function Calling 工具列表
    ToolChoice          string          // "auto"/"required"/"none"/<tool_name>
    Format              json.RawMessage // JSON Schema 输出格式
}
```

### 3.3 Chat 实例工厂

```go
// internal/models/chat/chat.go NewRemoteChat
func NewRemoteChat(config *ChatConfig) (Chat, error) {
    switch providerName {
    case ProviderLKEAP:
        return NewLKEAPChat(config)          // 腾讯云 LKEAP（特殊 thinking 参数格式）
    case ProviderAliyun:
        if IsQwen3Model(config.ModelName) {
            return NewQwenChat(config)       // Qwen3（enable_thinking 字段）
        }
        return NewRemoteAPIChat(config)
    case ProviderDeepSeek:
        return NewDeepSeekChat(config)      // 禁用 tool_choice
    case ProviderGeneric:
        return NewGenericChat(config)       // vLLM ChatTemplateKwargs
    default:
        return NewRemoteAPIChat(config)     // 标准 OpenAI 兼容
    }
}
```

---

## 4. RemoteAPIChat：核心 OpenAI 兼容实现

文件：`internal/models/chat/remote_api.go`（660 行）

### 4.1 结构体

```go
type RemoteAPIChat struct {
    modelName string
    client    *openai.Client          // sashabaranov/go-openai
    modelID   string
    baseURL   string
    apiKey    string
    provider  provider.ProviderName

    // 子类自定义请求钩子（DeepSeek/Qwen3/LKEAP等特殊处理）
    requestCustomizer func(req *openai.ChatCompletionRequest, opts *ChatOptions, isStream bool) (customReq any, useRawHTTP bool)
}
```

`requestCustomizer` 是扩展点，各特殊 Provider 通过设置此函数来定制请求体，无需继承。

### 4.2 非流式调用（Chat）

```go
func (c *RemoteAPIChat) Chat(ctx, messages, opts) (*types.ChatResponse, error) {
    req := c.BuildChatCompletionRequest(messages, opts, false)

    // 特殊 Provider 可通过 requestCustomizer 覆盖请求
    if c.requestCustomizer != nil {
        customReq, useRawHTTP := c.requestCustomizer(&req, opts, false)
        if useRawHTTP {
            return c.chatWithRawHTTP(ctx, customReq)  // 原始 HTTP 请求
        }
    }

    resp, err := c.client.CreateChatCompletion(ctx, req)
    // 解析响应（移除 <think></think> 思考内容）
    return c.parseCompletionResponse(&resp)
}
```

**思考内容处理**（`removeThinkingContent`）：
```go
// 兜底策略：即使 opts.Thinking=false，部分模型仍可能返回 <think> 标签
func removeThinkingContent(content string) string {
    if !strings.HasPrefix(trimmed, "<think>") { return content }
    if lastEndIdx := strings.LastIndex(trimmed, "</think>"); lastEndIdx != -1 {
        return strings.TrimSpace(trimmed[lastEndIdx+len("</think>"):])
    }
    return "" // 截断的思考内容
}
```

### 4.3 流式调用（ChatStream）

```go
func (c *RemoteAPIChat) ChatStream(ctx, messages, opts) (<-chan types.StreamResponse, error) {
    req := c.BuildChatCompletionRequest(messages, opts, true)

    stream, _ := c.client.CreateChatCompletionStream(ctx, req)
    streamChan := make(chan types.StreamResponse)
    go c.processStream(ctx, stream, streamChan)

    return streamChan, nil
}
```

### 4.4 流式状态机（streamState）

流式响应通过 `streamState` 管理多工具调用的增量拼接：

```go
type streamState struct {
    toolCallMap      map[int]*types.LLMToolCall       // index → 工具调用（增量拼接）
    lastFunctionName map[int]string                   // 上一次函数名（检测名称完整性）
    nameNotified     map[int]bool                     // 是否已发送 ToolCall 事件
    hasThinking      bool                             // 是否处于思考阶段
    fieldExtractors  map[int]*jsonFieldExtractor      // 流式 JSON 字段提取器
}
```

### 4.5 StreamResponse 事件类型

| ResponseType           | 含义                       |
| ---------------------- | -------------------------- |
| `ResponseTypeAnswer`   | 回答内容（普通文本）       |
| `ResponseTypeThinking` | 思考内容（DeepSeek R1 等） |
| `ResponseTypeToolCall` | 工具调用开始通知           |
| `ResponseTypeError`    | 错误事件                   |

### 4.6 特殊工具流式处理

```go
// final_answer 工具：将 answer 字段流式解析为 ResponseTypeAnswer
if toolCallEntry.Function.Name == "final_answer" {
    extractor = newJSONFieldExtractor("answer")
    answerChunk := extractor.Feed(tc.Function.Arguments)
    // → ResponseTypeAnswer
}

// thinking 工具：将 thought 字段流式解析为 ResponseTypeThinking
if toolCallEntry.Function.Name == "thinking" {
    extractor = newJSONFieldExtractor("thought")
    thoughtChunk := extractor.Feed(tc.Function.Arguments)
    // → ResponseTypeThinking
}
```

这意味着即使在使用 Function Calling 的 Agent 模式下，思考过程（`thinking` 工具）也能实时流式推送给前端。

---

## 5. 特殊 Provider 实现

### 5.1 DeepSeek（deepseek.go）

```go
// DeepSeekChat 禁用 tool_choice（DeepSeek API 不支持）
type DeepSeekChat struct {
    *RemoteAPIChat
}
// 通过 requestCustomizer 清空 req.ToolChoice
```

DeepSeek 特殊能力：
- `delta.ReasoningContent`：原生思考 token，直接映射为 `ResponseTypeThinking`
- R1 模型自动识别 `reasoning_content` 字段

### 5.2 Qwen3（qwen.go）

```go
// Qwen3 通过 enable_thinking 参数控制思考模式
// 使用 requestCustomizer 注入 extra_body.enable_thinking
```

Qwen3 模型识别：
```go
func IsQwen3Model(modelName string) bool {
    return strings.Contains(strings.ToLower(modelName), "qwen3")
}
```

### 5.3 LKEAP（lkeap.go）

腾讯云知识引擎原子能力（LKEAP）的特殊 `thinking` 参数格式，与标准 OpenAI 不同。

### 5.4 Generic/vLLM（generic.go）

针对私有化部署（如 vLLM）注入 `chat_template_kwargs`：
```json
{
  "chat_template_kwargs": {"enable_thinking": true/false}
}
```

### 5.5 Ollama（ollama.go）

本地 Ollama 模型实现：
- 使用 `github.com/ollama/ollama/api` 客户端
- 支持 `Think` 字段（Qwen3、DeepSeek 本地版）
- `ensureModelAvailable`：自动 Pull 模型（若不存在）
- Token 统计：`resp.PromptEvalCount` + `resp.EvalCount`

---

## 6. Embedding 模型调用

文件：`internal/models/embedding/embedder.go`

### 6.1 Embedder 接口

```go
type Embedder interface {
    Embed(ctx context.Context, text string) ([]float32, error)
    BatchEmbed(ctx context.Context, texts []string) ([][]float32, error)
    GetModelName() string
    GetDimensions() int
    GetModelID() string
    BatchEmbedWithPool(ctx context.Context, model Embedder, texts []string) ([][]float32, error)
}
```

### 6.2 批量 Embedding 并发池（EmbedderPooler）

```go
// BatchEmbedWithPool 使用协程池并发批量向量化
// 并发度 = 5（EmbeddingConcurrency）
func BatchEmbedWithPool(ctx, model, texts) ([][]float32, error) {
    pool, _ := ants.NewPool(EmbeddingConcurrency)
    results := make([][]float32, len(texts))

    for i, text := range texts {
        pool.Submit(func() {
            vec, _ := model.Embed(ctx, text)
            results[i] = vec
        })
    }
}
```

### 6.3 各提供商 Embedding 实现

| 文件            | 提供商           | 特点                                                          |
| --------------- | ---------------- | ------------------------------------------------------------- |
| `openai.go`     | 通用 OpenAI 兼容 | 标准实现（SiliconFlow/DeepSeek等）                            |
| `aliyun.go`     | 阿里云 DashScope | 多模态支持（`tongyi-embedding-vision-*`），文本模型走兼容模式 |
| `volcengine.go` | 火山引擎 Ark     | 多模态 Embedding API                                          |
| `jina.go`       | Jina AI          | `truncate` 参数（非 `truncate_prompt_tokens`）                |
| `ollama.go`     | Ollama 本地      | 本地向量化                                                    |

---

## 7. Rerank 模型调用

文件：`internal/models/rerank/reranker.go`

### 7.1 Reranker 接口

```go
type Reranker interface {
    Rerank(ctx context.Context, query string, documents []string) ([]RankResult, error)
    GetModelName() string
    GetModelID() string
}

type RankResult struct {
    Index          int          // 文档原始索引
    Document       DocumentInfo // 文档内容
    RelevanceScore float64      // 相关性分数
}
```

### 7.2 RankResult JSON 兼容解析

```go
// 兼容 relevance_score（标准）和 score（部分供应商）两种字段名
func (r *RankResult) UnmarshalJSON(data []byte) error {
    if temp.RelevanceScore != nil { r.RelevanceScore = *temp.RelevanceScore }
    else if temp.Score != nil     { r.RelevanceScore = *temp.Score }
}
```

### 7.3 DocumentInfo 解析兼容

```go
// 兼容两种格式：
// 1. 字符串格式: "document text"
// 2. 对象格式:  {"text": "document text"}
func (d *DocumentInfo) UnmarshalJSON(data []byte) error {
    var text string
    if json.Unmarshal(data, &text) == nil { d.Text = text; return nil }
    // fallback to object format
}
```

### 7.4 Reranker 实例工厂

```go
func NewReranker(config *RerankerConfig) (Reranker, error) {
    switch providerName {
    case ProviderAliyun: return NewAliyunReranker(config)
    case ProviderZhipu:  return NewZhipuReranker(config)
    case ProviderJina:   return NewJinaReranker(config)
    default:             return NewOpenAIReranker(config)  // OpenAI 兼容
    }
}
```

---

## 8. 模型数据库表（models 表）

```sql
CREATE TABLE models (
    id VARCHAR(64) PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id INTEGER NOT NULL,         -- 租户隔离
    name VARCHAR(255) NOT NULL,         -- 模型名称（用于API调用）
    type VARCHAR(50) NOT NULL,          -- 模型类型：chat/embedding/rerank/vlm
    source VARCHAR(50) NOT NULL,        -- 来源：remote/local
    description TEXT,
    parameters JSONB NOT NULL,          -- 配置参数（baseURL/apiKey/provider等）
    is_default BOOLEAN NOT NULL DEFAULT false,
    is_builtin BOOLEAN NOT NULL DEFAULT false, -- 是否内置模型
    status VARCHAR(50) NOT NULL DEFAULT 'active',
    created_at, updated_at, deleted_at
);

-- parameters JSONB 示例：
{
  "base_url": "https://api.deepseek.com/v1",
  "api_key": "sk-xxx",
  "provider": "deepseek",
  "model_name": "deepseek-reasoner",
  "dimensions": 1536    -- Embedding专用
}
```

---

## 9. 模型类型与使用场景

| 模型类型  | 表中 type 值 | 使用场景                              |
| --------- | ------------ | ------------------------------------- |
| 对话/生成 | `chat`       | RAG 回答生成、Agent 推理、查询重写    |
| 向量嵌入  | `embedding`  | 文档向量化、查询向量化                |
| 重排序    | `rerank`     | 检索结果重排序                        |
| 视觉语言  | `vlm`        | 图片内容理解（文档中的图片 OCR/描述） |

---

## 10. 思考模型（Reasoning Model）全链路支持

WeKnora 对支持推理能力的模型有完整的多层处理：

### 10.1 触发层（ChatOptions.Thinking）

```go
// Agent 推理时启用 thinking
opts := &chat.ChatOptions{
    Thinking: &thinking, // true=启用，false=关闭，nil=不传
}
```

### 10.2 传输层（各 Provider 特殊处理）

| Provider       | 思考参数方式                           |
| -------------- | -------------------------------------- |
| DeepSeek       | `delta.ReasoningContent` 原生字段      |
| Qwen3 (Aliyun) | `extra_body.enable_thinking`           |
| LKEAP          | 特殊 thinking 格式                     |
| Ollama         | `Think` 字段（ollama/api）             |
| vLLM (Generic) | `chat_template_kwargs.enable_thinking` |

### 10.3 响应层（StreamResponse）

```go
// 思考内容事件
types.StreamResponse{ResponseType: ResponseTypeThinking, Content: "思考过程..."}

// 思考完成标志
types.StreamResponse{ResponseType: ResponseTypeThinking, Done: true}

// 正式回答
types.StreamResponse{ResponseType: ResponseTypeAnswer, Content: "最终回答..."}
```

### 10.4 前端渲染（AgentStreamDisplay.vue）

前端监听 `ResponseTypeThinking` 事件，渲染折叠的"思考过程"区块，用户可展开查看。

---

## 11. Function Calling（工具调用）支持

### 11.1 工具定义格式

```go
type Tool struct {
    Type     string      // "function"
    Function FunctionDef
}
type FunctionDef struct {
    Name        string
    Description string
    Parameters  json.RawMessage  // JSON Schema 格式
}
```

### 11.2 ToolChoice 控制

```go
switch opts.ToolChoice {
case "none", "required", "auto":
    req.ToolChoice = opts.ToolChoice
default:  // 指定特定工具名
    req.ToolChoice = openai.ToolChoice{
        Type:     "function",
        Function: openai.ToolFunction{Name: opts.ToolChoice},
    }
}
```

**DeepSeek 特殊处理**：DeepSeek 不支持 `tool_choice`，`DeepSeekChat` 的 `requestCustomizer` 会清空该字段。

### 11.3 Tool Call 流式拼接

OpenAI 流式 Function Calling 中，函数名和参数通过多个 delta 逐步传输：

```go
// 每个 delta 追加到对应 toolCallMap[index]
toolCallEntry.Function.Name += tc.Function.Name
toolCallEntry.Function.Arguments += tc.Function.Arguments

// 名称完整后（stabliized）发送 ToolCall 开始通知
if currName != "" && currName == lastFunctionName && !nameNotified {
    streamChan <- ResponseTypeToolCall{tool_name, tool_call_id}
}
```

---

## 12. 模型调用全流程

```
用户请求
    │
    ▼
[Agent/RAG 逻辑] 构建 Messages[]
    │
    ▼
NewChat(config, ollamaService) ← 工厂函数（根据 source/provider 路由）
    │
    ├── source=local → OllamaChat.ChatStream()
    │       └── ollama HTTP API → 本地推理
    │
    └── source=remote → NewRemoteChat(config)
            │
            ├── provider=LKEAP   → LKEAPChat (requestCustomizer)
            ├── provider=aliyun  → QwenChat (enable_thinking) 或 RemoteAPIChat
            ├── provider=deepseek → DeepSeekChat (no tool_choice)
            ├── provider=generic  → GenericChat (ChatTemplateKwargs)
            └── default          → RemoteAPIChat
                    │
                    └── sashabaranov/go-openai SDK
                            │
                            ├── 非流式: CreateChatCompletion
                            └── 流式:  CreateChatCompletionStream
                                        │
                                        └── processStream goroutine
                                                │
                                                ├── ResponseTypeThinking（思考过程）
                                                ├── ResponseTypeToolCall（工具调用开始）
                                                └── ResponseTypeAnswer（最终回答）
                                                         │
                                                         ▼
                                                    SSE 推流至前端
```
