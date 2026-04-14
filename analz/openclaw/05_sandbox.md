# OpenClaw 沙盒安全：三层隔离与权限控制

## 一、安全挑战

OpenClaw 赋予 AI 直接操作系统的能力，这带来了巨大的安全风险：

### 核心风险

| 风险类型   | 描述                      | 危害等级 |
| ---------- | ------------------------- | -------- |
| 提示注入   | 恶意指令隐藏在网页/文档中 | 严重     |
| 权限滥用   | AI 执行超出预期的操作     | 高       |
| 数据泄露   | 敏感数据被发送到外部      | 高       |
| 供应链攻击 | 恶意 Skill 植入后门       | 严重     |
| 远程控制   | 暴露实例被攻击者接管      | 严重     |

### 已披露漏洞

```
CVE-2026-25253: Gateway 控制平面漏洞
+ 7 个 CVE: RCE、命令注入、SSRF、认证绕过、路径遍历
+ 独立审计: 512 个漏洞，8 个严重级别
```

---

## 二、三层隔离模型

### 架构图

```
┌─────────────────────────────────────────────────────────────────┐
│                        宿主系统                                  │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │                     第1层：进程隔离                          │  │
│  │  ┌─────────────────────────────────────────────────────┐  │  │
│  │  │                第2层：容器隔离                         │  │  │
│  │  │  ┌───────────────────────────────────────────────┐  │  │  │
│  │  │  │              第3层：资源限制                     │  │  │  │
│  │  │  │  ┌─────────────────────────────────────────┐  │  │  │  │
│  │  │  │  │          技能执行环境                      │  │  │  │  │
│  │  │  │  │      (Skill Execution Context)           │  │  │  │  │
│  │  │  │  └─────────────────────────────────────────┘  │  │  │  │
│  │  │  │           内存限制 | CPU限制 | 网络限制         │  │  │  │
│  │  │  └───────────────────────────────────────────────┘  │  │  │
│  │  │                   Docker 容器                        │  │  │
│  │  └─────────────────────────────────────────────────────┘  │  │
│  │                      独立进程                              │  │
│  └───────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
```

### 第1层：进程隔离

每个技能执行在独立进程中：

```javascript
async function executeInProcess(command, args) {
    return new Promise((resolve, reject) => {
        const child = spawn(command, args, {
            detached: true,    // 独立进程组
            stdio: ['ignore', 'pipe', 'pipe']
        });
        
        // 超时控制
        const timeout = setTimeout(() => {
            process.kill(-child.pid);  // 杀死整个进程组
            reject(new Error('Execution timeout'));
        }, EXECUTION_TIMEOUT);
        
        child.on('close', (code) => {
            clearTimeout(timeout);
            resolve(code);
        });
    });
}
```

### 第2层：容器隔离

使用 Docker 容器隔离执行环境：

```javascript
const containerConfig = {
    Image: 'openclaw-sandbox:latest',
    
    // 资源限制
    HostConfig: {
        Memory: 256 * 1024 * 1024,     // 256MB 内存
        MemorySwap: 256 * 1024 * 1024, // 禁用 swap
        CpuQuota: 50000,               // 50% CPU
        CpuPeriod: 100000,
        PidsLimit: 100,                // 最大进程数
        
        // 文件系统
        ReadonlyRootfs: true,          // 只读根文件系统
        Binds: [
            '/tmp/sandbox:/tmp:rw'     // 限制写入目录
        ],
        
        // 网络隔离
        NetworkMode: 'none',           // 禁用网络
        
        // 安全选项
        SecurityOpt: [
            'no-new-privileges',       // 禁止提权
        ],
        
        // 用户权限
        User: '1000:1000'              // 非root用户
    }
};
```

### 第3层：资源限制

细粒度的资源控制：

```javascript
const resourceLimits = {
    // 内存限制
    memory: {
        max: '256MB',
        swap: 0,
        swappiness: 0
    },
    
    // CPU限制
    cpu: {
        quota: 50000,      // 50% of 100ms
        period: 100000,
        shares: 512
    },
    
    // I/O限制
    io: {
        readBps: 10 * 1024 * 1024,   // 10MB/s
        writeBps: 10 * 1024 * 1024,
    },
    
    // 网络限制
    network: {
        enabled: false,
        allowedHosts: [],
        blockedPorts: [22, 23, 25, 445, 3389]
    },
    
    // 进程限制
    process: {
        maxPids: 100,
        maxOpenFiles: 1024,
        maxThreads: 50
    }
};
```

---

## 三、权限控制系统

### 权限分级

```javascript
const permissionLevels = {
    // 低风险 - 自动执行
    low: {
        actions: ['read_file', 'search', 'web_fetch'],
        autoApprove: true,
        logLevel: 'info'
    },
    
    // 中等风险 - 通知用户
    medium: {
        actions: ['write_file', 'send_message', 'create_document'],
        autoApprove: false,
        notifyUser: true,
        logLevel: 'warn'
    },
    
    // 高风险 - 需要确认
    high: {
        actions: ['execute_shell', 'modify_system', 'install_package'],
        requireConfirmation: true,
        timeout: 300000,  // 5分钟确认超时
        logLevel: 'error'
    },
    
    // 关键操作 - 二次验证
    critical: {
        actions: ['modify_auth', 'delete_user', 'change_password'],
        require2FA: true,
        requireAdminApproval: true,
        logLevel: 'critical'
    }
};
```

### 用户审批流程

```
技能执行请求 → 权限检查
    → 低风险: 自动执行
    → 中风险: 通知用户 → 记录日志
    → 高风险: 等待确认 → 用户确认? 
        → 是: 执行
        → 否: 拒绝
        → 超时: 取消
    → 关键操作: 二次验证 → 验证通过?
        → 是: 执行
        → 否: 拒绝
```

---

## 四、后台任务管理

### 后台任务类型

| 类型     | 说明     | 管理                |
| -------- | -------- | ------------------- |
| 短任务   | 秒级完成 | 同步执行            |
| 长任务   | 分钟级   | 后台运行 + 进度报告 |
| 守护任务 | 持续运行 | 监控 + 自动重启     |
| 定时任务 | 周期执行 | Cron 调度           |

### 进程管理

```javascript
class ProcessManager {
    constructor() {
        this.processes = new Map();
        this.maxProcesses = 10;
    }
    
    // 启动后台进程
    async startBackground(command, options) {
        if (this.processes.size >= this.maxProcesses) {
            throw new Error('Maximum background processes reached');
        }
        
        const process = spawn(command, options);
        const id = generateId();
        
        this.processes.set(id, {
            process,
            command,
            startTime: Date.now(),
            status: 'running'
        });
        
        return id;
    }
    
    // 列出所有进程
    listProcesses() {
        return Array.from(this.processes.entries()).map(([id, p]) => ({
            id,
            command: p.command,
            status: p.status,
            uptime: Date.now() - p.startTime
        }));
    }
    
    // 终止进程
    async killProcess(id) {
        const p = this.processes.get(id);
        if (p && p.process) {
            process.kill(-p.process.pid);
            this.processes.delete(id);
        }
    }
}
```

---

## 五、安全边界设计

### 边界检查

```javascript
const securityChecks = [
    // 1. 命令白名单
    {
        name: 'command_whitelist',
        check: (cmd) => {
            const allowed = ['ls', 'cat', 'grep', 'find', 'node', 'python'];
            return allowed.some(c => cmd.startsWith(c));
        }
    },
    
    // 2. 路径限制
    {
        name: 'path_restriction',
        check: (path) => {
            const forbidden = ['/etc', '/root', '/home', '~/.ssh'];
            return !forbidden.some(p => path.startsWith(p));
        }
    },
    
    // 3. 网络限制
    {
        name: 'network_restriction',
        check: (host) => {
            const blocked = ['localhost', '127.0.0.1', '0.0.0.0'];
            return !blocked.some(h => host.match(h));
        }
    },
    
    // 4. 敏感文件检测
    {
        name: 'sensitive_file',
        check: (content) => {
            const patterns = [
                /password\s*=\s*.+/i,
                /api[_-]?key\s*=\s*.+/i,
                /secret\s*=\s*.+/i
            ];
            return !patterns.some(p => p.test(content));
        }
    }
];
```

### 防护策略

```javascript
const securityPolicy = {
    // 1. 提示注入防护
    promptInjection: {
        enabled: true,
        detectPatterns: [
            /ignore previous instructions/i,
            /disregard all above/i,
            /you are now/i
        ]
    },
    
    // 2. 权限提升防护
    privilegeEscalation: {
        enabled: true,
        preventSudo: true,
        preventSetuid: true
    },
    
    // 3. 数据泄露防护
    dataExfiltration: {
        enabled: true,
        blockExternalRequests: true,
        maxUploadSize: 1024 * 1024  // 1MB
    },
    
    // 4. 资源耗尽防护
    resourceExhaustion: {
        enabled: true,
        maxMemory: '256MB',
        maxCpu: '50%',
        maxTime: 60000
    }
};
```

---

## 六、审计与日志

### 日志记录

```javascript
const auditLog = {
    // 记录所有执行
    logExecution: async (event) => {
        await db.insert('audit_log', {
            timestamp: Date.now(),
            agentId: event.agentId,
            action: event.action,
            params: sanitizeParams(event.params),
            result: event.result,
            riskLevel: event.riskLevel,
            duration: event.duration,
            userApproved: event.userApproved
        });
    }
};
```

### 安全告警

```javascript
const securityAlerts = {
    // 异常行为检测
    detectAnomalies: (recentEvents) => {
        const alerts = [];
        
        // 1. 高频执行
        if (recentEvents.length > 100) {
            alerts.push({ type: 'high_frequency', severity: 'medium' });
        }
        
        // 2. 权限升级尝试
        if (recentEvents.some(e => e.action.includes('sudo'))) {
            alerts.push({ type: 'privilege_escalation', severity: 'high' });
        }
        
        // 3. 外部通信尝试
        if (recentEvents.some(e => e.action.includes('curl'))) {
            alerts.push({ type: 'external_communication', severity: 'high' });
        }
        
        return alerts;
    }
};
```

---

## 七、最佳实践建议

### 部署建议

```
1. 使用独立环境
   - 隔离的服务器或虚拟机
   - 不要在开发机或生产服务器上直接运行

2. 网络隔离
   - 使用防火墙限制入站连接
   - 禁止直接暴露到公网

3. 最小权限
   - 使用非 root 用户运行
   - 只授予必要的文件系统访问权限

4. 定期更新
   - 关注安全公告
   - 及时更新到最新版本

5. 监控审计
   - 启用完整审计日志
   - 定期检查异常行为
```

### 安全配置

```yaml
security:
  dmPolicy: "pairing"           # 需要配对才能访问
  shellExecution: "confirm"      # Shell 执行需要确认
  networkAccess: "restricted"    # 限制网络访问
  
  sandbox:
    enabled: true
    memory: "256MB"
    cpu: "50%"
    timeout: 60000
    
  audit:
    enabled: true
    retention: 30d
    sensitiveLog: false
```

---

## 八、与 WeKnora 对比

| 维度     | OpenClaw | WeKnora      |
| -------- | -------- | ------------ |
| 隔离层数 | 3 层     | 3 层（兼容） |
| 容器技术 | Docker   | Docker       |
| 权限分级 | 4 级     | 细粒度 RBAC  |
| 审批机制 | 用户确认 | 多级审批流   |
| 审计日志 | 文件存储 | PostgreSQL   |

### WeKnora 增强

```go
// WeKnora 的多级审批流
type ApprovalFlow struct {
    Steps []ApprovalStep
}

type ApprovalStep struct {
    Level       string    // "manager" | "admin" | "2fa"
    Timeout     time.Duration
    AutoEscalate bool
}

// 企业级安全策略
type EnterpriseSecurityPolicy struct {
    TenantIsolation   bool     // 租户隔离
    DataClassification []string // 数据分类
    ComplianceMode    string   // "GDPR" | "SOC2" | "HIPAA"
}
```
