# WeKnora 整体架构与技术栈深度分析

## 1. 项目概述

WeKnora 是腾讯开源的企业级知识库 + Agent 平台，提供完整的 RAG（检索增强生成）流程、多种 Agent 推理框架、文档智能解析、Skills 扩展能力和 MCP（Model Context Protocol）集成。核心代码托管在 `github.com/Tencent/WeKnora`。

---

## 2. 整体系统架构

```
┌─────────────────────────────────────────────────────────────────┐
│                        前端 (Vue 3 + Vite)                        │
│   Chat界面 │ 知识库管理 │ Agent配置 │ 模型管理 │ MCP管理 │ 组织管理  │
└──────────────────────────┬──────────────────────────────────────┘
                           │ HTTP / SSE
┌──────────────────────────▼──────────────────────────────────────┐
│                   Go 后端 (Gin HTTP Server)                       │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────────────┐ │
│  │ 认证中间件 │  │  Router  │  │  Handler  │  │ Swagger API Docs │ │
│  └──────────┘  └──────────┘  └──────────┘  └──────────────────┘ │
│                                                                   │
│  ┌───────────────────────────────────────────────────────────┐   │
│  │                    应用服务层 (Service)                      │   │
│  │ KnowledgeService │ SessionService │ AgentService │ ...     │   │
│  └───────────────────────────────────────────────────────────┘   │
│                                                                   │
│  ┌────────────┐  ┌────────────┐  ┌──────────┐  ┌─────────────┐  │
│  │ ChatPipline│  │AgentEngine │  │MCPManager│  │SkillsManager│  │
│  │  (RAG流程) │  │(ReAct循环) │  │(MCP客户端)│  │(技能管理器)  │  │
│  └────────────┘  └────────────┘  └──────────┘  └─────────────┘  │
│                                                                   │
│  ┌───────────────────── 模型层 ──────────────────────────────┐   │
│  │ Chat（OpenAI兼容）│ Embedding │ Rerank │ VLM │ Provider注册 │  │
│  └────────────────────────────────────────────────────────────┘  │
└──────────────────────────┬──────────────────────────────────────┘
                           │
     ┌─────────────────────┼──────────────────────────┐
     │                     │                          │
┌────▼────┐       ┌────────▼────────┐       ┌────────▼────────┐
│PostgreSQL│       │向量数据库(可选)  │       │   Redis         │
│(GORM)   │       │Milvus/Qdrant/   │       │(Asynq任务队列)   │
│         │       │Weaviate/        │       │                  │
│+pgvector│       │Elasticsearch    │       │                  │
│+pg_search│      └─────────────────┘       └──────────────────┘
└─────────┘
     │
┌────▼────────────────────────────────────────┐
│         docreader 微服务 (Python/gRPC)         │
│  PDF│DOCX│DOC│Excel│Markdown│图片│Web解析     │
│  MinerU高精度PDF解析引擎                        │
└────────────────────────────────────────────┘
```

---

## 3. 技术栈详细清单

### 3.1 后端（Go）

| 组件           | 技术/库                           | 版本    | 说明                               |
| -------------- | --------------------------------- | ------- | ---------------------------------- |
| 语言           | Go                                | 1.24.11 | 模块：`github.com/Tencent/WeKnora` |
| HTTP框架       | gin-gonic/gin                     | v1.11.0 | RESTful API + SSE流式响应          |
| 跨域           | gin-contrib/cors                  | v1.7.5  | CORS中间件                         |
| ORM            | gorm.io/gorm                      | v1.30.0 | 支持 PostgreSQL/MySQL/SQLite       |
| 数据库驱动     | gorm.io/driver/postgres           | v1.5.11 | 主数据库PostgreSQL                 |
| 数据库驱动     | gorm.io/driver/mysql              | v1.5.6  | 可选MySQL支持                      |
| 数据库驱动     | gorm.io/driver/sqlite             | v1.6.0  | 用于DuckDB/轻量场景                |
| 迁移工具       | golang-migrate/migrate            | v4.19.1 | SQL版本化迁移                      |
| 向量扩展       | pgvector/pgvector-go              | v0.3.0  | PostgreSQL向量检索                 |
| LLM SDK        | sashabaranov/go-openai            | v1.40.5 | OpenAI兼容API客户端                |
| MCP            | mark3labs/mcp-go                  | v0.43.0 | Model Context Protocol客户端       |
| Milvus         | milvus-io/milvus/client/v2        | v2.6.2  | 向量数据库客户端                   |
| Qdrant         | qdrant/go-client                  | v1.16.1 | 向量数据库客户端                   |
| Weaviate       | weaviate/weaviate-go-client/v5    | v5.5.0  | 向量数据库客户端                   |
| Elasticsearch  | elastic/go-elasticsearch/v8       | v8.18.0 | 全文检索                           |
| Redis          | redis/go-redis/v9                 | v9.14.0 | 任务队列 + 缓存                    |
| 任务队列       | hibiken/asynq                     | v0.25.1 | 基于Redis的异步任务                |
| 对象存储       | aws/aws-sdk-go-v2/service/s3      | v1.83.0 | S3/MinIO兼容存储                   |
| 对象存储       | minio/minio-go/v7                 | v7.0.91 | MinIO原生客户端                    |
| 腾讯云存储     | tencentyun/cos-go-sdk-v5          | v0.7.65 | 腾讯云COS                          |
| 火山引擎存储   | volcengine/ve-tos-golang-sdk      | v2.7.23 | 火山引擎TOS                        |
| 知识图谱       | neo4j/neo4j-go-driver/v6          | v6.0.0  | Neo4j图数据库                      |
| DuckDB         | duckdb/duckdb-go/v2               | v2.5.4  | 数据分析SQL引擎                    |
| SQLite-Vec     | asg017/sqlite-vec-go-bindings     | v0.1.6  | 轻量级向量检索                     |
| Ollama         | ollama/ollama                     | v0.11.4 | 本地模型推理                       |
| JWT认证        | golang-jwt/jwt/v5                 | v5.3.0  | 用户身份验证                       |
| 配置管理       | spf13/viper                       | v1.20.1 | 支持YAML/ENV配置                   |
| 日志           | sirupsen/logrus                   | v1.9.3  | 结构化日志                         |
| 链路追踪       | go.opentelemetry.io/otel          | v1.38.0 | OpenTelemetry追踪                  |
| gRPC           | google.golang.org/grpc            | v1.78.0 | 与docreader微服务通信              |
| 协程池         | panjf2000/ants/v2                 | v2.11.3 | 并发处理池                         |
| 并发工具       | golang.org/x/sync                 | v0.19.0 | errgroup等并发原语                 |
| 中文分词       | yanyiwu/gojieba                   | v1.4.5  | 中文关键词提取                     |
| 浏览器自动化   | chromedp/chromedp                 | v0.14.2 | 网页抓取                           |
| HTML转Markdown | JohannesKaufmann/html-to-markdown | v2.5.0  | 网页内容处理                       |
| JSON Schema    | google/jsonschema-go              | v0.4.2  | JSON schema验证                    |
| DI容器         | go.uber.org/dig                   | v1.18.1 | 依赖注入                           |
| API文档        | swaggo/gin-swagger                | v1.6.1  | Swagger文档生成                    |

### 3.2 前端（Vue 3）

| 组件         | 技术/库                             | 说明                   |
| ------------ | ----------------------------------- | ---------------------- |
| 框架         | Vue 3 + TypeScript                  | Composition API        |
| 构建工具     | Vite                                | 快速开发+生产构建      |
| UI库         | TDesign Vue Next (tdesign-vue-next) | 腾讯TDesign组件库      |
| 路由         | Vue Router 4                        | SPA路由                |
| 状态管理     | Pinia                               | 替代Vuex的现代状态管理 |
| HTTP客户端   | Axios                               | REST API请求           |
| 图标         | TDesign Icons + SVG inline          | 图标支持               |
| 国际化       | Vue I18n                            | 多语言支持             |
| Markdown渲染 | marked / markdown-it                | 消息内容渲染           |
| 代码高亮     | highlight.js                        | 代码块高亮             |
| 图表         | ECharts                             | 数据可视化             |
| 富文本       | tiptap / codemirror                 | 内容编辑               |

### 3.3 文档解析微服务（Python）

| 组件       | 技术/库                   | 说明                   |
| ---------- | ------------------------- | ---------------------- |
| 语言       | Python 3.x                | docreader/目录         |
| gRPC服务   | grpcio + proto            | 与Go后端通信           |
| 包管理     | uv (pyproject.toml)       | 现代Python包管理       |
| PDF解析    | pdfplumber + pymupdf      | 原生PDF解析            |
| DOCX解析   | python-docx + docx2       | Word文档解析           |
| Excel解析  | openpyxl / xlrd           | 表格解析               |
| 图片OCR    | tesseract / paddleocr     | 图片文字识别           |
| Markdown   | mistune                   | Markdown解析           |
| 高精度PDF  | MinerU (magic-pdf)        | 腾讯高精度PDF解析引擎  |
| DOC转换    | antiword + LibreOffice    | 旧版DOC格式转换        |
| 网页解析   | beautifulsoup4 / requests | 网页内容抓取           |
| MarkItDown | Microsoft MarkItDown      | 微软通用文档转Markdown |

### 3.4 基础设施

| 组件       | 技术                                       | 说明               |
| ---------- | ------------------------------------------ | ------------------ |
| 容器化     | Docker + Docker Compose                    | 多服务编排         |
| 数据库     | PostgreSQL (主)                            | 业务数据存储       |
| 向量扩展   | pgvector + pg_search (ParadeDB BM25)       | 向量+全文检索      |
| 缓存/队列  | Redis                                      | 异步任务队列       |
| 对象存储   | MinIO / S3 / COS / TOS                     | 文件存储           |
| 可选向量库 | Milvus / Qdrant / Weaviate / Elasticsearch | 外置向量数据库     |
| 知识图谱   | Neo4j                                      | 实体关系存储       |
| 数据分析   | DuckDB                                     | 嵌入式分析数据库   |
| 监控       | OpenTelemetry                              | 分布式追踪         |
| API文档    | Swagger/OpenAPI                            | 接口文档           |
| K8s部署    | Helm Charts                                | Kubernetes部署配置 |

---

## 4. 目录结构解析

```
WeKnora/
├── cmd/server/main.go          # 服务入口点
├── config/                     # 配置文件（config.yaml + prompt模板）
├── internal/                   # 核心业务逻辑
│   ├── agent/                  # Agent推理引擎
│   │   ├── engine.go           # ReAct循环核心（35KB）
│   │   ├── prompts.go          # 系统提示词模板（22KB）
│   │   ├── const.go            # 默认常量配置
│   │   ├── skills/             # Skills技能管理
│   │   └── tools/              # Agent工具集（19个工具文件）
│   ├── application/service/    # 应用服务层
│   │   ├── agent_service.go    # Agent服务（19KB）
│   │   ├── knowledge.go        # 知识库核心服务（294KB！最大文件）
│   │   ├── session.go          # 会话服务（67KB）
│   │   ├── knowledgebase.go    # 知识库管理（50KB）
│   │   ├── chat_pipline/       # RAG Pipeline插件体系（21个文件）
│   │   └── retriever/          # 检索引擎实现
│   ├── handler/                # HTTP处理器（路由层）
│   ├── mcp/                    # MCP客户端和管理器
│   ├── models/                 # 模型适配层
│   │   ├── chat/               # 对话模型（OpenAI兼容）
│   │   ├── embedding/          # 向量模型
│   │   ├── rerank/             # 重排序模型
│   │   ├── provider/           # 20+服务商注册
│   │   └── vlm/                # 视觉语言模型
│   ├── types/                  # 核心数据类型定义（29个类型文件）
│   ├── middleware/             # HTTP中间件（认证/日志/限流等）
│   ├── router/                 # 路由注册
│   └── container/              # 依赖注入容器
├── migrations/versioned/       # 数据库迁移脚本（21个版本）
├── docreader/                  # Python文档解析微服务
├── frontend/src/               # Vue 3前端
│   ├── views/chat/             # 对话界面（含AgentStreamDisplay等）
│   ├── views/knowledge-base/   # 知识库管理界面
│   └── api/                    # API调用层
├── mcp-server/                 # Python版MCP服务器（对外暴露WeKnora能力）
├── skills/preloaded/           # 预置Skills技能
├── client/                     # Go SDK客户端库
└── docker-compose.custom.yml   # 生产部署配置
```

---

## 5. 数据库整体表结构

WeKnora 使用 PostgreSQL 作为主数据库，共维护 **21 个版本的迁移文件**（`migrations/versioned/`）。

### 5.1 核心业务表一览

| 表名                   | 说明               | 关键字段                                                  |
| ---------------------- | ------------------ | --------------------------------------------------------- |
| `tenants`              | 租户（多租户隔离） | `api_key`, `agent_config`, `storage_quota`                |
| `users`                | 用户账号           | `username`, `email`, `password_hash`, `tenant_id`         |
| `auth_tokens`          | 认证令牌           | `token`, `token_type`, `expires_at`, `is_revoked`         |
| `knowledge_bases`      | 知识库             | `embedding_model_id`, `chunking_config`, `type`           |
| `knowledges`           | 知识文档           | `file_type`, `parse_status`, `file_path`, `file_hash`     |
| `chunks`               | 文本切片           | `content`, `chunk_index`, `chunk_type`, `parent_chunk_id` |
| `embeddings`           | 向量嵌入           | `embedding halfvec`, `source_id`, `dimension`             |
| `sessions`             | 会话               | `agent_id`, `agent_config`, `context_config`              |
| `messages`             | 消息记录           | `agent_steps`, `knowledge_references`                     |
| `models`               | 模型配置           | `type`, `source`, `parameters(JSONB)`                     |
| `knowledge_tags`       | 知识标签           | `name`, `color`, `knowledge_base_id`                      |
| `mcp_services`         | MCP服务配置        | `transport_type`, `url`, `auth_config`                    |
| `custom_agents`        | 自定义Agent        | `config(JSONB)`, `is_builtin`, `avatar`                   |
| `organizations`        | 组织（跨租户协作） | `invite_code`, `require_approval`, `member_limit`         |
| `organization_members` | 组织成员           | `role`, `tenant_id`                                       |
| `kb_shares`            | 知识库共享         | `knowledge_base_id`, `organization_id`, `permission`      |
| `agent_shares`         | Agent共享          | `agent_id`, `organization_id`, `permission`               |

### 5.2 embeddings 表向量索引

```sql
-- 向量列类型为 halfvec（半精度浮点，节省存储）
embedding halfvec

-- BM25全文检索索引（ParadeDB pg_search扩展）
CREATE INDEX embeddings_search_idx ON embeddings
USING bm25 (id, knowledge_base_id, content, knowledge_id, chunk_id)
WITH (text_fields = '{"content": {"tokenizer": {"type": "chinese_lindera"}}}');

-- 多维度HNSW向量索引
CREATE INDEX embeddings_embedding_idx_3584 ON embeddings
USING hnsw ((embedding::halfvec(3584)) halfvec_cosine_ops)
WITH (m = 16, ef_construction = 64) WHERE (dimension = 3584);

CREATE INDEX embeddings_embedding_idx_798 ON embeddings
USING hnsw ((embedding::halfvec(798)) halfvec_cosine_ops)
WITH (m = 16, ef_construction = 64) WHERE (dimension = 798);
```

---

## 6. 多租户架构

WeKnora 采用数据库级多租户隔离：
- 所有业务表均有 `tenant_id` 字段，服务层所有查询强制带 `tenant_id` 过滤
- `tenants` 表维护每个租户的 API Key（Bearer Token认证）
- `tenants.storage_quota`：默认 10GB 存储配额（字节）
- `tenants.agent_config`（JSONB）：租户级 Agent 默认配置
- `sessions.id_seq` 从 10000 开始，避免租户数据碰撞

---

## 7. 服务间通信

### 7.1 Go后端 ↔ docreader（Python微服务）
- 协议：**gRPC** (`google.golang.org/grpc v1.78.0`)
- Proto定义位于：`docreader/proto/`
- 用途：文档解析任务（PDF/DOCX/图片等解析、切片）

### 7.2 Go后端 ↔ MCP服务
- 协议：**HTTP SSE** 或 **HTTP Streamable**（mark3labs/mcp-go库）
- 注意：出于安全原因，**Stdio传输已被禁用**

### 7.3 Go后端 ↔ LLM 模型
- 协议：**HTTP/HTTPS**（OpenAI兼容 REST API）
- 流式：Server-Sent Events（SSE）
- 本地模型：Ollama HTTP API

---

## 8. 部署架构

### Docker Compose 服务组成（docker-compose.custom.yml）

| 服务名      | 镜像                | 说明                              |
| ----------- | ------------------- | --------------------------------- |
| `app`       | `:local`            | Go 后端 + Vue 前端（Nginx）       |
| `docreader` | `:local`            | Python 文档解析微服务             |
| `postgres`  | `paradedb/paradedb` | PostgreSQL + pgvector + pg_search |
| `redis`     | `redis:alpine`      | 任务队列 + 缓存                   |

### 可选外部服务（不在 compose 中）
- Milvus / Qdrant / Weaviate / Elasticsearch（向量数据库）
- Neo4j（知识图谱）
- MinIO / COS / TOS（对象存储）
- MinerU（高精度PDF解析）

---

## 9. 认证与安全

- **JWT Token**：用户登录后颁发 `access_token` + `refresh_token`
- **API Key**：租户级 API Key，用于外部调用（Bearer Token）
- **中间件链**：`RequestID` → `Logger` → `Cors` → `Auth` → `TenantResolver` → `Handler`
- **MCP安全**：Stdio传输已禁用，防止命令注入；SSE/HTTP Streamable 传输仅允许经认证的服务

---

## 10. 异步任务体系

基于 **Redis + Asynq**：
- 文档解析任务（Document Parse Task）：上传文档后异步触发解析流程
- 文档 Embedding 任务：解析完成后异步触发向量化
- 知识图谱提取任务（可选）
- 文档摘要生成任务

任务状态通过 WebSocket/SSE 实时推送到前端，展示解析进度。

---

## 11. 链路追踪

使用 **OpenTelemetry** 标准：
- 支持 OTLP gRPC/HTTP 导出器
- 支持 stdout 导出（开发调试）
- Pipeline 中所有关键步骤均有 Trace Span，包括：
  - 检索过程（embedding查询、关键词查询、rerank）
  - Agent工具调用（think/execute/observe）
  - LLM调用（token统计、延迟）
