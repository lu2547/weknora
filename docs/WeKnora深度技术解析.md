# 腾讯开源 WeKnora：你的本地私有 NotebookLM

> 比 IMA 更懂你的文档，比 RAGFlow 更好扩展的企业级 AI 知识库平台
>
> GitHub：https://github.com/Tencent/WeKnora | 官网：https://weknora.weixin.qq.com | 协议：MIT License | 版本：v0.3.3

---

## 目录

- [Part 1：部署与使用](#part-1部署与使用)
  - [1.1 WeKnora 是什么？](#11-weknora-是什么)
  - [1.2 五分钟本地部署](#12-五分钟本地部署)
  - [1.3 知识库配置与使用](#13-知识库配置与使用)
  - [1.4 Agent 模式使用](#14-agent-模式使用)
  - [1.5 接入 MCP（Cursor / Claude Desktop）](#15-接入-mcpcursor--claude-desktop)
  - [1.6 开发者快速模式](#16-开发者快速模式)
- [Part 2：架构与核心流程](#part-2架构与核心流程)
  - [2.1 整体系统架构](#21-整体系统架构)
  - [2.2 RAG Pipeline 完整流程](#22-rag-pipeline-完整流程)
  - [2.3 ReAct Agent 执行流程](#23-react-agent-执行流程)
  - [2.4 文档解析异步流程](#24-文档解析异步流程)
  - [2.5 MCP 双向集成架构](#25-mcp-双向集成架构)
  - [2.6 技术栈全景](#26-技术栈全景)
  - [2.7 数据模型设计](#27-数据模型设计)
- [总结](#总结)

---

# Part 1：部署与使用

## 1.1 WeKnora 是什么？

> 你有没有想过，有一天公司内部知识库可以像聊天一样被问答，复杂报告可以被 AI 代劳分析，连外部工具都能被 Agent 自动调用——而这一切，数据永不出内网？腾讯开源的 WeKnora，正在做这件事。

**WeKnora** 是腾讯开源的企业级文档理解与 AI 检索框架。如果你用过 Google 的 NotebookLM 或腾讯自家的 IMA，可以把 WeKnora 理解成**可私有化部署、完全掌控数据、深度可定制**的「本地版 NotebookLM」。

它的核心能力：**上传你的文档，问它任何问题，Agent 会自主检索、推理、调用工具，直到给你一个有据可查的答案。**

| 场景             | 具体用法                                        | 核心价值               |
| ---------------- | ----------------------------------------------- | ---------------------- |
| 🏢 企业知识库问答 | 上传内部手册、规范文档，员工用自然语言查询      | 降低沟通成本，知识沉淀 |
| 📝 合同/法规分析  | 上传合同 PDF，提问关键条款，标注引用来源        | 提升合规效率           |
| 📊 数据分析报告   | 上传 CSV/Excel，Agent 自动写 SQL 分析并生成图表 | 替代初级分析师工作     |
| 🔬 学术研究辅助   | 上传论文集，跨文档检索关联结论                  | 加速文献综述           |
| 🎧 产品技术支持   | 搭建产品手册知识库，对接客服系统                | 降低支持成本           |

---

## 1.2 五分钟本地部署

### 环境要求

| 必须安装                | 推荐配置                      |
| ----------------------- | ----------------------------- |
| Docker + Docker Compose | 16G 内存（含高精度 PDF 解析） |
| Git                     | GPU 可选（本地模型推理）      |
| 机器：4 核 8G 起步      | Ollama（本地模型服务）        |

### 图 1-1：WeKnora 本地部署流程

```mermaid
flowchart TD
    A([开始]) --> B[克隆仓库\ngit clone Tencent/WeKnora]
    B --> C[复制环境配置\ncp .env.example .env]
    C --> D{选择模型来源}
    D -->|本地 Ollama| E1[配置 EMBEDDING_BASE_URL\n= http://localhost:11434]
    D -->|云端 API| E2[配置 LLM_BASE_URL\n+ LLM_API_KEY]
    E1 --> F[选择启动模式]
    E2 --> F
    F -->|最小核心| G1[docker compose up -d]
    F -->|全功能| G2[docker-compose --profile full up -d]
    F -->|含知识图谱| G3[--profile neo4j --profile minio]
    G1 --> H[等待容器启动]
    G2 --> H
    G3 --> H
    H --> I{健康检查}
    I -->|PostgreSQL 就绪| J[访问 http://localhost]
    I -->|失败| K[查看日志\ndocker compose logs]
    K --> H
    J --> L[注册账号]
    L --> M[创建第一个知识库]
    M --> N([完成！])
```

### 关键环境变量说明

```bash
# ── 大模型配置（支持 OpenAI 兼容接口）──────────────────────────
LLM_BASE_URL=https://api.deepseek.com/v1      # 或 Qwen、本地 Ollama
LLM_API_KEY=sk-xxxxxxxxxxxxxxxxxxxx

# ── Embedding 模型（本地 Ollama 最省钱）───────────────────────
EMBEDDING_BASE_URL=http://localhost:11434
EMBEDDING_MODEL=nomic-embed-text              # 或 bge-m3

# ── 数据库（Docker Compose 已内置，通常无需改动）──────────────
POSTGRES_DB=weknora
POSTGRES_USER=weknora
POSTGRES_PASSWORD=your_strong_password

# ── 对象存储（默认本地存储，可选 MinIO/COS/TOS）──────────────
STORAGE_TYPE=local                            # local | minio | cos | tos
```

### 图 1-2：Docker Compose 服务拓扑

```mermaid
graph LR
    subgraph core["核心服务（默认启动）"]
        app["app 容器\nGo后端 + Vue前端(Nginx)\n:80 / :8080"]
        doc["docreader 容器\nPython文档解析微服务\ngRPC :50051"]
        pg["postgres 容器\nParadeDB\n:5432"]
        redis["redis 容器\n任务队列+缓存\n:6379"]
    end
    subgraph optional["可选服务（--profile）"]
        neo4j["Neo4j\n知识图谱\n:7474"]
        minio["MinIO\n对象存储\n:9000"]
        jaeger["Jaeger\n链路追踪\n:16686"]
        milvus["Milvus\n外置向量库\n:19530"]
    end
    app -->|gRPC| doc
    app -->|GORM| pg
    app -->|Asynq| redis
    app -.->|可选| neo4j
    app -.->|可选| minio
    app -.->|可选| jaeger
    app -.->|可选| milvus
```

---

## 1.3 知识库配置与使用

### 图 1-3：知识库创建与文档上传流程

```mermaid
sequenceDiagram
    actor User as 用户
    participant UI as Web UI
    participant API as Go 后端
    participant Queue as Redis 队列
    participant DocReader as docreader 微服务
    participant DB as PostgreSQL

    User->>UI: 创建知识库（选择类型：文档/FAQ）
    UI->>API: POST /api/v1/knowledge-bases
    API->>DB: 创建 knowledge_bases 记录
    API-->>UI: 返回知识库 ID

    User->>UI: 上传文档（拖拽/URL/文件夹）
    UI->>API: POST /api/v1/knowledges（multipart）
    API->>DB: 创建 knowledges 记录（parse_status=pending）
    API->>Queue: 推送解析任务（Asynq）
    API-->>UI: 上传成功，等待解析

    Queue->>DocReader: 触发解析任务（gRPC）
    DocReader->>DocReader: 解析文档（PDF/DOCX/图片/Excel...）
    DocReader-->>API: 返回结构化内容
    API->>DB: 写入 chunks 表（切片）
    API->>Queue: 推送 Embedding 任务
    Queue->>API: Embedding Worker 执行
    API->>API: 调用 Embedding 模型
    API->>DB: 写入 embeddings 表（向量+BM25）
    API->>DB: 更新 parse_status=completed
    API-->>UI: SSE 推送解析完成通知
    UI-->>User: 显示"解析完成"✓
```

### 支持的文档类型

| 类型     | 格式                | 解析引擎                          | 特殊能力             |
| -------- | ------------------- | --------------------------------- | -------------------- |
| PDF      | .pdf                | pdfplumber + MinerU（高精度模式） | 表格、公式、多栏布局 |
| Word     | .docx / .doc        | python-docx / LibreOffice         | 图片 OCR 提取        |
| 图片     | .png / .jpg / .webp | PaddleOCR / VLM 视觉描述          | 文字识别 + 图片描述  |
| 表格     | .xlsx / .csv        | openpyxl / DuckDB                 | Data Analyst 模式    |
| Markdown | .md                 | mistune                           | 保留标题层级         |
| 网页     | URL                 | ChromeDP + html2markdown          | 动态页面渲染         |

---

## 1.4 Agent 模式使用

WeKnora 提供两种对话模式，在输入框左侧可以切换：

| 模式                              | 说明                             | 适合场景                                     |
| --------------------------------- | -------------------------------- | -------------------------------------------- |
| ⚡ **快速回答（Quick Answer）**    | 单次 RAG 检索，毫秒级响应        | 明确的知识查询、FAQ 问答、简单信息检索       |
| 🧠 **深度推理（Smart Reasoning）** | ReAct 多轮工具调用（最多 20 轮） | 复杂多步骤推理、跨文档综合分析、数据分析报告 |

### Agent 可用的内置工具

| 工具名                  | 功能                   | 典型使用场景             |
| ----------------------- | ---------------------- | ------------------------ |
| `knowledge_search`      | 语义向量检索知识库     | 根据问题语义找相关内容   |
| `grep_chunks`           | 关键词精确匹配切片     | 精确查找特定术语、编号   |
| `list_knowledge_chunks` | 按 ID 读取完整切片内容 | 深度读取已定位的文档段落 |
| `get_document_content`  | 获取文档全文           | 需要完整阅读某份文档     |
| `query_knowledge_graph` | Neo4j 图谱关系查询     | 查询实体关联关系         |
| `web_search`            | 网络搜索（需启用）     | 知识库不足时补充外网信息 |
| `web_fetch`             | 抓取指定网页内容       | 获取特定 URL 页面        |
| `todo_write`            | 创建/更新任务计划      | 多步任务分解与跟踪       |
| `thinking`              | 链式思考记录           | 复杂推理中间过程可视化   |
| `execute_skill`         | 执行 Skills 技能       | 调用外部沙箱脚本         |

> 💡 在「Agent 管理」页面，你可以创建自定义 Agent，配置：模型选择、绑定知识库、允许使用的工具、最大迭代次数（默认 20）、是否开启反思机制、系统 Prompt 模板、RAG 检索阈值等。每个业务场景都可以有一套专属的 Agent 配置。

---

## 1.5 接入 MCP（Cursor / Claude Desktop）

### 图 1-4：WeKnora MCP 双向接入示意

```mermaid
graph TB
    subgraph external["外部 MCP 客户端"]
        cursor["Cursor IDE"]
        claude["Claude Desktop"]
        other["其他 MCP 客户端"]
    end
    subgraph weknora["WeKnora 系统"]
        mcpserver["mcp-server/\nPython MCP Server\n暴露 WeKnora 能力"]
        app["Go 后端 + Agent"]
        mcpclient["internal/mcp/\nGo MCP Client\n连接外部MCP服务"]
        kb["知识库 & 检索引擎"]
    end
    subgraph extmcp["外部 MCP 服务"]
        fs["文件系统 MCP"]
        db2["数据库 MCP"]
        custom["自定义 MCP 服务"]
    end

    cursor -->|HTTP SSE| mcpserver
    claude -->|HTTP SSE| mcpserver
    other -->|HTTP Streamable| mcpserver
    mcpserver --> app
    app --> kb

    app --> mcpclient
    mcpclient -->|HTTP SSE| fs
    mcpclient -->|HTTP SSE| db2
    mcpclient -->|HTTP SSE| custom
```

### 配置 Claude Desktop / Cursor 调用 WeKnora

```json
{
  "mcpServers": {
    "weknora": {
      "args": [
        "path/to/WeKnora/mcp-server/run_server.py"
      ],
      "command": "python",
      "env": {
        "WEKNORA_API_KEY": "sk-你的APIKey（在开发者工具请求头 x-api-key 中获取）",
        "WEKNORA_BASE_URL": "http://localhost:8080/api/v1"
      }
    }
  }
}
```

也可以 pip 安装后直接使用：

```bash
pip install weknora-mcp-server
python -m weknora-mcp-server
```

---

## 1.6 开发者快速模式

```bash
# 方法一：Make 命令（推荐）
make dev-start      # 启动 PostgreSQL + Redis 等基础设施
make dev-app        # 启动 Go 后端（Air 热重载，修改代码 5-10s 生效）
make dev-frontend   # 启动前端（Vite HMR 即时热更新）

# 方法二：一键启动
./scripts/quick-dev.sh
```

- ✅ 前端修改：Vite HMR 即时生效，无需刷新
- ✅ 后端修改：Air 热重载，5-10 秒重启
- ✅ 无需重新 build Docker 镜像
- ✅ 支持 IDE 断点调试（Go delve）

---

# Part 2：架构与核心流程

## 2.1 整体系统架构

### 图 2-1：WeKnora 整体系统架构图

```mermaid
graph TB
    subgraph frontend["前端层（Vue 3 + TypeScript + TDesign）"]
        chat["💬 Chat 对话界面"]
        kb_ui["📚 知识库管理"]
        agent_ui["🤖 Agent 配置"]
        model_ui["⚙️ 模型管理"]
        mcp_ui["🔌 MCP 管理"]
    end

    subgraph backend["Go 后端（Gin HTTP Server）"]
        direction TB
        subgraph middleware["中间件链"]
            mid["RequestID → Logger → CORS → Auth → TenantResolver"]
        end
        subgraph services["应用服务层（Service）"]
            s1["ChatPipeline\n(RAG流程·插件链)"]
            s2["AgentEngine\n(ReAct·最多20轮)"]
            s3["KnowledgeService\n(294KB·最大业务文件)"]
            s4["SessionService"]
        end
        subgraph models["模型层"]
            m1["Chat\n(OpenAI兼容)"]
            m2["Embedding\n(多Provider)"]
            m3["Rerank\n(重排序)"]
            m4["VLM\n(视觉语言)"]
        end
    end

    subgraph infra["基础设施层"]
        pg["PostgreSQL\n+ pgvector\n+ ParadeDB BM25"]
        redis["Redis\n+ Asynq\n任务队列"]
        storage["对象存储\nMinIO/COS/TOS/本地"]
        docreader["docreader\nPython 微服务\ngRPC"]
        neo4j["Neo4j\n知识图谱（可选）"]
        vector_ext["外置向量库（可选）\nMilvus/Qdrant/ES"]
    end

    frontend -->|HTTP / SSE 流式| backend
    backend -->|GORM| pg
    backend -->|Asynq| redis
    backend -->|gRPC| docreader
    backend -->|S3 API| storage
    backend -.->|可选| neo4j
    backend -.->|可选| vector_ext
    s1 --> m1
    s1 --> m2
    s1 --> m3
    s2 --> m1
```

---

## 2.2 RAG Pipeline 完整流程

WeKnora 的 RAG 流程采用**事件驱动的插件化责任链架构**，每个阶段是独立插件，通过 `EventManager` 串联，支持灵活替换和扩展。

### 图 2-2：RAG Pipeline 完整数据流（插件责任链）

```mermaid
flowchart TD
    UserQ(["👤 用户提问"])

    subgraph P1["插件1：PluginRewrite（查询重写）"]
        r1["获取最近 20 条历史消息"]
        r2["按 RequestID 分组 → 取最近5轮对话"]
        r3["LLM 重写\nTemperature=0.3, MaxTokens=50"]
        r1 --> r2 --> r3
    end

    subgraph P2["插件2：PluginSearch（混合检索·并发）"]
        s1["知识库检索（并发多KB）"]
        s2["网络搜索（可选）"]
        subgraph kb_search["知识库检索细节"]
            ks1{"文档 ≤50块?"}
            ks2["直接全量加载\nScore=1.0"]
            ks3["向量检索\nHNSW余弦相似度"]
            ks4["BM25关键词检索\n中文lindera分词"]
            ks5["合并去重\n按Score排序"]
            ks1 -->|是| ks2
            ks1 -->|否| ks3
            ks1 -->|否| ks4
            ks3 --> ks5
            ks4 --> ks5
        end
        s1 --> kb_search
        s2 -->|RAG压缩→临时知识库| ks5
    end

    subgraph P3["插件3：PluginRerank（重排序）"]
        rk1["清洗Passage\n去除Markdown/代码噪声"]
        rk2["增强Passage\n图片OCR + 生成问题"]
        rk3["调用Rerank模型API"]
        rk4["阈值过滤\n自适应降级（×0.7重试）"]
        rk5["综合评分\n0.6×模型分+0.3×检索分+0.1×来源权重"]
        rk6["MMR去重\nlambda=0.7，保证多样性"]
        rk1 --> rk2 --> rk3 --> rk4 --> rk5 --> rk6
    end

    subgraph P4["插件4：PluginMerge（切片合并）"]
        m1["ID+内容签名去重"]
        m2["子切片 → 父切片升级\n获取更完整上下文"]
        m3["按文档分组+ChunkIndex排序"]
        m4["合并相邻切片+扩展上下文窗口"]
        m1 --> m2 --> m3 --> m4
    end

    subgraph P5["插件5：PluginChatCompletion（生成）"]
        c1["渲染 Context 模板\n填充检索到的切片内容"]
        c2["构建消息历史\n(system+history+context+query)"]
        c3["流式调用 LLM\nSSE 推流给前端"]
        c1 --> c2 --> c3
    end

    QExp["🔄 查询扩展（可选）\n召回量不足时LLM生成多变体查询\n重新检索追加结果"]

    UserQ --> P1
    P1 --> P2
    P2 --> QExp
    QExp -.->|追加| P2
    P2 --> P3
    P3 --> P4
    P4 --> P5
    P5 --> Answer(["📝 流式答案返回用户"])
```

### 综合评分公式

| 权重      | 来源            | 说明                         |
| --------- | --------------- | ---------------------------- |
| **60%**   | Rerank 模型评分 | 语义相关性（最重要）         |
| **30%**   | 基础检索评分    | 向量余弦/BM25 原始分         |
| **10%**   | 来源权重        | 知识库(1.0) > 网络搜索(0.95) |
| **×乘数** | 位置先验        | 文档前段内容轻微加分 ±5%     |

---

## 2.3 ReAct Agent 执行流程

### 图 2-3：ReAct Agent 完整执行时序图

```mermaid
sequenceDiagram
    actor User as 用户
    participant CH as Chat Handler
    participant AE as AgentEngine
    participant LLM as LLM（流式）
    participant TR as 工具注册表
    participant KB as 知识库检索
    participant EB as EventBus
    participant UI as 前端 SSE

    User->>CH: 发送消息（smart-reasoning模式）
    CH->>AE: Execute(query, sessionID)
    AE->>AE: BuildSystemPromptWithOptions\n（注入KB列表+Skills元数据）
    AE->>AE: buildMessagesWithLLMContext\n（system+历史+当前query）
    AE->>EB: emit(session_start)
    EB->>UI: SSE推送

    loop ReAct 循环（最多20轮）
        Note over AE: Phase 1: THINK
        AE->>LLM: 流式推理（含所有工具定义）
        LLM-->>EB: 流式输出 agent_thought 事件
        EB-->>UI: SSE推送思考内容

        alt LLM 选择调用工具
            LLM-->>AE: 返回 ToolCalls[]
            EB->>UI: SSE推送 agent_tool_call(pending)

            Note over AE: Phase 3: ACT（并发执行工具）
            par 工具1
                AE->>TR: ExecuteTool(name, args)
                TR->>KB: 知识库检索/图谱查询等
                KB-->>TR: 返回结果
                TR-->>AE: ToolResult
            and 工具2
                AE->>TR: ExecuteTool(name2, args2)
                TR-->>AE: ToolResult2
            end

            opt Reflection 开启
                AE->>LLM: 评估工具结果（Temperature=0.5）
                LLM-->>EB: 流式输出 agent_reflection 事件
                EB-->>UI: SSE推送反思内容
            end

            Note over AE: Phase 4: OBSERVE
            AE->>AE: appendToolResults 写回上下文
            AE->>EB: emit agent_tool_result
            EB-->>UI: SSE推送工具结果（结构化数据）

        else LLM 调用 final_answer 工具
            LLM-->>AE: final_answer(content=...)
            AE->>EB: emit agent_final_answer（流式）
            EB-->>UI: SSE推送最终答案
            Note over AE: 跳出循环
        end
    end

    AE->>EB: emit agent_complete（含完整步骤+引用）
    EB-->>UI: SSE推送完成事件
    UI-->>User: 渲染答案+折叠执行步骤树
```

### 图 2-4：Agent 工具选择决策流程

```mermaid
flowchart TD
    Start(["开始一轮推理"]) --> Think["LLM 分析问题\n决定调用哪个工具"]
    Think --> TC{有 ToolCall?}
    TC -->|final_answer| Done(["输出最终答案\n结束循环"])
    TC -->|knowledge_search| KS["语义检索知识库\n→ 返回相关切片"]
    TC -->|grep_chunks| GC["关键词精确匹配\n→ 返回精确切片"]
    TC -->|list_knowledge_chunks| LC["按ID读取完整切片\n→ 深度读取（强制二步策略）"]
    TC -->|web_search| WS["网络搜索\n→ 压缩为知识"]
    TC -->|query_knowledge_graph| KG["Neo4j图谱查询\n→ 实体关系"]
    TC -->|todo_write| TW["更新任务计划\n→ 多步任务管理"]
    TC -->|execute_skill| ES["沙箱执行技能脚本\n→ 代码运行结果"]
    TC -->|无ToolCall超20轮| Force["强制生成答案"]

    KS --> Obs["结果写回上下文\nObserve阶段"]
    GC --> Obs
    LC --> Obs
    WS --> Obs
    KG --> Obs
    TW --> Obs
    ES --> Obs
    Force --> Done

    Obs --> NextRound["开始下一轮推理"]
    NextRound --> Think
```

---

## 2.4 文档解析异步流程

### 图 2-5：文档上传到向量化完整异步流程

```mermaid
flowchart TD
    Upload(["用户上传文档"]) --> GoBackend["Go 后端\n写入 knowledges 表\nparse_status = pending"]
    GoBackend --> PushTask["推送解析任务\nAsynq → Redis"]
    PushTask --> DocWorker["文档解析 Worker 消费"]

    DocWorker --> gRPC["通过 gRPC 调用\ndocreader 微服务"]

    subgraph docreader["docreader Python 微服务"]
        DetectType{"文件类型判断"}
        PDF["PDF Parser\npdfplumber/MinerU"]
        DOCX["Word Parser\npython-docx/LibreOffice"]
        IMG["图片 Parser\nPaddleOCR + VLM描述"]
        Excel["Excel Parser\nopenpyxl/xlrd"]
        MD["Markdown Parser\nmistune"]
        Web["Web Parser\nChrome + bs4"]
        Splitter["Splitter\n按标题层级+滑动窗口切片"]
        ParentChild["父子切片构建\n大段落→父 句子→子"]

        DetectType --> PDF & DOCX & IMG & Excel & MD & Web
        PDF & DOCX & IMG & Excel & MD & Web --> Splitter
        Splitter --> ParentChild
    end

    gRPC --> DetectType
    ParentChild --> SaveChunks["Go 后端\n写入 chunks 表"]
    SaveChunks --> PushEmbed["推送 Embedding 任务\nAsynq → Redis"]

    subgraph embedding["Embedding 向量化"]
        EmbedWorker["Embedding Worker\n并发度=5（ants协程池）"]
        EmbedModel["调用 Embedding 模型\n（Ollama/OpenAI兼容/阿里云...）"]
        HalfVec["halfvec 存储\nFP16 节省50%空间"]
        HNSW["HNSW 索引\nm=16, ef_construction=64"]
        BM25["BM25 全文索引\nParadeDB pg_search\nchinese_lindera中文分词"]
        EmbedWorker --> EmbedModel --> HalfVec
        HalfVec --> HNSW & BM25
    end

    PushEmbed --> EmbedWorker
    HNSW & BM25 --> Complete["更新 parse_status=completed"]
    Complete --> SSE["SSE 推送进度通知\n→ 前端显示'解析完成'✓"]
```

### 图 2-6：父子切片层级结构与检索升级机制

```mermaid
graph TB
    Doc["📄 原始文档"]
    P1["父切片1\n第一章：概述（完整段落）"]
    P2["父切片2\n第二章：技术细节（完整段落）"]
    C1["子切片1-1\n概述第一句"]
    C2["子切片1-2\n概述第二句 ⭐检索命中"]
    C3["子切片2-1\n技术细节段落A"]
    C4["子切片2-2\n技术细节段落B"]

    Doc --> P1 & P2
    P1 --> C1 & C2
    P2 --> C3 & C4

    C2 -->|"检索命中子切片\n自动升级为父切片"| Upgrade["返回父切片1\n获取完整上下文\n（更丰富语义）"]
```

---

## 2.5 MCP 双向集成架构

### 图 2-7：MCP 双向集成完整架构

```mermaid
flowchart LR
    subgraph outside["外部生态（MCP 客户端）"]
        claude["Claude Desktop"]
        cursor["Cursor IDE"]
        other["其他 MCP 工具"]
    end

    subgraph weknora_mcp["WeKnora MCP Server（Python）\nmcp-server/weknora_mcp_server.py"]
        direction TB
        t1["tenant_management 租户管理"]
        t2["knowledge_base_ops 知识库操作"]
        t3["semantic_search 语义检索"]
        t4["model_management 模型管理"]
    end

    subgraph weknora_core["WeKnora 核心（Go）"]
        agent_engine["AgentEngine (ReAct)"]
        mcp_manager["MCPManager\ninternal/mcp/"]
        kb_engine["知识库检索引擎"]
    end

    subgraph ext_mcp["外部 MCP 服务（可配置）"]
        fs_mcp["文件系统 MCP"]
        db_mcp["数据库 MCP"]
        custom_mcp["自定义 MCP 服务"]
    end

    outside -->|"HTTP SSE（Stdio已禁用·安全）"| weknora_mcp
    weknora_mcp -->|"REST API"| weknora_core
    agent_engine -->|"调用外部工具"| mcp_manager
    mcp_manager -->|"HTTP SSE"| fs_mcp & db_mcp & custom_mcp
```

> 🔒 **安全设计**：WeKnora 已禁用 MCP Stdio 传输（防止命令注入攻击），仅允许 HTTP SSE 和 HTTP Streamable 传输。工具注册采用 first-wins 策略防止同名工具覆盖攻击。

---

## 2.6 技术栈全景

### 图 2-8：WeKnora 完整技术栈

```mermaid
mindmap
  root((WeKnora 技术栈))
    后端 Go 1.24
      HTTP框架 Gin v1.11
      ORM GORM
        PostgreSQL 主数据库
        MySQL 可选
        SQLite 轻量
      LLM SDK go-openai v1.40
      MCP mark3labs/mcp-go
      任务队列 Asynq+Redis
      gRPC 微服务通信
      依赖注入 uber/dig
      JWT 认证
      OpenTelemetry 追踪
    向量存储
      pgvector halfvec FP16
      HNSW 索引 m16
      ParadeDB BM25
      Milvus 可选
      Qdrant 可选
      Weaviate 可选
      Elasticsearch 可选
    文档解析 Python
      pdfplumber 标准PDF
      MinerU 高精度PDF
      PaddleOCR 图片文字
      python-docx Word
      openpyxl Excel
      ChromeDP 网页
    前端 Vue 3
      TypeScript
      TDesign 腾讯组件库
      Vite 构建
      Pinia 状态管理
      ECharts 图表
      marked Markdown渲染
    知识图谱
      Neo4j 实体关系
      LLM实体抽取
      GraphRAG检索
    对象存储
      MinIO S3兼容
      腾讯云 COS
      火山引擎 TOS
      本地存储
    部署
      Docker Compose
      Helm K8s
      Air 热重载
```

---

## 2.7 数据模型设计

### 图 2-9：核心数据库表关系图（ER 图）

```mermaid
erDiagram
    tenants {
        int id PK
        string api_key
        jsonb agent_config
        bigint storage_quota
    }
    users {
        int id PK
        int tenant_id FK
        string username
        string email
        string password_hash
    }
    knowledge_bases {
        uuid id PK
        int tenant_id FK
        string name
        string type
        string embedding_model_id FK
        jsonb chunking_config
    }
    knowledges {
        uuid id PK
        uuid knowledge_base_id FK
        string file_type
        string parse_status
        string file_path
        string file_hash
    }
    chunks {
        uuid id PK
        uuid knowledge_id FK
        text content
        int chunk_index
        string chunk_type
        uuid parent_chunk_id
        jsonb image_info
    }
    embeddings {
        uuid id PK
        uuid chunk_id FK
        uuid knowledge_id FK
        uuid knowledge_base_id FK
        halfvec embedding
        int dimension
    }
    sessions {
        uuid id PK
        int tenant_id FK
        string agent_id FK
        jsonb agent_config
        jsonb context_config
    }
    messages {
        uuid id PK
        uuid session_id FK
        string role
        text content
        jsonb agent_steps
        jsonb knowledge_references
    }
    custom_agents {
        string id PK
        int tenant_id FK
        jsonb config
        bool is_builtin
    }
    mcp_services {
        uuid id PK
        int tenant_id FK
        string transport_type
        string url
        jsonb auth_config
    }

    tenants ||--o{ users : "has"
    tenants ||--o{ knowledge_bases : "owns"
    tenants ||--o{ sessions : "has"
    knowledge_bases ||--o{ knowledges : "contains"
    knowledges ||--o{ chunks : "split_into"
    chunks ||--o{ embeddings : "vectorized_as"
    sessions ||--o{ messages : "has"
    custom_agents ||--o{ sessions : "used_in"
```

### 图 2-10：多租户隔离与认证流程

```mermaid
flowchart LR
    Req(["HTTP 请求"])
    Req --> Mid1["RequestID 中间件\n生成唯一请求ID"]
    Mid1 --> Mid2["Logger 中间件\n结构化日志记录"]
    Mid2 --> Mid3["CORS 中间件\n跨域处理"]
    Mid3 --> Auth{"认证方式"}

    Auth -->|Bearer JWT Token| JWTAuth["JWT 验证\ngolang-jwt/v5\n→ 解析 user_id + tenant_id"]
    Auth -->|API Key| APIAuth["API Key 验证\n查询 tenants.api_key\n→ 解析 tenant_id"]

    JWTAuth --> TenantRes["TenantResolver 中间件\n注入 tenant_id 到 Context"]
    APIAuth --> TenantRes

    TenantRes --> Handler["业务 Handler"]
    Handler --> Service["Service 层\n所有查询强制带 WHERE tenant_id = ?"]
    Service --> DB[("PostgreSQL\n数据天然隔离")]
```

---

# 总结

WeKnora 是一个工程完成度相当高的开源项目。从架构设计来看，有几点特别值得关注：

**架构设计亮点：**
- RAG Pipeline 插件化责任链，可灵活扩展阶段
- ReAct Agent 事件驱动，全程 SSE 流式推送
- 文档解析独立微服务，gRPC 解耦，可单独扩容
- MCP 双向集成，既对外暴露能力，又能调用外部工具
- halfvec + HNSW 向量存储，节省 50% 空间
- PostgreSQL 一库搞定向量+全文检索，减少依赖

**与同类产品横向对比：**

| 维度       | WeKnora                   | RAGFlow    | Dify       |
| ---------- | ------------------------- | ---------- | ---------- |
| RAG 深度   | ⭐⭐⭐⭐⭐ 插件链+父子切片+MMR | ⭐⭐⭐⭐⭐      | ⭐⭐⭐⭐       |
| Agent 能力 | ⭐⭐⭐⭐⭐ ReAct+工具+反思     | ⭐⭐⭐        | ⭐⭐⭐⭐⭐      |
| MCP 集成   | ⭐⭐⭐⭐⭐ 双向集成            | ⭐⭐         | ⭐⭐⭐⭐       |
| 部署难度   | ⭐⭐⭐ Docker 一键           | ⭐⭐⭐        | ⭐⭐⭐        |
| 二次开发   | ⭐⭐⭐⭐⭐ Go+Vue 标准栈       | ⭐⭐⭐        | ⭐⭐⭐⭐       |
| 开源协议   | MIT（完全开源）           | Apache 2.0 | Apache 2.0 |

> 🌟 **项目地址**：https://github.com/Tencent/WeKnora
> **官网**：https://weknora.weixin.qq.com
> **协议**：MIT License
