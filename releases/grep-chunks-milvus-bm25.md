# GrepChunks 关键词搜索迁移至 Milvus BM25 全文检索

**日期**：2026-04-17  
**涉及模块**：`internal/agent/tools/grep_chunks.go`、`internal/application/service/agent_service.go`、`internal/application/repository/retriever/milvus/repository.go`

---

## 背景与动机

WeKnora 的 `grep_chunks` 工具此前通过 PostgreSQL `LIKE` 语句对 `chunks` 表做全文关键词搜索，存在以下问题：

- `LIKE '%keyword%'` 无法利用索引，全表扫描，大数据量下性能差
- 不支持 BM25 相关性打分，无法按相关度排序
- 中文分词完全依赖数据库层，效果差

WeKnora 的 Milvus 集成中，`content_sparse`（`SparseVector`）字段和 `text_bm25_emb` BM25 内置 function 已在 schema 中定义好，但从未被 `grep_chunks` 工具使用。本次迁移将 `grep_chunks` 的搜索后端切换到 Milvus BM25 全文检索。

---

## 架构说明

### Milvus BM25 工作原理

```
写入时：
  content (VarChar) ──[text_bm25_emb function]──> content_sparse (SparseVector)
                        （Milvus 自动完成，业务层无感知）

搜索时：
  query string ──[entity.Text(query)]──> ANN search on content_sparse ──> BM25 ranked results
```

关键点：
- `content` 字段设置了 `WithEnableAnalyzer(true)` + `WithEnableMatch(true)`
- BM25 function 绑定在 collection schema 级别，写入时 Milvus 自动填充 `content_sparse`，**业务层 upsert 不需要传 `content_sparse`**
- 搜索时通过 `entity.Text(query)` + `WithANNSField(fieldContentSparse)` 触发 BM25 检索

### 调用链

```
agent_service.go
  └── NewGrepChunksTool(db, milvusRetrieveEngine, knowledgeService, searchTargets)
        └── grep_chunks.go Execute()
              ├── [retrieveEngine != nil] searchChunksMilvus()
              │     └── retrieveEngine.Retrieve(ctx, RetrieveParams{RetrieverType: KeywordsRetrieverType})
              │           └── milvusRepository.KeywordsRetrieve() ← BM25 搜索
              └── [retrieveEngine == nil] searchChunks() ← 降级 PG LIKE
```

---

## 改动详情

### 1. `grep_chunks.go`

**结构体变更：**

```go
// 改前
type GrepChunksTool struct {
    BaseTool
    db            *gorm.DB
    searchTargets types.SearchTargets
}

// 改后
type GrepChunksTool struct {
    BaseTool
    db             *gorm.DB                    // fallback: PostgreSQL LIKE（Milvus 未配置时使用）
    retrieveEngine interfaces.RetrieveEngine   // primary: Milvus BM25（优先使用）
    knowledgeSvc   interfaces.KnowledgeService // optional: 回填 KnowledgeTitle
    searchTargets  types.SearchTargets
}
```

**构造函数变更：**

```go
// 改前
func NewGrepChunksTool(db *gorm.DB, searchTargets types.SearchTargets) *GrepChunksTool

// 改后
func NewGrepChunksTool(
    db *gorm.DB,
    retrieveEngine interfaces.RetrieveEngine,
    knowledgeSvc interfaces.KnowledgeService,
    searchTargets types.SearchTargets,
) *GrepChunksTool
```

**Execute() 分发逻辑：**

```go
if t.retrieveEngine != nil {
    results, totalCount, searchErr = t.searchChunksMilvus(ctx, patterns, kbIDs, allowedKnowledgeIDs, maxResults)
} else {
    results, totalCount, searchErr = t.searchChunks(ctx, patterns, kbIDs, allowedKnowledgeIDs, kbTenantMap)
}
```

**新增 `searchChunksMilvus()` 方法：**

- 遍历所有 patterns，逐一调用 `retrieveEngine.Retrieve()` + `RetrieverType: KeywordsRetrieverType`
- 结果按 `ChunkID` 去重，保留最高 BM25 分数
- 通过 `knowledgeSvc` 批量查询 `KnowledgeTitle`（未提供时显示 `Untitled`）
- 按分数降序排列，截取 `maxResults` 条

### 2. `agent_service.go`

**结构体新增字段：**

```go
milvusRetrieveEngine interfaces.RetrieveEngine
```

**构造函数新增参数：**

```go
func NewAgentService(
    // ...原有参数...
    retrieveEngineRegistry interfaces.RetrieveEngineRegistry, // 新增
) interfaces.AgentService
```

启动时从 registry 提取 Milvus engine（nil-safe，未配置时为 nil）：

```go
var milvusRetrieveEngine interfaces.RetrieveEngine
if retrieveEngineRegistry != nil {
    if svc, err := retrieveEngineRegistry.GetRetrieveEngineService(types.MilvusRetrieverEngineType); err == nil && svc != nil {
        milvusRetrieveEngine = svc
    }
}
```

**注册侧变更：**

```go
// 改前
case tools.ToolGrepChunks:
    toolToRegister = tools.NewGrepChunksTool(s.db, config.SearchTargets)

// 改后
case tools.ToolGrepChunks:
    toolToRegister = tools.NewGrepChunksTool(s.db, s.milvusRetrieveEngine, s.knowledgeService, config.SearchTargets)
```

### 3. `repository.go`（Milvus）

**新增 `collectionLacksBM25Function()` 辅助方法：**

通过 `DescribeCollection` 获取 collection schema，检查是否有 `FunctionTypeBM25` 类型的 function 绑定。

**`ensureCollection()` 新增自动迁移逻辑：**

```go
if hasCollection {
    if needsRecreate, _ := m.collectionLacksBM25Function(ctx, collectionName); needsRecreate {
        // 打 WARN 日志，drop 旧 collection，hasCollection = false
        // 继续往下用新 schema 重建
    }
}
```

---

## 旧版 Collection 迁移说明

> ⚠️ **重要**：如果你的 Milvus collection 是旧版本创建的（服务启动日志中会出现如下警告），collection 内所有向量数据将被清空，需要重新索引文档。

**迁移触发日志：**
```
[WARN] Collection weknora_1536 is missing BM25 function — dropping and recreating. All vectors will be lost and must be re-indexed.
```

**迁移后操作：**
1. 服务启动完成后，进入 WeKnora 管理后台
2. 对所有知识库执行"重新索引"操作，或通过 API 批量触发文档重新入库
3. 确认 Milvus collection 中出现 `content_sparse` 数据后，BM25 搜索即可正常工作

---

## 降级兼容

当 Milvus **未配置**（`retrieveEngineRegistry` 为 nil 或获取失败）时：
- `milvusRetrieveEngine` 为 nil
- `grep_chunks` 自动回退到原 PostgreSQL LIKE 逻辑
- 无任何功能影响，完全向后兼容

---

## 验证方法

1. 查看启动日志，确认出现：
   ```
   Registered grep_chunks tool with searchTargets: N targets (milvusBM25=true)
   ```
2. 在 Agent 对话中使用 `grep_chunks` 工具搜索关键词，观察返回结果的相关性是否优于之前
3. 检查 Milvus collection schema 确认 `text_bm25_emb` function 已绑定：
   ```
   Functions: [{Name: text_bm25_emb, Type: BM25, InputFields: [content], OutputFields: [content_sparse]}]
   ```
