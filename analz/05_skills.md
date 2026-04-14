# WeKnora Skills 能力详解

## 1. 概述

WeKnora Skills 是一种**模块化的 Agent 能力扩展机制**，借鉴了 Claude 的 **Progressive Disclosure（渐进式披露）** 模式，允许将领域知识、工作流程、数据文件、可执行脚本以目录结构方式打包，在 Agent 运行时按需加载。

核心设计哲学：
- **Level 1**（元数据）：仅暴露技能名称和描述，轻量注入到系统提示词
- **Level 2**（指令）：完整的 SKILL.md 正文，按需加载
- **Level 3**（资源）：技能目录中的附加文件（数据文件、脚本等）

---

## 2. SKILL.md 文件结构

每个 Skill 必须以一个包含 YAML Frontmatter 的 `SKILL.md` 文件为核心，放置在独立子目录中：

```
skills/
├── data-analysis/
│   ├── SKILL.md           ← 技能定义文件（必须）
│   ├── scripts/
│   │   ├── analyze.py     ← 可执行脚本
│   │   └── preprocess.py
│   ├── FORMS.md           ← 附加指令文件
│   └── data/
│       └── templates.csv  ← 数据文件
├── code-review/
│   └── SKILL.md
└── report-generator/
    ├── SKILL.md
    └── templates/
        └── report.md
```

### SKILL.md 格式规范

```markdown
---
name: data-analysis
description: Perform comprehensive data analysis on CSV, JSON, or Excel files. 
             Use this skill when the user needs statistical analysis, 
             data visualization, or pattern recognition.
---

## Overview
This skill helps analyze structured data files...

## Workflow
1. Parse the input data file
2. Perform statistical analysis
3. Generate visualizations
4. Summarize findings

## Available Scripts
- `scripts/analyze.py <filepath>`: Main analysis script
- `scripts/preprocess.py <filepath>`: Data preprocessing

## Output Format
Analysis results will include:
- Summary statistics
- Data distribution charts
- Key patterns and anomalies
```

### Frontmatter 字段规范（来自 skill.go）

| 字段          | YAML 键       | 类型   | 限制                                            |
| ------------- | ------------- | ------ | ----------------------------------------------- |
| `Name`        | `name`        | string | 必填；最长 64 字符；仅 Unicode 字母/数字/连字符 |
| `Description` | `description` | string | 必填；最长 1024 字符                            |

**命名规则**：
- 仅允许 Unicode 字母、数字、连字符（`[\p{L}\p{N}-]+`）
- 不得包含保留词：`anthropic`、`claude`
- 不得包含 XML 标签
- 长度限制：64 字符以内

---

## 3. Go 数据结构

### 3.1 Skill 三层结构体

```go
// internal/agent/skills/skill.go
type Skill struct {
    // ===== Level 1：元数据（始终加载）=====
    Name        string `yaml:"name"`
    Description string `yaml:"description"`

    // ===== 文件系统信息 =====
    BasePath string // 技能目录的绝对路径
    FilePath string // SKILL.md 的绝对路径

    // ===== Level 2：指令（按需加载）=====
    Instructions string // SKILL.md 正文（Frontmatter 后的内容）
    Loaded       bool   // 是否已加载 Level 2
}
```

### 3.2 SkillMetadata（Level 1 轻量表示）

```go
type SkillMetadata struct {
    Name        string
    Description string
    BasePath    string // 供后续 Level 2/3 加载使用
}
```

### 3.3 SkillFile（Level 3 资源文件）

```go
type SkillFile struct {
    Name     string // 相对于技能目录的路径（如 "scripts/analyze.py"）
    Path     string // 绝对路径（供沙箱执行使用）
    Content  string // 文件内容
    IsScript bool   // 是否为可执行脚本
}
```

---

## 4. ParseSkillFile 解析流程

```go
func ParseSkillFile(content string) (*Skill, error) {
    // 1. 验证以 --- 开头（YAML Frontmatter 必须存在）
    if !strings.HasPrefix(strings.TrimSpace(content), "---") {
        return nil, errors.New("SKILL.md must start with YAML frontmatter (---)")
    }

    // 2. 逐行扫描分割 Frontmatter 和 Body
    inFrontmatter = false
    frontmatterEnded = false
    for each line:
        if "---" (first) → inFrontmatter = true
        if "---" (second) → frontmatterEnded = true
        collect to frontmatterLines or bodyLines

    // 3. YAML 解析 Frontmatter → Skill.Name, Skill.Description
    yaml.Unmarshal(frontmatter, &skill)

    // 4. 正文内容（TrimSpace 后）→ Skill.Instructions
    skill.Instructions = strings.TrimSpace(body)
    skill.Loaded = true

    // 5. 验证（Validate）
    skill.Validate()  // 名称/描述 长度、字符集、保留词检查

    return skill, nil
}
```

---

## 5. Loader：文件系统加载器

```go
// internal/agent/skills/loader.go
type Loader struct {
    skillDirs        []string           // 多个技能搜索目录
    discoveredSkills map[string]*Skill  // 内存缓存（name → Skill）
}
```

### 5.1 DiscoverSkills（Level 1 发现）

```
DiscoverSkills()
    ├── 遍历 skillDirs 中每个目录
    │   └── discoverInDirectory(dir)
    │       ├── 枚举子目录
    │       ├── 检查每个子目录是否含 SKILL.md
    │       ├── 解析 SKILL.md（ParseSkillFile）
    │       ├── 缓存到 discoveredSkills[skill.Name]
    │       └── 返回 skill.ToMetadata()（Level 1 元数据）
    └── 返回 []*SkillMetadata（所有已发现技能的元数据列表）
```

**特性**：
- 目录不存在时静默跳过（`os.IsNotExist` 则 `return nil, nil`）
- 解析失败的技能跳过并继续（容错设计）
- 同名技能后发现者覆盖先发现者（`discoveredSkills[name] = skill`）

### 5.2 LoadSkillInstructions（Level 2 按需加载）

```go
func (l *Loader) LoadSkillInstructions(skillName string) (*Skill, error) {
    // 1. 检查缓存（已加载则直接返回）
    if skill, ok := l.discoveredSkills[skillName]; ok && skill.Loaded {
        return skill, nil
    }

    // 2. 在所有目录中查找
    //    a. 先按目录名直接匹配（skillDirs/skillName/SKILL.md）
    //    b. 再扫描全目录找到 Name 字段匹配的 SKILL.md
    for dir in skillDirs:
        skill = loadSkillFromDirectory(dir, skillName)
        if found: cache and return

    return nil, "skill not found"
}
```

### 5.3 LoadSkillFile（Level 3 资源加载）

```go
func (l *Loader) LoadSkillFile(skillName, relativePath string) (*SkillFile, error) {
    // 1. 获取或加载技能（确保 BasePath 已知）
    skill = discoveredSkills[skillName] or LoadSkillInstructions()

    // 2. 安全验证（Path Traversal 防护）
    cleanPath = filepath.Clean(relativePath)
    if cleanPath 以 ".." 开头 || 是绝对路径:
        return nil, "invalid file path"

    fullPath = skill.BasePath + "/" + cleanPath

    // 3. 边界验证（确保文件在技能目录内）
    absSkillPath = abs(skill.BasePath)
    absFilePath = abs(fullPath)
    if !strings.HasPrefix(absFilePath, absSkillPath):
        return nil, "file path outside skill directory"

    // 4. 读取文件内容
    content = os.ReadFile(fullPath)

    return &SkillFile{
        Name:     relativePath,
        Path:     absFilePath,   // 使用绝对路径供沙箱执行
        Content:  string(content),
        IsScript: IsScript(relativePath),
    }
}
```

**路径安全机制**：
1. `filepath.Clean` 消除 `./`、`../` 等路径成分
2. 检查清理后的路径不以 `..` 开头
3. 通过 `strings.HasPrefix(absFilePath, absSkillPath)` 确保文件在技能目录边界内

---

## 6. Manager：生命周期管理器

```go
// internal/agent/skills/manager.go
type Manager struct {
    loader        *Loader          // 文件系统加载器
    sandboxMgr    sandbox.Manager  // 沙箱执行管理器

    skillDirs     []string         // 技能搜索目录
    allowedSkills []string         // 白名单（空=全部允许）
    enabled       bool             // 功能总开关

    metadataCache []*SkillMetadata // Level 1 元数据缓存
    mu            sync.RWMutex     // 读写锁
}
```

### 6.1 初始化流程

```
Server 启动
    ↓
Manager.Initialize(ctx)
    ├── enabled == false → return nil（跳过）
    ├── loader.DiscoverSkills() → []*SkillMetadata
    ├── filterAllowedSkills(metadata) → 白名单过滤
    └── metadataCache = filtered metadata（加写锁）
```

### 6.2 GetAllMetadata（Level 1 注入系统提示）

```go
func (m *Manager) GetAllMetadata() []*SkillMetadata {
    if !m.enabled { return nil }
    m.mu.RLock()
    defer m.mu.RUnlock()
    // 返回副本，防止外部修改缓存
    result := make([]*SkillMetadata, len(m.metadataCache))
    copy(result, m.metadataCache)
    return result
}
```

调用时机：每次 Agent 构建系统提示词时（`BuildSystemPromptWithOptions`）。

### 6.3 LoadSkill（Level 2 按需加载）

```go
func (m *Manager) LoadSkill(ctx, skillName) (*Skill, error) {
    if !m.enabled → error
    if !m.isSkillAllowed(skillName) → "skill not allowed"
    return m.loader.LoadSkillInstructions(skillName)
}
```

### 6.4 ExecuteScript（Level 3 脚本执行）

```go
func (m *Manager) ExecuteScript(ctx, skillName, scriptPath, args, stdin) (*sandbox.ExecuteResult, error) {
    // 1. 基础检查
    enabled? allowed? sandboxMgr != nil?

    // 2. 获取技能基础路径
    basePath = loader.GetSkillBasePath(skillName)

    // 3. 加载脚本文件（验证存在且是脚本）
    file = loader.LoadSkillFile(skillName, scriptPath)
    if !file.IsScript → error

    // 4. 构建沙箱执行配置
    config = &sandbox.ExecuteConfig{
        Script:  file.Path,   // 绝对路径
        Args:    args,
        WorkDir: basePath,    // 工作目录 = 技能根目录
        Stdin:   stdin,
    }

    // 5. 在沙箱中执行
    return sandboxMgr.Execute(ctx, config)
}
```

---

## 7. 沙箱安全执行系统

### 7.1 沙箱类型

```go
// internal/sandbox/sandbox.go
const (
    SandboxTypeDocker   = "docker"   // Docker 容器隔离（生产推荐）
    SandboxTypeLocal    = "local"    // 本地进程（开发/测试）
    SandboxTypeDisabled = "disabled" // 禁用脚本执行
)
```

### 7.2 默认配置参数

| 参数                 | 值                                    | 说明             |
| -------------------- | ------------------------------------- | ---------------- |
| `DefaultTimeout`     | 60s                                   | 脚本最大执行时间 |
| `DefaultMemoryLimit` | 256MB                                 | 最大内存使用     |
| `DefaultCPULimit`    | 1.0                                   | 最大 CPU 核数    |
| `DefaultDockerImage` | `wechatopenai/weknora-sandbox:latest` | Docker 沙箱镜像  |

### 7.3 ExecuteConfig 字段说明

```go
type ExecuteConfig struct {
    Script         string            // 脚本文件绝对路径
    Args           []string          // 命令行参数
    WorkDir        string            // 工作目录
    Timeout        time.Duration     // 超时（0=使用默认60s）
    Env            map[string]string // 额外环境变量
    AllowedCmds    []string          // 允许的命令白名单
    AllowNetwork   bool              // 是否允许网络访问（Docker only）
    MemoryLimit    int64             // 内存限制字节（Docker only）
    CPULimit       float64           // CPU 核数限制（Docker only）
    ReadOnlyRootfs bool              // 只读根文件系统（Docker only）
    Stdin          string            // 标准输入
    SkipValidation bool              // 跳过安全验证（仅信任脚本使用）
    ScriptContent  string            // 脚本内容（用于验证，不提供则自动读取）
}
```

### 7.4 允许的默认命令列表

```go
[]string{
    "python", "python3", "node",
    "bash", "sh",
    "cat", "echo", "head", "tail",
    "grep", "sed", "awk",
    "sort", "uniq", "wc", "cut", "tr",
    "ls", "pwd", "date",
}
```

---

## 8. ScriptValidator 安全验证体系

`internal/sandbox/validator.go` 实现了多层次安全检查，在沙箱执行前验证脚本安全性。

### 8.1 ValidateAll 综合验证

```
ValidateAll(scriptContent, args, stdin)
    ├── ValidateScript(scriptContent)     ← 脚本内容检查
    ├── ValidateArgs(args)                ← 参数注入检查
    └── ValidateStdin(stdin)              ← 标准输入检查
```

### 8.2 脚本内容验证（ValidateScript）

**危险命令检测（字符串精确匹配）**：
| 类别         | 示例                                             |
| ------------ | ------------------------------------------------ |
| 文件系统破坏 | `rm -rf /`, `dd if=/dev/zero`, `mkfs`            |
| Fork Bomb    | `:(){ :                                          | :& };:`, `bomb(){ bomb\|bomb& };bomb` |
| 系统控制     | `shutdown`, `reboot`, `halt`, `killall`, `pkill` |
| 权限提升     | `chmod 777 /`, `setuid`, `passwd`                |
| 凭证访问     | `/etc/passwd`, `/etc/shadow`, `id_rsa`           |
| 容器逃逸     | `docker`, `kubectl`, `nsenter`, `unshare`        |
| 环境篡改     | `export PATH=`, `export LD_PRELOAD`              |

**危险模式检测（正则表达式，不区分大小写）**：
| 类别                  | 正则模式                                         |
| --------------------- | ------------------------------------------------ |
| Base64 解码执行       | `base64\s+(-d\|--decode)`                        |
| 管道执行              | `curl.*\|\s*(bash\|sh)`, `wget.*\|\s*(bash\|sh)` |
| 代码注入              | `eval\s*\(`, `exec\s*\(`, `os\.system\s*\(`      |
| Pickle 反序列化       | `pickle\.loads?\s*\(`                            |
| subprocess shell=True | `subprocess\.call.*shell\s*=\s*True`             |
| Fork Bomb 函数模式    | `:\s*\(\s*\)\s*\{\s*:`                           |
| 危险 rm 模式          | `rm\s+-[rf]+\s+/`                                |
| YAML 不安全加载       | `yaml\.load\s*\([^,]+\)`, `yaml\.unsafe_load`    |

**网络访问检测（hasNetworkAccess）**：
检测 `curl`, `wget`, `nc`, `socket.connect`, `requests.get`, `fetch()`, `axios`, `XMLHttpRequest` 等

**反弹 Shell 检测（hasReverseShellPattern）**：
检测 `/dev/tcp/`, `bash -i`, `python.*pty.spawn`, `socat.*exec`, `mkfifo` 等

### 8.3 参数注入检查（ValidateArgs）

检测 Shell 操作符（`&&`, `||`, `;`, `|`, `\n`, `$(`, `` ` ``, `>`, `<`）、命令替换（`$(...)`, `` `...` ``）、路径遍历（`../`）、环境变量注入（`${VAR}`, `$VAR`）。

### 8.4 标准输入检查（ValidateStdin）

检测嵌入的命令替换模式（`$(...)`、反引号）和 Shell 操作符。

---

## 9. 可执行脚本类型判断

```go
// IsScript 判断文件扩展名是否为可执行脚本
func IsScript(path string) bool {
    scriptExtensions := map[string]bool{
        ".py":   true,  // Python
        ".sh":   true,  // Shell
        ".bash": true,  // Bash
        ".js":   true,  // Node.js
        ".ts":   true,  // TypeScript (ts-node)
        ".rb":   true,  // Ruby
        ".pl":   true,  // Perl
        ".php":  true,  // PHP
    }
    ext := strings.ToLower(filepath.Ext(path))
    return scriptExtensions[ext]
}

// GetScriptLanguage 返回对应的解释器名称
func GetScriptLanguage(path string) string {
    // .py → "python"
    // .sh/.bash → "bash"
    // .js → "node"
    // .ts → "ts-node"
    // .rb → "ruby"
    // .pl → "perl"
    // .php → "php"
}
```

---

## 10. Progressive Disclosure 在 Agent 中的应用

### 10.1 Level 1：系统提示注入

每次 Agent 构建系统提示词时，调用 `GetAllMetadata()` 获取所有 Skills 的元数据，并通过 `formatSkillsMetadata` 注入：

```go
// internal/agent/prompts.go
func formatSkillsMetadata(metadata []*skills.SkillMetadata) string {
    // 生成类似如下的 Markdown 片段注入到系统提示词：
    // ## Available Skills
    // You have access to the following pre-installed skills:
    //
    // ### data-analysis
    // Perform comprehensive data analysis...
    // To use this skill, call: execute_skill("data-analysis")
    //
    // ### code-review
    // ...
}
```

LLM 看到技能名称和描述后，会在需要时主动调用 `execute_skill` 工具。

### 10.2 Level 2：指令加载（execute_skill 工具触发）

当 LLM 调用 `execute_skill("data-analysis")` 时：

```
execute_skill 工具处理
    ↓
manager.LoadSkill(ctx, "data-analysis")
    ↓
loader.LoadSkillInstructions("data-analysis")
    ↓
返回 Skill.Instructions（SKILL.md 正文内容）
    ↓
注入到当前对话上下文（作为 tool result 返回给 LLM）
    ↓
LLM 获得完整指令，继续执行技能工作流
```

### 10.3 Level 3：资源按需读取

Skill 指令中可以引用附加文件，LLM 通过 `execute_skill` 的子命令读取：

```
LLM 调用: execute_skill("data-analysis", action="read_file", file="scripts/analyze.py")
    ↓
manager.ReadSkillFile(ctx, "data-analysis", "scripts/analyze.py")
    ↓
loader.LoadSkillFile("data-analysis", "scripts/analyze.py")
    ↓
返回文件内容（路径安全验证通过后）

LLM 调用: execute_skill("data-analysis", action="run_script", script="scripts/analyze.py", args=["data.csv"])
    ↓
manager.ExecuteScript(ctx, "data-analysis", "scripts/analyze.py", ["data.csv"], "")
    ↓
sandboxMgr.Execute(ctx, config)
```

---

## 11. Docker 沙箱实现（docker.go）

Docker 沙箱提供最强隔离级别：

```
脚本执行请求
    ↓
DockerSandbox.Execute(ctx, config)
    ↓
1. 安全验证（ScriptValidator.ValidateAll）
    ↓
2. 创建 Docker 容器
   - 镜像: wechatopenai/weknora-sandbox:latest
   - 挂载: skill 目录（只读）→ /workspace
   - 资源限制: Memory=256MB, CPU=1.0
   - 网络: 默认禁用（AllowNetwork=false）
   - 根文件系统: 只读（ReadOnlyRootfs=true）
    ↓
3. 在容器中执行脚本
   - 工作目录: /workspace
   - 环境变量: 仅允许的变量
   - 超时: 60 秒
    ↓
4. 收集 stdout/stderr/exitCode
    ↓
5. 销毁容器
    ↓
返回 ExecuteResult
```

---

## 12. 本地沙箱实现（local.go）

本地沙箱用于开发环境或 Docker 不可用时的降级方案：

```
LocalSandbox.Execute(ctx, config)
    ↓
1. 安全验证（ScriptValidator.ValidateAll）
    ↓
2. 构建命令
   cmd = exec.CommandContext(ctx, interpreter, script, args...)
   cmd.Dir = workDir
   cmd.Env = filtered（仅允许的环境变量）
    ↓
3. 设置 stdin（如果有）
    ↓
4. 执行并监控（context 超时）
    ↓
5. 若超时 → Kill 进程
    ↓
返回 ExecuteResult
```

---

## 13. 与 Agent 系统的集成接口

### 13.1 技能选择模式

Agent 配置（`CustomAgentConfig`）中的 `skills_selection_mode`：

```
skills_selection_mode = "all"      → 使用所有预装技能
skills_selection_mode = "selected" → 仅使用 selected_skills 列表中的技能
skills_selection_mode = "none"     → 不使用任何技能
```

### 13.2 技能注入到 Agent

```go
// 在 Agent 创建时根据配置初始化 SkillsManager
agentSkillsConfig := &skills.ManagerConfig{
    SkillDirs:     config.SkillDirs,       // 全局技能目录
    AllowedSkills: agentConfig.SelectedSkills, // 白名单
    Enabled:       skillsEnabled,
}
skillManager := skills.NewManager(agentSkillsConfig, sandboxMgr)
skillManager.Initialize(ctx)

// 构建系统提示词时注入 Level 1 元数据
opts := &BuildSystemPromptOptions{
    SkillsMetadata: skillManager.GetAllMetadata(),
}
systemPrompt = BuildSystemPromptWithOptions(kbs, webEnabled, selectedDocs, opts)
```

### 13.3 execute_skill 工具注册

```go
// tools/registry.go 中注册 execute_skill 工具
registry.Register(&ExecuteSkillTool{
    skillManager: skillManager,
})
```

`execute_skill` 工具执行流程：
1. LLM 调用 `execute_skill({"skill": "data-analysis", "action": "get_instructions"})`
2. 工具调用 `manager.LoadSkill(ctx, "data-analysis")`
3. 返回 SKILL.md 正文给 LLM
4. LLM 理解指令后继续执行工作流

---

## 14. 前端 Skills 配置界面

### 14.1 Skills API

```typescript
// frontend/src/api/skill/index.ts
export interface SkillInfo {
  name: string;
  description: string;
}

// 获取预装 Skills 列表
// skills_available=false 表示沙箱未启用，前端应隐藏/禁用 Skills 配置
export function listSkills() {
  return get<{ data: SkillInfo[]; skills_available?: boolean }>('/api/v1/skills');
}
```

### 14.2 Agent 配置中的 Skills 设置

在 `AgentEditorModal.vue` 的 Skills 配置 Tab 中：

```typescript
// CustomAgentConfig 中的 Skills 相关字段
{
  skills_selection_mode: 'all' | 'selected' | 'none',
  selected_skills: string[],  // 选择的 Skill 名称列表
}
```

UI 交互：
- `skills_available=false`：隐藏 Skills 配置（沙箱未启用）
- 选择 "全部"：加载所有已发现的技能
- 选择 "指定"：显示多选列表，来自 `listSkills()` 接口
- 选择 "不使用"：不向 Agent 注入任何技能信息

---

## 15. 完整技能执行时序图

```
用户: "请帮我分析 sales_data.csv 文件"
    ↓
Agent 系统提示（Level 1 注入）:
    ## Available Skills
    ### data-analysis
    Perform comprehensive data analysis...
    ↓
LLM 决策: 需要使用 data-analysis 技能
    ↓
LLM 调用: execute_skill({"skill": "data-analysis", "action": "get_instructions"})
    ↓
Manager.LoadSkill → SKILL.md 正文返回给 LLM（Level 2 加载）
    ↓
LLM 读取指令，决定: "需要运行 scripts/analyze.py"
    ↓
LLM 调用: execute_skill({"skill": "data-analysis", "action": "run_script", 
                          "script": "scripts/analyze.py", "args": ["sales_data.csv"]})
    ↓
Manager.ExecuteScript
    ↓
Sandbox.Execute（安全验证 → 沙箱执行）
    ↓
ExecuteResult{Stdout: "分析结果...", ExitCode: 0}
    ↓
LLM 接收结果，调用 final_answer 输出总结
```

---

## 16. 安全边界总结

| 安全层     | 机制                                | 保护内容           |
| ---------- | ----------------------------------- | ------------------ |
| 名称验证   | 正则 + 保留词检查                   | 防止 XSS/注入      |
| 路径验证   | `filepath.Clean` + 绝对路径前缀检查 | 防止路径遍历       |
| 脚本验证   | `ScriptValidator.ValidateAll`       | 防止恶意脚本       |
| 参数验证   | Shell 操作符 + 命令替换检查         | 防止参数注入       |
| stdin 验证 | 嵌入命令检测                        | 防止输入注入       |
| 沙箱隔离   | Docker（内存/CPU/网络限制）         | 执行环境隔离       |
| 工具白名单 | `allowedSkills []string`            | Agent 级别权限控制 |
| 命令白名单 | `AllowedCmds []string`              | 系统命令访问控制   |
| 超时机制   | `DefaultTimeout = 60s`              | 防止无限执行       |
