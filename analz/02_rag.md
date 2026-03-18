# WeKnora RAG 能力详解

## 1. RAG 架构概述

WeKnora 实现了完整的检索增强生成（RAG）流程，采用**事件驱动的插件化 Pipeline 架构**，将整个 RAG 流程分解为多个可组合的阶段插件，通过 `EventManager` 串联执行。

### 1.1 Pipeline 核心架构

```
EventManager (事件管理器)
     │
     ├── REWRITE_QUERY ──► PluginRewrite（查询重写）
     ├── CHUNK_SEARCH  ──► PluginSearch（混合检索：KB + Web）
     ├── CHUNK_RERANK  ──► PluginRerank（重排序 + MMR去重）
     ├── CHUNK_MERGE   ──► PluginMerge（父子切片合并 + 上下文窗口）
     ├── DATA_ANALYSIS ──► PluginDataAnalysis（数据分析场景）
     └── CHAT_COMPLETE ──► PluginChatCompletion（LLM流式生成）
```

文件：`internal/application/service/chat_pipline/`

---

## 2. EventManager 插件机制

### 2.1 Plugin 接口定义

```go
// internal/application/service/chat_pipline/chat_pipline.go
type Plugin interface {
    OnEvent(ctx context.Context, eventType types.EventType,
            chatManage *types.ChatManage, next func() *PluginError) *PluginError
    ActivationEvents() []types.EventType
}
```

### 2.2 链式处理（责任链模式）

```go
// 插件链构建：反向迭代，构建 next 闭包链
func (e *EventManager) buildHandler(plugins []Plugin) func(...) *PluginError {
    next := func(ctx, eventType, chatManage) *PluginError { return nil }
    for i := len(plugins) - 1; i >= 0; i-- {
        current := plugins[i]
        prevNext := next
        next = func(...) *PluginError {
            return current.OnEvent(ctx, eventType, chatManage, func() *PluginError {
                return prevNext(ctx, eventType, chatManage)
            })
        }
    }
    return next
}
```

每个 Plugin 通过调用 `next()` 来传递控制权，支持**前置处理**（在 `next()` 之前）和**后置处理**（在 `next()` 之后）。

### 2.3 ChatManage：Pipeline 状态载体

`types.ChatManage` 是整个 Pipeline 的状态对象，在所有插件中传递：

| 字段                   | 类型            | 说明                             |
| ---------------------- | --------------- | -------------------------------- |
| `SessionID`            | string          | 会话ID                           |
| `TenantID`             | uint64          | 租户ID                           |
| `Query`                | string          | 用户原始问题                     |
| `RewriteQuery`         | string          | 重写后的查询                     |
| `SearchTargets`        | []*SearchTarget | 检索目标（KB/知识）              |
| `SearchResult`         | []*SearchResult | 检索结果（Embedding+关键词混合） |
| `RerankResult`         | []*SearchResult | 重排后结果                       |
| `MergedResult`         | []*MergeResult  | 合并后最终上下文                 |
| `EmbeddingTopK`        | int             | 向量检索TopK（默认10）           |
| `VectorThreshold`      | float64         | 向量相似度阈值（默认0.5）        |
| `KeywordThreshold`     | float64         | 关键词阈值（默认0.3）            |
| `RerankModelID`        | string          | 重排模型ID                       |
| `RerankThreshold`      | float64         | 重排阈值（默认0.65）             |
| `RerankTopK`           | int             | 重排TopK（默认10）               |
| `EnableRewrite`        | bool            | 是否启用查询重写                 |
| `EnableQueryExpansion` | bool            | 是否启用查询扩展                 |
| `WebSearchEnabled`     | bool            | 是否启用网络搜索                 |
| `History`              | []*History      | 对话历史                         |
| `FAQPriorityEnabled`   | bool            | 是否启用FAQ优先                  |
| `FAQScoreBoost`        | float64         | FAQ分数加成系数                  |

---

## 3. 第一阶段：查询重写（PluginRewrite）

文件：`internal/application/service/chat_pipline/rewrite.go`

### 3.1 工作流程

```
用户问题
   │
   ├── EnableRewrite=false → 跳过，直接使用原始问题
   ├── History=空 → 跳过（单轮对话不需要重写）
   └── History=有 → 
         ├── 从数据库获取最近20条消息（GetRecentMessagesBySession）
         ├── 按 RequestID 分组成 (Query, Answer) 对
         ├── 按时间排序，取最近 maxRounds 轮（默认5轮）
         ├── 构建 Prompt（system + user 模板）
         ├── 调用 LLM（Temperature=0.3, MaxCompletionTokens=50）
         └── 更新 chatManage.RewriteQuery
```

### 3.2 Prompt 模板

```go
// config/prompt_templates/rewrite_system.yaml
// config/prompt_templates/rewrite_user.yaml
// 占位符：{{conversation}} {{query}} {{current_time}} {{yesterday}}
```

重写调用参数：
- **Temperature**: `0.3`（低随机性，确保确定性重写）
- **MaxCompletionTokens**: `50`（重写结果简短）
- **Thinking**: `false`（禁用思考模式）

---

## 4. 第二阶段：混合检索（PluginSearch）

文件：`internal/application/service/chat_pipline/search.go`

### 4.1 并发搜索架构

```go
// KB搜索 和 Web搜索 并发执行
var wg sync.WaitGroup
wg.Add(2)

go func() { // Goroutine 1: 知识库检索
    kbResults := p.searchByTargets(ctx, chatManage)
}()

go func() { // Goroutine 2: 网络搜索（可选）
    webResults := p.searchWebIfEnabled(ctx, chatManage)
}()

wg.Wait()
```

### 4.2 知识库检索（searchByTargets）

对每个 `SearchTarget` 并发执行：

```go
for _, target := range chatManage.SearchTargets {
    go func(t *types.SearchTarget) {
        // 1. 尝试直接加载（小文件 <= 50 chunks 直接全加载，Score=1.0）
        directResults, skippedIDs := p.tryDirectChunkLoading(ctx, tenantID, t.KnowledgeIDs)

        // 2. 对无法直接加载的文件进行混合搜索
        params := types.SearchParams{
            QueryText:        chatManage.RewriteQuery,
            VectorThreshold:  chatManage.VectorThreshold,
            KeywordThreshold: chatManage.KeywordThreshold,
            MatchCount:       chatManage.EmbeddingTopK,
        }
        res, _ := p.knowledgeBaseService.HybridSearch(ctx, t.KnowledgeBaseID, params)
    }(target)
}
```

**直接加载机制**（DirectLoad）：
- 文档切片总数 ≤ 50 块时，全量加载所有切片
- Score = 1.0（最高分，绕过向量检索）
- 适用于小型文档精确问答

### 4.3 混合检索引擎（HybridSearch → KeywordsVectorHybridIndexer）

文件：`internal/application/service/retriever/keywords_vector_hybrid_indexer.go`

```
HybridSearch
     │
     ├── 向量检索（HNSW余弦相似度）── 并发 ──┐
     │   VectorThreshold 过滤                  ├── 合并去重 → 按Score排序
     └── 关键词检索（BM25）──────── 并发 ──────┘
         KeywordThreshold 过滤
```

**KeywordsVectorHybridIndexer 并发实现**：
```go
// 批量 Embedding，并发度 = 5
const EmbeddingConcurrency = 5

// 并发执行 Embedding 请求（panjf2000/ants 协程池）
pool, _ := ants.NewPool(EmbeddingConcurrency)
for i, text := range texts {
    pool.Submit(func() {
        vec, _ := embedder.Embed(ctx, text)
        // 存入 embeddings 表（halfvec列 + HNSW索引）
    })
}
```

**向量索引存储**：
- PostgreSQL `embeddings` 表
- 列类型：`halfvec`（半精度浮点，节省50%存储）
- 索引：`HNSW (m=16, ef_construction=64)` 余弦相似度

**BM25全文检索**：
- ParadeDB pg_search 扩展
- 中文分词：`chinese_lindera` tokenizer
- 索引：`embeddings_search_idx ON embeddings USING bm25`

### 4.4 查询扩展（EnableQueryExpansion）

```go
// 当召回量不足时，触发查询扩展
if chatManage.EnableQueryExpansion && len(chatManage.SearchResult) < max(1, chatManage.EmbeddingTopK) {
    expResults := p.runQueryExpansion(ctx, chatManage)
    chatManage.SearchResult = append(chatManage.SearchResult, expResults...)
}
```

文件：`internal/application/service/chat_pipline/query_expansion.go`
- 通过 LLM 对原始查询生成多个语义变体
- 对每个扩展查询重新检索
- 合并结果，增加召回率

### 4.5 网络搜索集成

```go
// search.go searchWebIfEnabled
webResults, _ := p.webSearchService.Search(ctx, tenant.WebSearchConfig, rewriteQuery)

// RAG压缩网络结果（存入临时知识库）
compressed, kbID, _, _, _ := p.webSearchService.CompressWithRAG(
    ctx, sessionID, tempKBID, questions, webResults, ...)

// 临时知识库状态持久化到 Redis
p.webSearchStateService.SaveWebSearchTempKBState(ctx, sessionID, kbID, newSeen, newIDs)
```

网络搜索结果会：
1. 进行 RAG 压缩（存入 Session 级临时知识库）
2. 在同一个会话中复用（避免重复搜索）
3. 转换为标准 `SearchResult`，与 KB 结果统一处理

---

## 5. 第三阶段：重排序（PluginRerank）

文件：`internal/application/service/chat_pipline/rerank.go`

### 5.1 重排核心流程

```
SearchResult（候选集）
   │
   ├── DirectLoad 结果 ─────────────── 跳过 Rerank（Score=1.0）
   │
   └── 普通检索结果 →
         ├── 清洗 Passage（去除Markdown/代码/图片噪声）
         ├── 增强 Passage（加入图片OCR文本 + 生成问题）
         ├── 调用 Rerank 模型（Reranker.Rerank）
         │     RelevanceScore >= RerankThreshold 过滤
         │     [无结果且阈值>0.3] → 降级阈值（×0.7 降至最低0.3）重试
         │     [结果仍为空且TopScore>=0.15] → 强制保留Top1结果
         ├── 计算综合分（compositeScore）
         └── FAQ 分数加成（如果启用）
```

### 5.2 综合分公式

```go
// compositeScore 综合评分算法
func compositeScore(sr *types.SearchResult, modelScore, baseScore float64) float64 {
    // 来源权重：知识库=1.0，网络搜索=0.95
    sourceWeight := 1.0
    if sr.KnowledgeSource == "web_search" { sourceWeight = 0.95 }

    // 位置先验：文档前段内容略有加分
    positionPrior := 1.0 + Clamp(1.0 - float64(sr.StartAt)/(sr.EndAt+1), -0.05, 0.05)

    // 加权组合
    composite := 0.6*modelScore + 0.3*baseScore + 0.1*sourceWeight
    composite *= positionPrior
    return Clamp(composite, 0, 1)
}
```

| 权重 | 来源                     | 说明             |
| ---- | ------------------------ | ---------------- |
| 60%  | 重排模型分 (modelScore)  | 语义相关性       |
| 30%  | 基础检索分 (baseScore)   | 向量/BM25原始分  |
| 10%  | 来源权重 (sourceWeight)  | 知识库 > 网络    |
| ×    | 位置先验 (positionPrior) | 前段内容轻微加分 |

### 5.3 MMR 去重算法（最大边际相关性）

```go
// applyMMR：在相关性和多样性之间平衡
// lambda=0.7（偏向相关性）
func applyMMR(results []*types.SearchResult, k int, lambda=0.7) []*types.SearchResult {
    for len(selected) < k {
        // MMR 分数 = lambda * 相关性 - (1-lambda) * 最大冗余度
        mmr := lambda*relevance - (1.0-lambda)*maxJaccardSimilarity
        // 选择 MMR 分最高的候选
    }
}
```

去重基于 **Jaccard Tokenized 相似度**（简单分词后 token 交集/并集）。

### 5.4 Passage 清洗（cleanPassageForRerank）

发送给 Rerank 模型前，清除语义噪声：
- 移除代码块、LaTeX 块
- 移除 HTML 标签
- Markdown 图片引用（`![alt](url)`) → 移除
- Markdown 链接（`[text](url)`) → 保留文本
- 裸 URL → 移除
- 表格分隔行、标题 `#` 符号、引用 `>` → 移除
- 粗体/斜体标记 → 保留文本内容
- 列表标记 → 移除

### 5.5 Passage 增强（getEnrichedPassage）

Rerank 时额外补充的语义信息：
- **图片 OCR 文本**：`chunk.ImageInfo` 中的 `OCRText`
- **图片描述（Caption）**：`chunk.ImageInfo` 中的 `Caption`
- **生成的问题**：`chunk.ChunkMetadata.GeneratedQuestions`

FAQ 分数加成：
```go
// FAQ 类型切片享受额外加分（chatManage.FAQScoreBoost 系数，最高1.0）
if sr.ChunkType == "faq" && chatManage.FAQPriorityEnabled {
    sr.Score = math.Min(sr.Score * chatManage.FAQScoreBoost, 1.0)
}
```

---

## 6. 第四阶段：切片合并（PluginMerge）

文件：`internal/application/service/chat_pipline/merge.go`

### 6.1 核心处理步骤

```
RerankResult（重排结果）
    │
    ├── 去重（按 chunk_id 和 content signature）
    │
    ├── 注入历史消息中的知识引用（相关性过滤）
    │
    ├── 父切片解析（resolveParentChunks）
    │   └── 子切片 → 获取 parent_chunk 完整内容（更丰富上下文）
    │
    ├── 按 KnowledgeID 分组
    │
    └── 对每个文档的切片按 ChunkIndex 排序
          ├── 合并相邻切片（间距 ≤ merge_gap）
          └── 扩展上下文窗口（context_window 前后扩展）
```

### 6.2 父切片解析

WeKnora 支持**层级切片**（parent-child chunks）：
- `parent_chunk_id`：父切片 ID（如段落→父，句子→子）
- 检索命中子切片时，自动升级为父切片（更完整内容）

```go
func (p *PluginMerge) resolveParentChunks(ctx, chatManage, results) []*types.SearchResult {
    for _, result := range results {
        if result.ParentChunkID != "" {
            parentChunk, _ := p.chunkService.GetChunk(ctx, result.ParentChunkID)
            // 用父切片内容替换子切片内容
            result.Content = parentChunk.Content
        }
    }
}
```

### 6.3 上下文窗口扩展

对同一文档的相邻切片按顺序合并，扩展检索上下文覆盖范围。

---

## 7. 支持的向量数据库引擎

WeKnora 通过 `CompositeRetrieveEngine` 支持多种检索引擎并行注册：

| 引擎类型        | 实现                       | 说明                        |
| --------------- | -------------------------- | --------------------------- |
| `postgres`      | pgvector + pg_search       | **默认引擎**，向量+BM25双路 |
| `milvus`        | milvus-io/milvus/client/v2 | 高性能向量数据库            |
| `qdrant`        | qdrant/go-client           | 轻量级向量数据库            |
| `weaviate`      | weaviate-go-client/v5      | 语义向量库                  |
| `elasticsearch` | go-elasticsearch/v8        | 全文+向量混合               |

多引擎并发操作：
```go
// CompositeRetrieveEngine.concurrentExecWithError
// 对所有注册引擎并发执行 Index/Delete 等操作
for _, engineInfo := range c.engineInfos {
    go func(eng *engineInfo) {
        eng.retrieveEngine.Index(ctx, embedder, indexInfo, ...)
    }(engineInfo)
}
```

---

## 8. Embedding 向量化体系

文件：`internal/models/embedding/`

### 8.1 Embedder 接口

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

### 8.2 支持的 Embedding 提供商

| 实现文件        | 支持提供商                                                    |
| --------------- | ------------------------------------------------------------- |
| `openai.go`     | OpenAI、DeepSeek、SiliconFlow 等 OpenAI 兼容                  |
| `aliyun.go`     | 阿里云 DashScope（文本 + 多模态 `tongyi-embedding-vision-*`） |
| `volcengine.go` | 火山引擎 Ark（多模态 Embedding）                              |
| `jina.go`       | Jina AI（truncate 参数差异处理）                              |
| `ollama.go`     | Ollama 本地模型                                               |

路由逻辑（`embedder.go`）：
```go
switch providerName {
case ProviderAliyun:
    if isMultimodalModel { return NewAliyunEmbedder(...) }
    return NewOpenAIEmbedder(...) // 兼容模式 URL
case ProviderVolcengine:
    return NewVolcengineEmbedder(...)
case ProviderJina:
    return NewJinaEmbedder(...)
default:
    return NewOpenAIEmbedder(...) // 通用 OpenAI 兼容
}
```

### 8.3 halfvec 优化

PostgreSQL 存储使用 `halfvec`（FP16）而非 `vector`（FP32）：
- 存储节省 50%
- HNSW 索引参数：`m=16, ef_construction=64`（平衡精度与性能）
- 支持多维度：当前配置了 `3584` 和 `798` 两种常见维度

---

## 9. Rerank 重排序模型体系

文件：`internal/models/rerank/`

### 9.1 Reranker 接口

```go
type Reranker interface {
    Rerank(ctx context.Context, query string, documents []string) ([]RankResult, error)
    GetModelName() string
    GetModelID() string
}
```

### 9.2 支持的 Reranker

| 实现文件             | 支持提供商          |
| -------------------- | ------------------- |
| `remote_api.go`      | OpenAI 兼容（默认） |
| `aliyun_reranker.go` | 阿里云 DashScope    |
| `jina_reranker.go`   | Jina AI             |
| `zhipu_reranker.go`  | 智谱 AI             |

### 9.3 RankResult 解析兼容

```go
// 兼容两种字段名（relevance_score 和 score）
func (r *RankResult) UnmarshalJSON(data []byte) error {
    if temp.RelevanceScore != nil { r.RelevanceScore = *temp.RelevanceScore }
    else if temp.Score != nil     { r.RelevanceScore = *temp.Score }
}
```

---

## 10. RAG 全流程数据流

```
用户提问 "XXX"
    │
    ▼
[1] QueryRewrite（PluginRewrite）
    ├── 获取最近 20 条消息
    ├── 按 requestID 分组，取最近 5 轮对话
    └── LLM 重写 → "优化后的查询"（Temperature=0.3, MaxTokens=50）
    │
    ▼
[2] Concurrent Search（PluginSearch）
    ├── [G1] KB 搜索（并发多知识库）
    │   ├── 小文档（≤50 chunks）→ 直接全加载（Score=1.0）
    │   └── 大文档 → HybridSearch
    │       ├── [并发] 向量检索（halfvec HNSW余弦）
    │       └── [并发] 关键词检索（BM25 中文分词）
    ├── [G2] Web 搜索（可选，并发）
    │   ├── 调用搜索服务
    │   └── RAG 压缩 → 存入 Session 临时知识库
    └── 合并所有结果 → SearchResult[]
    │
    ▼
[可选] QueryExpansion（召回量不足时）
    ├── LLM 生成多变体查询
    └── 重新检索，追加结果
    │
    ▼
[3] Rerank（PluginRerank）
    ├── 清洗 Passage（去除 Markdown/代码噪声）
    ├── 增强 Passage（图片 OCR + 生成问题）
    ├── 调用 Rerank 模型 API
    ├── 阈值过滤（自适应降级）
    ├── 计算综合分（0.6×模型分 + 0.3×基础分 + 0.1×来源权重）
    ├── FAQ 加分（可选）
    └── MMR 去重（lambda=0.7，k=RerankTopK）→ RerankResult[]
    │
    ▼
[4] Merge（PluginMerge）
    ├── ID+内容签名去重
    ├── 注入历史知识引用
    ├── 父切片升级（子→父，更完整内容）
    ├── 按文档分组 + 切片排序
    └── 合并相邻切片 + 扩展上下文窗口 → MergedResult[]
    │
    ▼
[5] ChatCompletion（PluginChatCompletion）
    ├── 渲染 Context 模板（{{contexts}} 占位符）
    ├── 构建消息历史（加入 MergedResult 上下文）
    └── 流式调用 LLM → SSE 推流给前端
```

---

## 11. RAG 配置参数（CustomAgent Config JSONB 字段）

```json
{
  "embedding_top_k": 10,        // 向量检索TopK
  "keyword_threshold": 0.3,     // 关键词阈值
  "vector_threshold": 0.5,      // 向量阈值
  "rerank_top_k": 5,            // 重排TopK
  "rerank_threshold": 0.5,      // 重排阈值
  "enable_query_expansion": true, // 查询扩展
  "enable_rewrite": true,        // 查询重写
  "web_search_enabled": false,   // 网络搜索
  "web_search_max_results": 5,   // 网络搜索结果数
  "multi_turn_enabled": true,    // 多轮对话
  "history_turns": 5,            // 历史对话轮数
  "fallback_strategy": "model",  // 无结果回退策略
  "fallback_response": "...",    // 固定回退文案
  "context_template": "..."      // 上下文模板
}
```

---

## 12. 相关数据库表

| 表               | 关键字段                                                                | 说明                   |
| ---------------- | ----------------------------------------------------------------------- | ---------------------- |
| `knowledges`     | `parse_status`, `enable_status`                                         | 知识文档状态管理       |
| `chunks`         | `content`, `chunk_index`, `parent_chunk_id`, `chunk_type`, `image_info` | 切片存储（含父子关系） |
| `embeddings`     | `embedding halfvec`, `dimension`, `source_id`                           | 向量存储 + BM25索引    |
| `sessions`       | `embedding_top_k`, `vector_threshold`, `rerank_threshold`               | 会话级检索配置         |
| `knowledge_tags` | `name`, `color`                                                         | 知识标签（切片过滤）   |

---

## 13. 知识图谱检索（可选）

WeKnora 支持 **Neo4j 知识图谱**增强检索：
- 服务：`internal/application/service/graph.go`（32KB）
- Agent 工具：`internal/agent/tools/query_knowledge_graph.go`
- 图谱构建：实体抽取（LLM） + 关系抽取 → 存入 Neo4j
- 检索路径：`knowledge_search` → 匹配知识图谱节点 → 扩展关联知识
