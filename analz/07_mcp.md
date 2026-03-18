# WeKnora MCP 能力详解

## 1. 概述

WeKnora 的 MCP（Model Context Protocol）能力允许 Agent 通过标准化协议接入第三方外部工具服务。MCP 是 Anthropic 提出的开放协议，WeKnora 实现了完整的 **MCP 客户端**，支持 SSE 和 HTTP Streamable 两种传输方式，通过连接池管理多个 MCP 服务的生命周期。

---

## 2. 数据库表结构

### 2.1 mcp_services 表（PostgreSQL）

```sql
-- migrations/versioned/000001_agent.up.sql + 000017_mcp_builtin.up.sql
CREATE TABLE IF NOT EXISTS mcp_services (
    id              VARCHAR(36)   PRIMARY KEY,        -- UUID
    tenant_id       INTEGER       NOT NULL,            -- 多租户隔离
    name            VARCHAR(255)  NOT NULL,            -- 服务显示名称
    description     TEXT,                             -- 服务描述
    enabled         BOOLEAN       DEFAULT true,        -- 启用/禁用开关
    transport_type  VARCHAR(50)   NOT NULL,            -- 传输类型: sse/http-streamable/stdio
    url             VARCHAR(512),                     -- 服务 URL（SSE/HTTP Streamable）
    headers         JSONB,                            -- 自定义 HTTP 头
    auth_config     JSONB,                            -- 认证配置（API Key/Bearer Token）
    advanced_config JSONB,                            -- 高级配置（超时/重试）
    stdio_config    JSONB,                            -- Stdio 配置（command/args）
    env_vars        JSONB,                            -- Stdio 环境变量
    is_builtin      BOOLEAN       NOT NULL DEFAULT false, -- 是否为内置服务（对所有租户可见）
    created_at      TIMESTAMP     DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP     DEFAULT CURRENT_TIMESTAMP,
    deleted_at      TIMESTAMP                         -- 软删除
);

-- 索引
CREATE INDEX idx_mcp_services_tenant_id ON mcp_services(tenant_id);
CREATE INDEX idx_mcp_services_enabled ON mcp_services(enabled);
CREATE INDEX idx_mcp_services_deleted_at ON mcp_services(deleted_at);
CREATE INDEX idx_mcp_services_is_builtin ON mcp_services(is_builtin);
```

**多租户隔离规则**：
- 普通服务：`WHERE tenant_id = ?` - 只对本租户可见
- 内置服务（`is_builtin = true`）：`WHERE tenant_id = ? OR is_builtin = true` - 对所有租户可见
- 内置服务的敏感信息（URL、AuthConfig、Headers、EnvVars、StdioConfig）不向租户暴露（`HideSensitiveInfo()` 方法）

### 2.2 JSONB 字段结构

**headers（MCPHeaders）**：
```json
{
  "X-Custom-Header": "value",
  "Accept": "application/json"
}
```

**auth_config（MCPAuthConfig）**：
```json
{
  "api_key": "sk-xxxx",
  "token": "bearer-token",
  "custom_headers": {"X-Auth-Extra": "value"}
}
```

**advanced_config（MCPAdvancedConfig）**：
```json
{
  "timeout": 30,       // 超时秒数，默认 30s，最大 60s
  "retry_count": 3,    // 重试次数，默认 3
  "retry_delay": 1     // 重试间隔秒数，默认 1s
}
```

**stdio_config（MCPStdioConfig）**（仅 stdio 传输，当前已禁用）：
```json
{
  "command": "uvx",    // 命令：uvx 或 npx
  "args": ["mcp-server-example", "--port", "3000"]
}
```

---

## 3. Go 数据结构

### 3.1 MCPService 主结构

```go
// internal/types/mcp.go
type MCPService struct {
    ID             string           `json:"id"              gorm:"primaryKey"`
    TenantID       uint64           `json:"tenant_id"       gorm:"index"`
    Name           string           `json:"name"`
    Description    string           `json:"description"`
    Enabled        bool             `json:"enabled"         gorm:"default:true;index"`
    TransportType  MCPTransportType `json:"transport_type"` // sse/http-streamable/stdio
    URL            *string          `json:"url,omitempty"`  // SSE/HTTP 端点
    Headers        MCPHeaders       `json:"headers"         gorm:"type:json"`
    AuthConfig     *MCPAuthConfig   `json:"auth_config"     gorm:"type:json"`
    AdvancedConfig *MCPAdvancedConfig `json:"advanced_config" gorm:"type:json"`
    StdioConfig    *MCPStdioConfig  `json:"stdio_config"    gorm:"type:json"`
    EnvVars        MCPEnvVars       `json:"env_vars"        gorm:"type:json"`
    IsBuiltin      bool             `json:"is_builtin"      gorm:"default:false"`
    CreatedAt      time.Time        `json:"created_at"`
    UpdatedAt      time.Time        `json:"updated_at"`
    DeletedAt      gorm.DeletedAt   `json:"deleted_at"      gorm:"index"`
}
```

**GORM 钩子**：
```go
func (m *MCPService) BeforeCreate(tx *gorm.DB) error {
    if m.ID == "" {
        m.ID = uuid.New().String()  // 自动生成 UUID
    }
    return nil
}
```

### 3.2 传输类型常量

```go
const (
    MCPTransportSSE            MCPTransportType = "sse"             // Server-Sent Events
    MCPTransportHTTPStreamable MCPTransportType = "http-streamable" // HTTP Streamable（新标准）
    MCPTransportStdio          MCPTransportType = "stdio"           // 已禁用（安全原因）
)
```

---

## 4. MCP 客户端实现

### 4.1 MCPClient 接口

```go
// internal/mcp/client.go
type MCPClient interface {
    Connect(ctx context.Context) error                                                    // 建立连接
    Disconnect() error                                                                    // 断开连接
    Initialize(ctx context.Context) (*InitializeResult, error)                           // MCP 握手
    ListTools(ctx context.Context) ([]*types.MCPTool, error)                             // 获取工具列表
    ListResources(ctx context.Context) ([]*types.MCPResource, error)                     // 获取资源列表
    CallTool(ctx context.Context, name string, args map[string]interface{}) (*CallToolResult, error) // 调用工具
    ReadResource(ctx context.Context, uri string) (*ReadResourceResult, error)           // 读取资源
    IsConnected() bool                                                                   // 连接状态
    GetServiceID() string                                                                // 服务 ID
}
```

### 4.2 NewMCPClient 工厂函数

```go
func NewMCPClient(config *ClientConfig) (MCPClient, error) {
    // 1. 配置超时（默认 30s，可通过 AdvancedConfig.Timeout 覆盖）
    timeout := 30 * time.Second
    if config.Service.AdvancedConfig != nil && config.Service.AdvancedConfig.Timeout > 0 {
        timeout = time.Duration(config.Service.AdvancedConfig.Timeout) * time.Second
    }
    httpClient := &http.Client{Timeout: timeout}

    // 2. 构建请求头（自定义头 + 认证头）
    headers := map[string]string{}
    // 合并 service.Headers
    // 追加 AuthConfig.APIKey → X-API-Key
    // 追加 AuthConfig.Token → Authorization: Bearer ...
    // 追加 AuthConfig.CustomHeaders

    // 3. 根据传输类型创建客户端
    switch config.Service.TransportType {
    case MCPTransportSSE:
        mcpClient = client.NewSSEMCPClient(*service.URL,
            client.WithHTTPClient(httpClient),
            client.WithHeaders(headers))

    case MCPTransportHTTPStreamable:
        mcpClient = client.NewStreamableHttpClient(*service.URL,
            transport.WithHTTPBasicClient(httpClient),
            transport.WithHTTPHeaders(headers))

    case MCPTransportStdio:
        return nil, fmt.Errorf("stdio transport is disabled for security reasons")

    default:
        return nil, ErrUnsupportedTransport
    }

    // 4. 注册连接丢失回调
    mcpClient.OnConnectionLost(instance.onConnectionLost)
    return instance, nil
}
```

### 4.3 Initialize 握手流程

```go
func (c *mcpGoClient) Initialize(ctx context.Context) (*InitializeResult, error) {
    req := mcp.InitializeRequest{
        Params: mcp.InitializeParams{
            ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,  // 使用最新协议版本
            Capabilities:    mcp.ClientCapabilities{},
            ClientInfo: mcp.Implementation{
                Name:    "WeKnora",
                Version: "1.0.0",
            },
        },
    }

    result, err := c.client.Initialize(ctx, req)
    if err != nil {
        c.checkErrorAndDisconnectIfNeeded(err)  // SSE 会话失效时自动断开
        return nil, err
    }

    c.initialized = true
    return &InitializeResult{
        ProtocolVersion: result.ProtocolVersion,
        ServerInfo: ServerInfo{
            Name:    result.ServerInfo.Name,
            Version: result.ServerInfo.Version,
        },
    }, nil
}
```

### 4.4 CallTool 工具调用

```go
func (c *mcpGoClient) CallTool(ctx context.Context, name string, args map[string]interface{}) (*CallToolResult, error) {
    req := mcp.CallToolRequest{
        Params: mcp.CallToolParams{
            Name:      name,
            Arguments: args,
        },
    }

    result, err := c.client.CallTool(ctx, req)
    if err != nil {
        c.checkErrorAndDisconnectIfNeeded(err)
        return nil, err
    }

    // 内容类型转换：text → ContentItem{Type:"text"}, image → ContentItem{Type:"image"}
    content := []ContentItem{}
    for _, item := range result.Content {
        if textContent, ok := mcp.AsTextContent(item); ok {
            content = append(content, ContentItem{Type: "text", Text: textContent.Text})
        } else if imageContent, ok := mcp.AsImageContent(item); ok {
            content = append(content, ContentItem{Type: "image", Data: imageContent.Data, MimeType: imageContent.MIMEType})
        }
    }

    return &CallToolResult{IsError: result.IsError, Content: content}, nil
}
```

### 4.5 SSE 会话失效处理

```go
// SSE 传输特有问题：连接丢失不会主动触发 onConnectionLost（mark3labs/mcp-go 库问题）
// 解决方案：检测到 "Invalid session ID" 错误时主动断开连接，触发重连
func (c *mcpGoClient) checkErrorAndDisconnectIfNeeded(err error) {
    var transportErr *transport.Error
    if c.service.TransportType == types.MCPTransportSSE &&
        errors.As(err, &transportErr) &&
        strings.Contains(transportErr.Err.Error(), "Invalid session ID") {
        _ = c.Disconnect()
    }
}
```

---

## 5. MCPManager 连接池

### 5.1 MCPManager 结构

```go
// internal/mcp/manager.go
type MCPManager struct {
    clients   map[string]MCPClient  // serviceID → client（连接池）
    clientsMu sync.RWMutex          // 读写锁
    ctx       context.Context       // 管理器生命周期上下文
    cancel    context.CancelFunc
}
```

### 5.2 GetOrCreateClient 连接复用策略

```
GetOrCreateClient(service)
    ↓
1. 检查 service.Enabled → false → 报错
2. 检查 service.TransportType → stdio → 报错（已禁用）
    ↓
3. 读锁：检查 clients[service.ID]
   ├── 存在且 IsConnected() → 直接返回（连接复用）
   └── 不存在或断开 → 继续
    ↓
4. 写锁：Double-Check（避免并发创建）
    ↓
5. NewMCPClient(config) → 创建新客户端
    ↓
6. client.Connect(m.ctx)  ← 使用 Manager 的长生命周期 context
   （SSE 连接需要持久 context，不能使用请求级 context）
    ↓
7. initializeClient(service, client)
   ├── initTimeout = min(AdvancedConfig.Timeout, 60s)，默认 30s
   ├── client.Initialize(initCtx) → MCP 握手
   └── 握手失败 → client.Disconnect()，返回错误
    ↓
8. clients[service.ID] = client（缓存）
    ↓
返回已连接并初始化的 client
```

### 5.3 空闲连接清理（cleanupIdleConnections）

```go
func (m *MCPManager) cleanupIdleConnections() {
    ticker := time.NewTicker(5 * time.Minute)  // 每 5 分钟检查一次
    defer ticker.Stop()

    for {
        select {
        case <-m.ctx.Done():
            return
        case <-ticker.C:
            m.removeDisconnectedClients()  // 清除 IsConnected() == false 的客户端
        }
    }
}
```

**清理策略**：只清理已断开（`IsConnected() == false`）的客户端，不主动断开活跃连接。

### 5.4 完整生命周期管理

| 方法                         | 说明                                    |
| ---------------------------- | --------------------------------------- |
| `GetOrCreateClient(service)` | 获取或创建连接（惰性初始化 + 连接复用） |
| `GetClient(serviceID)`       | 仅获取已有连接（不创建）                |
| `CloseClient(serviceID)`     | 关闭并移除指定连接                      |
| `CloseAll()`                 | 关闭所有连接                            |
| `Shutdown()`                 | 优雅关闭（取消 context + CloseAll）     |
| `GetActiveClients()`         | 获取活跃连接数                          |
| `ListActiveServices()`       | 列出所有活跃服务 ID                     |

---

## 6. MCP 服务接入 Agent

### 6.1 MCP 工具注入流程

当 Agent 配置中启用了 MCP 服务时：

```
Agent 执行请求
    ↓
[mcp_selection_mode == "all"]  → 加载租户所有启用的 MCP 服务
[mcp_selection_mode == "selected"] → 加载 mcp_services 列表中的 MCP 服务
[mcp_selection_mode == "none"] → 不加载 MCP 服务
    ↓
对每个 MCP 服务：
  mcpManager.GetOrCreateClient(service)
      ↓
  client.ListTools(ctx) → []*MCPTool
    ↓
将 MCP 工具注册到 toolRegistry：
  registry.Register(&MCPToolWrapper{
      serviceID: service.ID,
      tool:      mcpTool,
      client:    mcpClient,
  })
    ↓
buildToolsForLLM() → 包含 MCP 工具的 Function Definition
    ↓
LLM 可以直接调用 MCP 工具（如同内置工具）
```

### 6.2 MCP 工具执行

```
LLM 调用: tool_name = "mcp:{service_id}:{tool_name}"
    ↓
toolRegistry.ExecuteTool(ctx, name, args)
    ↓
MCPToolWrapper.Execute(ctx, args)
    ↓
mcpManager.GetOrCreateClient(service) → MCPClient
    ↓
client.CallTool(ctx, toolName, args)
    ↓
MCPServer 处理并返回结果
    ↓
CallToolResult{IsError, Content[]}
    ↓
将 Content 转为 ToolResult.Output（文本拼接）
```

---

## 7. REST API 接口

### 7.1 MCP 服务管理接口

| 方法   | 路径                                 | 说明                            |
| ------ | ------------------------------------ | ------------------------------- |
| GET    | `/api/v1/mcp-services`               | 获取租户所有 MCP 服务（含内置） |
| GET    | `/api/v1/mcp-services/:id`           | 获取单个 MCP 服务详情           |
| POST   | `/api/v1/mcp-services`               | 创建 MCP 服务                   |
| PUT    | `/api/v1/mcp-services/:id`           | 更新 MCP 服务配置               |
| DELETE | `/api/v1/mcp-services/:id`           | 删除 MCP 服务                   |
| POST   | `/api/v1/mcp-services/:id/test`      | 测试 MCP 服务连接               |
| GET    | `/api/v1/mcp-services/:id/tools`     | 获取 MCP 服务工具列表           |
| GET    | `/api/v1/mcp-services/:id/resources` | 获取 MCP 服务资源列表           |

### 7.2 MCPTestResult 响应结构

```go
type MCPTestResult struct {
    Success   bool           `json:"success"`
    Message   string         `json:"message,omitempty"` // 错误信息
    Tools     []*MCPTool     `json:"tools,omitempty"`   // 可用工具列表
    Resources []*MCPResource `json:"resources,omitempty"` // 可用资源列表
}
```

### 7.3 Repository 层多租户查询

```go
// GetByID：支持内置服务
WHERE id = ? AND (tenant_id = ? OR is_builtin = true)

// List：列出租户服务 + 内置服务
WHERE (tenant_id = ? OR is_builtin = true) ORDER BY created_at DESC

// ListEnabled：仅启用的服务
WHERE (tenant_id = ? OR is_builtin = true) AND enabled = true ORDER BY created_at DESC
```

---

## 8. MCPService 服务层接口

```go
// internal/types/interfaces/mcp_service.go
type MCPServiceService interface {
    CreateMCPService(ctx, service) error
    GetMCPServiceByID(ctx, tenantID, id) (*MCPService, error)
    ListMCPServices(ctx, tenantID) ([]*MCPService, error)
    ListMCPServicesByIDs(ctx, tenantID, ids) ([]*MCPService, error)
    UpdateMCPService(ctx, service) error
    DeleteMCPService(ctx, tenantID, id) error
    TestMCPService(ctx, tenantID, id) (*MCPTestResult, error)  // 测试连接 + 获取工具列表
    GetMCPServiceTools(ctx, tenantID, id) ([]*MCPTool, error)
    GetMCPServiceResources(ctx, tenantID, id) ([]*MCPResource, error)
}
```

**TestMCPService 流程**：
1. 从数据库加载服务配置
2. `mcpManager.GetOrCreateClient(service)` → 建立连接
3. `client.ListTools(ctx)` → 获取工具列表
4. `client.ListResources(ctx)` → 获取资源列表
5. 返回 `MCPTestResult{Success: true, Tools: [...], Resources: [...]}`

---

## 9. 前端 MCP 管理界面

### 9.1 TypeScript 接口定义

```typescript
// frontend/src/api/mcp-service.ts
export interface MCPService {
  id: string
  tenant_id?: number
  name: string
  description: string
  enabled: boolean
  transport_type: 'sse' | 'http-streamable' | 'stdio'
  url?: string
  headers?: Record<string, string>
  auth_config?: {
    api_key?: string
    token?: string
    custom_headers?: Record<string, string>
  }
  advanced_config?: {
    timeout?: number     // 超时秒数
    retry_count?: number // 重试次数
    retry_delay?: number // 重试间隔秒数
  }
  stdio_config?: {
    command: 'uvx' | 'npx'
    args: string[]
  }
  env_vars?: Record<string, string>
  is_builtin?: boolean
  created_at?: string
  updated_at?: string
}
```

### 9.2 前端 API 调用

```typescript
// 基础 CRUD
listMCPServices()                          // GET /api/v1/mcp-services
getMCPService(id)                          // GET /api/v1/mcp-services/:id
createMCPService(data)                     // POST /api/v1/mcp-services
updateMCPService(id, data)                 // PUT /api/v1/mcp-services/:id
deleteMCPService(id)                       // DELETE /api/v1/mcp-services/:id

// 连接测试（重要：用于配置验证）
testMCPService(id)                         // POST /api/v1/mcp-services/:id/test

// 工具和资源查询
getMCPServiceTools(id)                     // GET /api/v1/mcp-services/:id/tools
getMCPServiceResources(id)                 // GET /api/v1/mcp-services/:id/resources
```

---

## 10. MCP 协议通信流程

### 10.1 SSE 传输连接序列

```
WeKnora → MCP Server
    ↓
1. HTTP GET /sse    ← 建立 SSE 长连接（Server-Sent Events）
   ← endpoint: /messages?session_id=xxx   ← Server 返回消息端点

2. HTTP POST /messages?session_id=xxx
   Body: {"jsonrpc":"2.0","method":"initialize","params":{...},"id":1}
   ← Response: {"jsonrpc":"2.0","result":{...},"id":1}

3. HTTP POST /messages
   Body: {"jsonrpc":"2.0","method":"tools/list","params":{},"id":2}
   ← Response: {"jsonrpc":"2.0","result":{"tools":[...]},"id":2}

4. HTTP POST /messages
   Body: {"jsonrpc":"2.0","method":"tools/call","params":{"name":"xxx","arguments":{}},"id":3}
   ← Response: {"jsonrpc":"2.0","result":{"content":[...]},"id":3}
```

### 10.2 HTTP Streamable 传输（新标准）

```
WeKnora → MCP Server
    ↓
所有请求通过单一 HTTP 端点（POST），支持流式响应：

POST /mcp
Content-Type: application/json
Accept: text/event-stream (or application/json)

Body: {"jsonrpc":"2.0","method":"initialize","params":{...},"id":1}
← 流式响应（SSE）或单次 JSON 响应
```

**与 SSE 的区别**：
- SSE：需要保持长连接的 GET 请求 + 消息通过 POST 发送
- HTTP Streamable：单一端点处理所有请求，更简洁灵活

---

## 11. 安全设计

### 11.1 Stdio 传输禁用

```go
case types.MCPTransportStdio:
    return nil, fmt.Errorf("stdio transport is disabled for security reasons; " +
        "please use SSE or HTTP Streamable transport instead")
```

Stdio 传输涉及在服务器上执行外部命令（如 `uvx`, `npx`），存在**命令注入漏洞**风险，因此在生产环境中被完全禁用。

### 11.2 内置服务信息隐藏

```go
func (m *MCPService) HideSensitiveInfo() *MCPService {
    if !m.IsBuiltin {
        return m  // 非内置服务：不隐藏
    }
    copy := *m
    copy.URL = nil
    copy.AuthConfig = nil
    copy.Headers = nil
    copy.EnvVars = nil
    copy.StdioConfig = nil
    return &copy
}
```

内置 MCP 服务（由管理员配置）对租户用户隐藏敏感配置，租户只能使用服务功能，不能看到 URL 和认证信息。

### 11.3 连接超时控制

```go
// 初始化握手超时：最大 60s
initTimeout = min(service.AdvancedConfig.Timeout, 60s) || 30s

// HTTP 请求超时：
httpClient.Timeout = service.AdvancedConfig.Timeout || 30s
```

---

## 12. WeKnora 自带 MCP Server

WeKnora 项目附带了一个 Python 实现的 MCP Server（`mcp-server/`），使 WeKnora 知识库能够被其他支持 MCP 的 AI 应用（如 Claude Desktop、Cursor 等）使用。

```python
# mcp-server/weknora_mcp_server.py
# 对外暴露的 MCP 工具：
# - search_knowledge: 语义搜索知识库
# - list_knowledge_bases: 列出可用知识库
# - get_document: 获取文档详情
```

这使得 WeKnora 既是 MCP **客户端**（Agent 调用外部 MCP 服务），也是 MCP **服务端**（将知识库能力暴露给其他 AI 工具）。

---

## 13. 完整 MCP 调用链路图

```
用户查询（需要外部工具）
    ↓
AgentEngine.Execute()
    ↓
构建工具列表时加载 MCP 工具：
  ├── mcpManager.GetOrCreateClient(service1)
  │   ├── 检查连接池（hit → 直接返回）
  │   ├── 创建 SSE/HTTP Streamable 客户端
  │   ├── client.Connect(manager.ctx)
  │   └── client.Initialize() → MCP 握手
  ├── client.ListTools() → Tool1, Tool2, ...
  └── 注册到 toolRegistry
    ↓
LLM 决策使用 Tool1（来自外部 MCP 服务）
    ↓
toolRegistry.ExecuteTool("mcp:service1:Tool1", args)
    ↓
MCPToolWrapper.Execute(ctx, args)
    ↓
mcpManager.GetOrCreateClient(service1)  ← 复用已有连接
    ↓
client.CallTool(ctx, "Tool1", args)
    ↓
[SSE/HTTP Streamable] → MCP Server
    ↓
CallToolResult{IsError: false, Content: [{Type:"text", Text:"..."}]}
    ↓
ToolResult.Output = 拼接所有 text 内容
    ↓
EventBus.Emit(EventAgentToolResult)
    ↓
前端渲染工具调用结果
    ↓
LLM 下一轮推理（基于工具结果）
    ↓
final_answer → 完成
```

---

## 14. 关键配置参数汇总

| 参数             | 默认值          | 位置                        | 说明                   |
| ---------------- | --------------- | --------------------------- | ---------------------- |
| HTTP 连接超时    | 30s             | `AdvancedConfig.Timeout`    | MCP 服务 HTTP 请求超时 |
| 初始化握手超时   | 30s（最大 60s） | `AdvancedConfig.Timeout`    | Initialize 握手超时    |
| 空闲连接清理间隔 | 5 分钟          | `cleanupIdleConnections`    | 定期清理断连客户端     |
| 重试次数         | 3               | `AdvancedConfig.RetryCount` | 工具调用失败重试       |
| 重试间隔         | 1s              | `AdvancedConfig.RetryDelay` | 重试等待时间           |
| Stdio 传输       | 禁用            | 代码硬编码                  | 安全原因禁止           |
