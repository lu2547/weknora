# OpenClaw Skills 系统：文档即工具的设计哲学

## 一、Skills 系统概述

Skills（技能）系统是 OpenClaw 的能力底座，通过模块化插件实现功能解耦。其核心理念是**"文档即工具"**——用 Markdown 文件描述能力，让模型理解并调用。

### 设计哲学

```
传统方式：编写代码 → 编译 → 部署 → API 调用
OpenClaw：编写 SKILL.md → 放入目录 → 自动发现
```

### 核心优势

1. **零代码门槛**：用自然语言描述能力
2. **即插即用**：放入目录即可使用
3. **标准化复用**：ClawHub 共享 25,000+ 技能
4. **灵活定制**：用户可随时修改

---

## 二、SKILL.md 规范

### 文件结构

```
skills/
├── mail-assistant/
│   ├── SKILL.md          # 技能描述（必需）
│   ├── skill.js          # 执行脚本（可选）
│   └── templates/        # 模板文件（可选）
├── web-scraper/
│   └── SKILL.md
└── report-generator/
    └── SKILL.md
```

### SKILL.md 格式

```markdown
# Mail Assistant

读取和管理邮箱，筛选重要邮件，自动回复。

## Capabilities

- 读取收件箱邮件列表
- 按条件筛选邮件（发件人、主题、日期）
- 标记邮件为已读/未读
- 发送邮件回复

## Parameters

### mail_list
读取邮件列表

**参数：**
- `folder` (string, 可选): 邮件文件夹，默认 "inbox"
- `limit` (number, 可选): 返回数量，默认 20
- `unread_only` (boolean, 可选): 仅未读邮件

**返回：**
{
    "emails": [
        {
            "id": "msg_001",
            "from": "sender@example.com",
            "subject": "项目进度更新",
            "date": "2026-03-22",
            "is_read": false
        }
    ],
    "total": 15
}

### mail_send
发送邮件

**参数：**
- `to` (string, 必需): 收件人邮箱
- `subject` (string, 必需): 邮件主题
- `body` (string, 必需): 邮件正文

## Examples

**示例 1：读取今天的未读邮件**
使用 mail_list 工具，设置 unread_only=true

**示例 2：自动回复确认邮件**
使用 mail_send 工具，主题设为 "Re: [原主题]"，正文包含确认信息

## Constraints

- 每分钟最多读取 100 封邮件
- 不允许发送垃圾邮件
- 敏感邮件需要用户确认
```

---

## 三、技能发现与加载

### 发现流程

```
启动 Gateway → 扫描 skills 目录 → 解析 SKILL.md 
→ 提取元数据 → 校验格式 → 校验通过? 
    → 是: 注册到技能库
    → 否: 记录警告
→ 技能就绪
```

### 元数据提取

```javascript
function parseSkillMarkdown(content) {
    return {
        name: extractTitle(content),
        description: extractDescription(content),
        capabilities: extractCapabilities(content),
        parameters: extractParameters(content),
        examples: extractExamples(content),
        constraints: extractConstraints(content)
    };
}
```

### 技能过滤

```javascript
async function filterSkills(skills, context) {
    return skills.filter(skill => {
        // 1. 权限检查
        if (!checkPermission(skill, context.user)) return false;
        
        // 2. 安全策略检查
        if (violatesSecurityPolicy(skill)) return false;
        
        // 3. 渠道匹配
        if (!matchesChannel(skill, context.channel)) return false;
        
        return true;
    });
}
```

---

## 四、技能注入

### 注入到系统提示词

```javascript
function buildSkillPrompt(skills) {
    let prompt = '## Available Skills\n\n';
    
    for (const skill of skills) {
        prompt += `### ${skill.name}\n`;
        prompt += `${skill.description}\n\n`;
        prompt += '**Capabilities:**\n';
        for (const cap of skill.capabilities) {
            prompt += `- ${cap}\n`;
        }
        prompt += '\n';
    }
    
    return prompt;
}
```

### 注入示例

```
## Available Skills

### Mail Assistant
读取和管理邮箱，筛选重要邮件，自动回复。

**Capabilities:**
- 读取收件箱邮件列表
- 按条件筛选邮件
- 标记邮件为已读/未读
- 发送邮件回复

### Web Scraper
浏览网页，提取内容，截图保存。

**Capabilities:**
- 打开指定 URL
- 提取页面文本
- 截取屏幕快照
- 填写表单
```

---

## 五、技能执行

### 执行流程

```
LLM 选择技能 → 解析参数 → 参数校验 → 校验通过?
    → 否: 返回错误
    → 是: 权限检查 → 需要审批?
        → 是: 等待用户确认
        → 否: 执行技能
→ 返回结果 → 结果后处理
```

### 执行方式

#### 1. Shell 脚本执行

```javascript
async function executeSkill(params) {
    const result = await execShell(`node skill.js ${JSON.stringify(params)}`);
    return JSON.parse(result);
}
```

#### 2. 内置工具调用

```javascript
async function executeSkill(params) {
    switch (params.action) {
        case 'mail_list':
            return await mailClient.list(params);
        case 'mail_send':
            return await mailClient.send(params);
    }
}
```

#### 3. HTTP API 调用

```javascript
async function executeSkill(params) {
    const response = await fetch(params.apiEndpoint, {
        method: 'POST',
        body: JSON.stringify(params.data)
    });
    return response.json();
}
```

---

## 六、安全机制

### 权限分级

| 级别     | 权限                 | 示例技能           |
| -------- | -------------------- | ------------------ |
| Low      | 读取文件、搜索       | 文件搜索、网页抓取 |
| Medium   | 写入文件、发送消息   | 邮件发送、文件创建 |
| High     | 执行 Shell、修改系统 | 软件安装、进程管理 |
| Critical | 系统配置、权限变更   | 用户管理、安全设置 |

### 审批流程

```javascript
const approvalPolicy = {
    low: 'auto',        // 自动执行
    medium: 'notify',   // 通知用户
    high: 'confirm',    // 等待确认
    critical: 'require_2fa'  // 二次验证
};
```

### 执行隔离

高危技能在沙箱中执行：

```javascript
async function executeInSandbox(skill, params) {
    const container = await docker.createContainer({
        Image: 'openclaw-sandbox:latest',
        Cmd: ['node', '/skill/skill.js', JSON.stringify(params)],
        HostConfig: {
            Memory: 256 * 1024 * 1024,  // 256MB
            CpuQuota: 50000,             // 50% CPU
            NetworkMode: 'none',         // 禁用网络
            ReadonlyRootfs: true         // 只读文件系统
        }
    });
    
    await container.start();
    const result = await container.wait();
    return result;
}
```

---

## 七、技能开发指南

### 开发流程

```
1. 创建技能目录
2. 编写 SKILL.md
3. (可选) 编写执行脚本
4. 测试验证
5. 发布到 ClawHub
```

### 最佳实践

#### 1. 清晰的能力描述

```markdown
# Bad
这个技能可以做事。

# Good
读取指定邮箱文件夹中的邮件列表，支持按发件人、主题、日期筛选，
返回最多 100 封邮件的基本信息（ID、发件人、主题、日期、已读状态）。
```

#### 2. 完整的参数说明

```markdown
## Parameters

### search
搜索文件内容

**参数：**
- `query` (string, 必需): 搜索关键词，支持通配符 * 和 ?
- `path` (string, 可选): 搜索路径，默认为工作区根目录
- `file_pattern` (string, 可选): 文件匹配模式，如 "*.md"
- `case_sensitive` (boolean, 可选): 是否区分大小写，默认 false
```

#### 3. 实用的示例

```markdown
## Examples

**搜索所有 Markdown 文件中的 "OpenClaw"：**
search(query="OpenClaw", file_pattern="*.md")

**在 src 目录搜索区分大小写的 "API"：**
search(query="API", path="src", case_sensitive=true)
```

---

## 八、ClawHub 生态

### 技能库规模

- 截至 2026 年 3 月：**25,000+ 技能**
- 官方认证技能：**500+**
- 社区贡献：**24,500+**

### 热门技能分类

| 类别       | 技能数量 | 热门示例                     |
| ---------- | -------- | ---------------------------- |
| 办公自动化 | 8,000+   | 邮件处理、日程管理、文档转换 |
| 开发工具   | 6,000+   | Git 操作、代码生成、调试     |
| 数据处理   | 5,000+   | Excel 操作、数据分析、可视化 |
| 网络服务   | 3,000+   | API 调用、网页抓取、消息推送 |
| 生活服务   | 2,000+   | 天气查询、翻译、购物比价     |
| 其他       | 1,000+   | 游戏、娱乐、学习             |

### 安全警告

ClawHub 发现的恶意技能：
- 初次扫描：**341 个恶意技能（12%）**
- 后续扫描：**超过 800 个（约 20%）**
- 主要威胁：投放 Atomic macOS Stealer

---

## 九、与 WeKnora 对比

| 维度     | OpenClaw    | WeKnora             |
| -------- | ----------- | ------------------- |
| 技能描述 | SKILL.md    | SKILL.md（兼容）    |
| 加载方式 | 一次性注入  | 三级渐进加载        |
| 执行环境 | Docker 沙箱 | Docker 沙箱（兼容） |
| 权限控制 | 四级分级    | 细粒度权限          |

### WeKnora 三级渐进加载

```go
// Level 1: 元数据注入（轻量）
type SkillMetadata struct {
    Name        string
    Description string
    Category    string
}

// Level 2: 按需加载指令
type SkillInstruction struct {
    Parameters  []Parameter
    Examples    []Example
    Constraints []string
}

// Level 3: 脚本执行
type SkillExecutor struct {
    Script      string
    DockerImage string
    Timeout     time.Duration
}
```

这种设计避免了上下文爆炸，只在需要时加载详细信息。
