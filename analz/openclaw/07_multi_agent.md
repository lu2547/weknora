# OpenClaw 多 Agent 协作：子 Agent 创建与任务分发

## 一、多 Agent 协作概述

OpenClaw 支持多 Agent 协作，主 Agent 可以创建子 Agent 来处理复杂任务，形成层级化的协作网络。

### 设计理念

```
单 Agent 模式：用户 → Agent → 结果
多 Agent 模式：用户 → 主Agent → 子Agent1 → 结果1
                            → 子Agent2 → 结果2
                            → 子Agent3 → 结果3
                            ← 汇总返回 ←
```

### 应用场景

| 场景       | 说明                                    |
| ---------- | --------------------------------------- |
| 任务分解   | 将复杂任务拆分为多个子任务并行执行      |
| 专业分工   | 不同 Agent 处理不同领域的问题           |
| 隔离执行   | 子 Agent 在独立环境中运行，互不干扰     |
| 工作流编排 | 主 Agent 作为协调者，编排多个专家 Agent |

---

## 二、多 Agent 架构

### 层级结构

```
                    ┌─────────────┐
                    │  主 Agent   │
                    │  (Master)   │
                    └──────┬──────┘
                           │
         ┌─────────────────┼─────────────────┐
         │                 │                 │
         v                 v                 v
    ┌─────────┐       ┌─────────┐       ┌─────────┐
    │子Agent 1│       │子Agent 2│       │子Agent 3│
    │(Worker) │       │(Worker) │       │(Worker) │
    └─────────┘       └─────────┘       └─────────┘
         │                 │                 │
         v                 v                 v
    ┌─────────┐       ┌─────────┐       ┌─────────┐
    │ 结果 1  │       │ 结果 2  │       │ 结果 3  │
    └─────────┘       └─────────┘       └─────────┘
         │                 │                 │
         └─────────────────┼─────────────────┘
                           v
                    ┌─────────────┐
                    │   结果汇总   │
                    └─────────────┘
```

### 资源隔离

每个 Agent 拥有独立的工作区：

```
workspace/
├── workspace-main/         # 主 Agent 工作区
│   ├── MEMORY.md
│   ├── sessions/
│   └── memory/
├── workspace-worker-1/     # 子 Agent 1 工作区
│   ├── MEMORY.md
│   └── sessions/
└── workspace-worker-2/     # 子 Agent 2 工作区
    ├── MEMORY.md
    └── sessions/
```

---

## 三、子 Agent 创建

### sessions_spawn 工具

主 Agent 通过 `sessions_spawn` 工具创建子 Agent：

```javascript
{
    name: "sessions_spawn",
    description: "创建一个新的子 Agent 来执行特定任务",
    parameters: {
        type: "object",
        properties: {
            task: {
                type: "string",
                description: "子 Agent 需要完成的任务描述"
            },
            skills: {
                type: "array",
                items: { type: "string" },
                description: "子 Agent 可以使用的技能列表"
            },
            workspace: {
                type: "string",
                description: "子 Agent 的工作区路径"
            },
            timeout: {
                type: "number",
                description: "超时时间（秒）"
            },
            context: {
                type: "string",
                description: "传递给子 Agent 的上下文信息"
            }
        },
        required: ["task"]
    }
}
```

### 创建流程

```
主Agent调用sessions_spawn → 参数验证 → 安全检查 → 检查通过?
    → 否: 返回错误
    → 是: 创建工作区 → 生成子Agent配置 → 启动子Agent进程 
        → 返回Agent ID → 异步执行任务
```

### 安全检查

```javascript
const spawnSecurityChecks = {
    // 1. 嵌套深度检查
    maxDepth: 3,  // 最多 3 层嵌套
    
    // 2. 并发数检查
    maxConcurrent: 5,  // 最多 5 个并发子 Agent
    
    // 3. 技能限制
    allowedSkills: (parentSkills) => {
        return parentSkills.filter(s => !s.restricted);
    },
    
    // 4. 超时限制
    maxTimeout: 600,  // 最多 10 分钟
    
    // 5. 资源限制
    resourceQuota: {
        memory: '128MB',
        cpu: '25%'
    }
};
```

---

## 四、子 Agent 配置

### 系统提示词

子 Agent 拥有专门的系统提示词：

```markdown
# 你是一个子 Agent

## 身份
- 你是由主 Agent 创建的临时执行单元
- 你的生命周期仅限于完成分配的任务
- 任务完成后，你的工作区将被清理

## 任务边界
- 你只需要完成分配给你的特定任务
- 不要尝试超出任务范围的行动
- 如果遇到无法解决的问题，及时报告给主 Agent

## 技能限制
- 你只能使用被授权的技能列表
- 不要尝试调用未授权的工具

## 结果报告
- 完成任务后，清晰地报告结果
- 如果任务失败，说明失败原因
```

### 配置继承

```javascript
// 三级继承机制：Agent级 > 全局默认 > 代码默认
const configInheritance = {
    agentLevel: {
        model: 'claude-3-opus',
        skills: ['file_ops', 'web_scraper']
    },
    
    globalDefault: {
        model: 'gpt-4',
        timeout: 300,
        maxIterations: 20
    },
    
    codeDefault: {
        model: 'gpt-3.5-turbo',
        timeout: 60,
        maxIterations: 10
    }
};
```

---

## 五、任务分发与执行

### 任务分发策略

```javascript
const dispatchStrategies = {
    // 1. 顺序执行
    sequential: async (tasks) => {
        const results = [];
        for (const task of tasks) {
            const result = await spawnAgent(task);
            results.push(result);
        }
        return results;
    },
    
    // 2. 并行执行
    parallel: async (tasks) => {
        const promises = tasks.map(task => spawnAgent(task));
        return Promise.all(promises);
    },
    
    // 3. 条件分支
    conditional: async (task, condition) => {
        if (evaluateCondition(condition)) {
            return await spawnAgent(task);
        }
        return null;
    }
};
```

### 执行示例

```javascript
async function processComplexTask(userRequest) {
    // 1. 分解任务
    const subtasks = analyzeTask(userRequest);
    
    // 2. 创建子 Agent 并行执行
    const results = await Promise.all(
        subtasks.map(task => 
            sessions_spawn({
                task: task.description,
                skills: task.requiredSkills,
                context: task.context,
                timeout: 300
            })
        )
    );
    
    // 3. 汇总结果
    return await summarizeResults(results);
}
```

---

## 六、结果回传

### 内部事件机制

```javascript
const agentEvents = {
    SPAWN_COMPLETE: 'spawn:complete',
    SPAWN_ERROR: 'spawn:error',
    SPAWN_PROGRESS: 'spawn:progress'
};

eventBus.on(agentEvents.SPAWN_COMPLETE, (event) => {
    const { spawnId, result, duration } = event;
    
    notifyMasterAgent({
        type: 'spawn_result',
        spawnId,
        result,
        duration
    });
});
```

### 结果格式

```javascript
const resultFormat = {
    status: 'completed',  // completed | failed | timeout
    
    result: {
        summary: '任务完成摘要',
        data: { /* 任务数据 */ },
        files: [ /* 生成的文件列表 */ ]
    },
    
    metadata: {
        duration: 45000,
        iterations: 5,
        toolsUsed: ['file_read', 'web_fetch'],
        tokensUsed: 15000
    },
    
    error: {
        code: 'TIMEOUT',
        message: '执行超时'
    }
};
```

---

## 七、多 Agent 配置实战

### 配置示例

```json
{
    "agents": {
        "list": [
            {
                "id": "main",
                "workspace": "~/.openclaw/workspace-main",
                "model": "claude-3-opus"
            },
            {
                "id": "researcher",
                "workspace": "~/.openclaw/workspace-researcher",
                "model": "gpt-4",
                "skills": ["web_search", "web_scraper", "summarizer"]
            },
            {
                "id": "coder",
                "workspace": "~/.openclaw/workspace-coder",
                "model": "claude-3-sonnet",
                "skills": ["file_ops", "execute_code", "git"]
            },
            {
                "id": "writer",
                "workspace": "~/.openclaw/workspace-writer",
                "model": "gpt-4",
                "skills": ["file_ops", "markdown", "format"]
            }
        ]
    }
}
```

### 协作流程示例

用户请求：`帮我研究 AI Agent 的最新进展，写一份报告`

```
1. 主 Agent 分析任务
   ↓
2. 创建研究员 Agent
   - 任务：搜索 AI Agent 最新论文和文章
   - 技能：web_search, web_scraper, summarizer
   ↓
3. 研究员 Agent 返回研究结果
   ↓
4. 主 Agent 分析研究结果
   ↓
5. 创建写作 Agent
   - 任务：根据研究结果撰写报告
   - 技能：file_ops, markdown, format
   ↓
6. 写作 Agent 返回报告文件
   ↓
7. 主 Agent 汇总返回用户
```

---

## 八、资源治理

### 并发控制

```javascript
class AgentPool {
    constructor(maxAgents = 10) {
        this.maxAgents = maxAgents;
        this.activeAgents = new Map();
    }
    
    async spawn(config) {
        if (this.activeAgents.size >= this.maxAgents) {
            throw new Error('Maximum concurrent agents reached');
        }
        
        const agentId = generateId();
        const agent = new SubAgent(config);
        
        this.activeAgents.set(agentId, {
            agent,
            startTime: Date.now(),
            status: 'running'
        });
        
        try {
            const result = await agent.execute();
            return result;
        } finally {
            setTimeout(() => {
                this.activeAgents.delete(agentId);
            }, 60000);
        }
    }
}
```

### 资源配额

```javascript
const resourceQuotas = {
    // 主 Agent 配额
    main: {
        memory: '512MB',
        cpu: '100%',
        maxSpawns: 5,
        maxSpawnDepth: 3
    },
    
    // 子 Agent 配额
    child: {
        memory: '256MB',
        cpu: '50%',
        maxSpawns: 2,
        maxSpawnDepth: 1
    },
    
    // 孙 Agent 配额
    grandchild: {
        memory: '128MB',
        cpu: '25%',
        maxSpawns: 0,
        maxSpawnDepth: 0
    }
};
```

---

## 九、错误处理

### 错误类型

```javascript
const errorTypes = {
    SPAWN_FAILED: {
        code: 'SPAWN_FAILED',
        message: '子 Agent 创建失败'
    },
    
    TIMEOUT: {
        code: 'TIMEOUT',
        message: '子 Agent 执行超时'
    },
    
    RESOURCE_EXHAUSTED: {
        code: 'RESOURCE_EXHAUSTED',
        message: '资源不足，无法创建子 Agent'
    },
    
    PERMISSION_DENIED: {
        code: 'PERMISSION_DENIED',
        message: '权限不足，无法执行此操作'
    }
};
```

### 错误恢复

```javascript
async function executeWithRecovery(task, options) {
    const { maxRetries = 2, fallbackStrategy = 'report' } = options;
    
    for (let attempt = 0; attempt <= maxRetries; attempt++) {
        try {
            return await spawnAgent(task);
        } catch (error) {
            if (attempt === maxRetries) {
                if (fallbackStrategy === 'report') {
                    return {
                        status: 'failed',
                        error: error.message
                    };
                } else if (fallbackStrategy === 'alternative') {
                    return await executeAlternative(task);
                }
            }
            await sleep(1000 * (attempt + 1));
        }
    }
}
```

---

## 十、与 WeKnora 对比

| 维度          | OpenClaw            | WeKnora                  |
| ------------- | ------------------- | ------------------------ |
| 子 Agent 创建 | sessions_spawn 工具 | Agent 独立部署           |
| 通信方式      | 内部事件            | EventBus + Redis Pub/Sub |
| 资源隔离      | 文件系统            | 租户 + Agent ID          |
| 并发控制      | Agent Pool          | Go 协程池                |
| 结果回传      | 同步等待            | 异步事件推送             |

### WeKnora 多 Agent 增强

```go
type AgentCoordinator struct {
    eventBus    *EventBus
    agentPool   *ants.Pool
    resultChan  chan AgentResult
}

func (c *AgentCoordinator) SpawnChild(ctx context.Context, req SpawnRequest) (*AgentResult, error) {
    task := func() {
        result := c.executeChildAgent(ctx, req)
        c.eventBus.Publish(ChildAgentComplete, result)
        c.resultChan <- result
    }
    
    if err := c.agentPool.Submit(task); err != nil {
        return nil, err
    }
    
    select {
    case result := <-c.resultChan:
        return &result, nil
    case <-time.After(req.Timeout):
        return nil, ErrTimeout
    }
}
```

WeKnora 通过 EventBus 实现了流式的事件推送，支持实时展示子 Agent 的执行进度。
