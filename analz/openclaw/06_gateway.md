# OpenClaw Gateway：协议适配与路由分发

## 一、Gateway 定位

Gateway 是 OpenClaw 的核心中枢，负责：

- **多渠道集成**：统一管理 50+ 通讯平台的接入
- **路由分发**：将消息分发到正确的 Agent
- **会话治理**：维护对话状态和历史
- **状态同步**：保证系统一致性

```
OpenClaw = Gateway + Agents + Tools + Sessions
           ^^^^^^^^
           核心中枢
```

---

## 二、核心架构

### 端口与通信

```
默认端口: 18789 (WebSocket)
管理端口: 18790 (HTTP API)
```

### Gateway 组件

```
┌─────────────────────────────────────────────────────────────────┐
│                         Gateway                                  │
│                                                                  │
│  ┌──────────────┐ ┌──────────────┐ ┌──────────────┐            │
│  │  连接管理器   │ │  路由引擎    │ │  会话管理器   │            │
│  │ Connection   │ │   Router     │ │   Session    │            │
│  │  Manager     │ │   Engine     │ │   Manager    │            │
│  └──────────────┘ └──────────────┘ └──────────────┘            │
│                                                                  │
│  ┌──────────────┐ ┌──────────────┐ ┌──────────────┐            │
│  │  协议适配器   │ │  配置热加载  │ │  健康监控    │            │
│  │  Protocol    │ │   Hot        │ │   Health     │            │
│  │  Adapters    │ │   Reload     │ │   Monitor    │            │
│  └──────────────┘ └──────────────┘ └──────────────┘            │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

---

## 三、协议适配

### 统一消息模型

不同平台的消息格式各异，Gateway 通过协议适配器将它们统一为 `MsgContext`：

```typescript
interface MsgContext {
    // 消息正文
    Body: string;
    BodyForAgent?: string;      // 给 Agent 看的正文
    BodyForCommands?: string;   // 命令解析用正文
    RawBody?: string;           // 原始消息体
    
    // 会话标识
    SessionKey: string;         // 格式: {agentId}:{scope}
    
    // 来源信息
    Provider: string;           // 来源平台
    Surface?: string;           // 界面类型
    ChatType?: "direct" | "group";
    
    // 发送者信息
    SenderId?: string;
    SenderName?: string;
    SenderUsername?: string;
    
    // 渠道信息
    OriginatingChannel?: string;
    OriginatingTo?: string;
    AccountId?: string;
    
    // 线程/消息ID
    MessageThreadId?: string;
    MessageSid?: string;
    
    // 权限标记
    CommandAuthorized?: boolean;
    GatewayClientScopes?: string[];
}
```

### 平台适配器

每个外部平台都有专属适配器：

```javascript
// 适配器接口
interface ChannelAdapter {
    name: string;
    normalize(rawMessage: any): MsgContext;
    denormalize(response: AgentResponse): PlatformMessage;
    getCapabilities(): string[];
}

// 钉钉适配器示例
class DingTalkAdapter implements ChannelAdapter {
    name = 'dingtalk';
    
    normalize(msg) {
        return {
            Body: msg.content,
            SessionKey: `default:${msg.conversationId}`,
            Provider: 'dingtalk',
            ChatType: msg.conversationType === '1' ? 'direct' : 'group',
            SenderId: msg.senderId,
            SenderName: msg.senderNick
        };
    }
    
    denormalize(resp) {
        return {
            msgtype: 'text',
            text: { content: resp.content }
        };
    }
    
    getCapabilities() {
        return ['markdown', 'image', 'actionCard'];
    }
}
```

### 支持的平台

| 平台     | 适配器   | 特性支持             |
| -------- | -------- | -------------------- |
| 钉钉     | dingtalk | Markdown、卡片、图片 |
| 飞书     | feishu   | 富文本、卡片、文件   |
| 企业微信 | wework   | Markdown、图片       |
| Telegram | telegram | 全功能               |
| WhatsApp | whatsapp | 全功能               |
| Discord  | discord  | 全功能               |
| Slack    | slack    | 全功能               |
| Web      | web      | 全功能               |
| CLI      | cli      | 基础功能             |

---

## 四、路由分发

### 路由流程

```
消息进入 → 协议适配 → dispatchInboundMessage 
→ 去重检查 → 已处理过? 
    → 是: 丢弃 
    → 否: 路由匹配 → 确定目标Agent → 获取会话锁 → 执行Agent
```

### 去重机制

```javascript
const IDEMPOTENCY_TTL = 20 * 60 * 1000;  // 20分钟

async function checkIdempotency(msgContext) {
    const key = `${ctx.Provider}:${ctx.MessageSid}:${ctx.SenderId}`;
    
    const exists = await cache.get(`idempotency:${key}`);
    if (exists) {
        return false;  // 跳过处理
    }
    
    await cache.set(`idempotency:${key}`, '1', IDEMPOTENCY_TTL);
    return true;
}
```

### 路由规则

```javascript
const routingRules = {
    // Web 内部通道：直接指定 sessionKey
    web: {
        type: 'direct',
        sessionKeyPattern: '{agentId}:{scope}'
    },
    
    // 外部通道：绑定规则匹配
    external: {
        type: 'binding',
        rules: [
            {
                agentId: 'work',
                match: { channel: 'whatsapp', accountId: 'work' }
            },
            {
                agentId: 'life',
                match: { channel: 'whatsapp', accountId: 'personal' }
            }
        ]
    }
};
```

### 绑定配置

```yaml
# config.yaml
agents:
  list:
    - id: work
      workspace: ~/.openclaw/workspace-work
    - id: life
      workspace: ~/.openclaw/workspace-life

bindings:
  - agentId: work
    match:
      channel: whatsapp
      accountId: work
      
  - agentId: life
    match:
      channel: whatsapp
      accountId: personal
```

---

## 五、会话管理

### 会话车道机制

保证同一 `sessionKey` 下的消息**串行执行**：

```javascript
class SessionLane {
    constructor() {
        this.locks = new Map();
    }
    
    async process(sessionKey, message) {
        if (!this.locks.has(sessionKey)) {
            this.locks.set(sessionKey, new Mutex());
        }
        
        const lock = this.locks.get(sessionKey);
        
        await lock.runExclusive(async () => {
            await this.executeAgent(sessionKey, message);
        });
    }
}
```

### 全局并发控制

```javascript
class ConcurrencyManager {
    constructor(maxConcurrent = 100) {
        this.semaphore = new Semaphore(maxConcurrent);
        this.activeSessions = new Map();
    }
    
    async acquire(sessionKey) {
        await this.semaphore.acquire();
        this.activeSessions.set(sessionKey, {
            startTime: Date.now()
        });
    }
    
    release(sessionKey) {
        this.activeSessions.delete(sessionKey);
        this.semaphore.release();
    }
    
    getStatus() {
        return {
            active: this.activeSessions.size,
            available: this.semaphore.available
        };
    }
}
```

---

## 六、渠道插件生命周期

### 插件管理器

```javascript
class ChannelPluginManager {
    constructor() {
        this.plugins = new Map();
        this.registry = new PluginRegistry();
    }
    
    async register(plugin) {
        this.validatePlugin(plugin);
        await plugin.initialize();
        this.plugins.set(plugin.name, plugin);
        this.registry.add(plugin);
    }
    
    async unregister(pluginName) {
        const plugin = this.plugins.get(pluginName);
        if (plugin) {
            await plugin.shutdown();
            this.plugins.delete(pluginName);
        }
    }
    
    async reload(pluginName) {
        await this.unregister(pluginName);
        const newPlugin = await this.loadPlugin(pluginName);
        await this.register(newPlugin);
    }
}
```

### 插件生命周期

```
加载插件 → 验证配置 → 初始化 → 注册 → 运行 
    ↑                          ↓
    └──── 收到更新? ← 热重载 ←─┘
                    ↓
                  关闭
```

---

## 七、消息流入处理

### monitor-inbox.ts

```javascript
class InboxMonitor {
    constructor() {
        this.deduplicator = new Deduplicator();
        this.debouncer = new Debouncer();
    }
    
    async process(rawMessage) {
        // 1. 解析消息
        const parsed = this.parseMessage(rawMessage);
        
        // 2. 去重
        if (this.deduplicator.isDuplicate(parsed)) {
            return;
        }
        
        // 3. 防抖
        await this.debouncer.wait(parsed.sessionKey, 100);
        
        // 4. 分发处理
        await this.dispatch(parsed);
    }
}
```

---

## 八、聊天 RPC 接口

### chat.ts

```javascript
const chatRPC = {
    // 获取历史消息
    getHistory: async (sessionKey, options) => {
        const { limit = 20, before } = options;
        return await sessionManager.getHistory(sessionKey, limit, before);
    },
    
    // 发送消息
    send: async (sessionKey, message) => {
        return await gateway.send(sessionKey, message);
    },
    
    // 中止执行
    abort: async (sessionKey) => {
        return await gateway.abort(sessionKey);
    },
    
    // 获取状态
    getStatus: async (sessionKey) => {
        return await sessionManager.getStatus(sessionKey);
    }
};
```

---

## 九、配置热加载

### 配置管理

```javascript
class ConfigManager {
    constructor(configPath) {
        this.configPath = configPath;
        this.config = null;
        this.watchers = [];
    }
    
    async load() {
        const content = await fs.readFile(this.configPath, 'utf8');
        this.config = yaml.parse(content);
        return this.config;
    }
    
    watch(callback) {
        const watcher = fs.watch(this.configPath, async (event) => {
            if (event === 'change') {
                const newConfig = await this.load();
                callback(newConfig);
            }
        });
        this.watchers.push(watcher);
    }
    
    async hotReload() {
        const newConfig = await this.load();
        this.emit('configChanged', newConfig);
        await this.applyConfig(newConfig);
    }
}
```

---

## 十、健康监控

### 健康检查端点

```javascript
app.get('/health', async (req, res) => {
    const health = {
        status: 'ok',
        timestamp: Date.now(),
        components: {
            gateway: await checkGateway(),
            agents: await checkAgents(),
            channels: await checkChannels(),
            storage: await checkStorage()
        }
    };
    
    const allHealthy = Object.values(health.components)
        .every(c => c.status === 'ok');
    
    res.status(allHealthy ? 200 : 503).json(health);
});
```

### 监控指标

```javascript
const metrics = {
    activeConnections: () => connectionManager.count(),
    activeSessions: () => sessionManager.count(),
    messageRate: () => messageCounter.rate(),
    errorRate: () => errorCounter.rate(),
    latency: () => latencyHistogram.percentile(99)
};
```

---

## 十一、与 WeKnora 对比

| 维度     | OpenClaw    | WeKnora            |
| -------- | ----------- | ------------------ |
| 核心框架 | Node.js     | Go (Gin)           |
| 协议适配 | 插件化      | 中间件模式         |
| 路由机制 | 绑定规则    | 动态路由           |
| 会话管理 | 内存 + 文件 | Redis + PostgreSQL |
| 热加载   | 文件监听    | 配置中心           |
| 多租户   | 无          | 原生支持           |

### WeKnora Gateway 增强

```go
type Gateway struct {
    router       *gin.Engine
    sessionMgr   *SessionManager
    tenantMgr    *TenantManager
    mcpManager   *MCPManager
}

func (g *Gateway) tenantMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        tenantID := extractTenantID(c)
        ctx := context.WithTenant(c, tenantID)
        c.Set("ctx", ctx)
        c.Next()
    }
}
```
