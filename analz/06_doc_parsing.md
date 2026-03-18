# WeKnora 文档解析能力详解

## 1. 概述

WeKnora 的文档解析能力通过独立的 **docreader** 微服务实现，采用 **Python gRPC 服务**提供文档解析功能，Go 主应用通过 gRPC 客户端调用。docreader 将各类文档（PDF、Word、Excel、图片等）转换为结构化的 Markdown 文本 + 切片（Chunk），供后续的向量化和检索使用。

---

## 2. 服务架构

```
Go 主应用（知识库写入流程）
    ↓
docreader/client/client.go（gRPC Client）
    ↓ proto.ReadRequest
    ↓
docreader/main.py（gRPC Server）
  ├── DocReaderServicer.Read()   ← 文件/URL 解析
  └── DocReaderServicer.ListEngines() ← 引擎列表查询
    ↓
Parser.parse_file() / parse_url()
    ↓
ParserEngineRegistry.get_parser_class(engine, file_type)
    ↓
具体 Parser 实现（PDF/DOCX/DOC/Excel/Image/Web/Markdown）
    ↓
Document{content: Markdown, images: {ref→base64}}
    ↓ proto.ReadResponse
    ↓
Go 应用：接收 Markdown 内容 + 图片字节数据
    ↓
文本切分（TextSplitter）→ 向量化 → 存入 PostgreSQL/向量数据库
```

---

## 3. gRPC 接口定义

### 3.1 ReadRequest（请求）

```protobuf
message ReadRequest {
    string request_id   = 1;  // 请求追踪 ID
    string file_name    = 2;  // 文件名（含扩展名）
    string file_type    = 3;  // 文件类型（可选，优先从 file_name 解析）
    bytes  file_content = 4;  // 文件字节内容（文件模式）
    string url          = 5;  // URL（URL 模式）
    string title        = 6;  // URL 标题
    ReadConfig config   = 7;  // 解析配置
}

message ReadConfig {
    string parser_engine = 1;  // 引擎名（builtin / markitdown）
    map<string, string> parser_engine_overrides = 2; // 引擎参数覆盖
}
```

### 3.2 ReadResponse（响应）

```protobuf
message ReadResponse {
    string markdown_content = 1;  // 解析后的 Markdown 文本
    repeated ImageRef image_refs = 2; // 图片引用列表
    string image_dir_path = 3;    // （保留字段，Go 侧持久化）
    string error = 4;             // 错误信息（空=成功）
}

message ImageRef {
    string filename     = 1;  // 图片文件名
    string original_ref = 2;  // 文档中的原始引用路径
    string mime_type    = 3;  // MIME 类型（image/png 等）
    bytes  image_data   = 4;  // 图片原始字节数据（内联传输）
    string storage_key  = 5;  // 存储 Key（Go 侧持久化后填充）
}
```

### 3.3 Go 客户端

```go
// docreader/client/client.go
type Client struct {
    conn *grpc.ClientConn
    proto.DocReaderClient
    debug bool
}

func NewClient(addr string) (*Client, error) {
    maxMsgSize := getMaxMessageSize()  // 默认 50MB，MAX_FILE_SIZE_MB 环境变量控制
    opts := []grpc.DialOption{
        grpc.WithTransportCredentials(insecure.NewCredentials()),
        grpc.WithDefaultServiceConfig(`{"loadBalancingPolicy":"round_robin"}`),
        grpc.WithDefaultCallOptions(
            grpc.MaxCallRecvMsgSize(maxMsgSize),
            grpc.MaxCallSendMsgSize(maxMsgSize),
        ),
    }
    resolver.SetDefaultScheme("dns")
    conn, _ := grpc.Dial("dns:///"+addr, opts...)  // DNS 负载均衡
    return &Client{conn: conn, DocReaderClient: proto.NewDocReaderClient(conn)}, nil
}
```

**特性**：
- `round_robin` 负载均衡策略（多实例部署）
- `dns:///` 前缀支持 DNS 服务发现
- 消息大小通过环境变量 `MAX_FILE_SIZE_MB` 配置（默认 50MB）

---

## 4. 解析引擎注册表

### 4.1 ParserEngineRegistry 架构

```python
# docreader/parser/registry.py
class ParserEngineRegistry:
    _engines: Dict[str, Dict[str, Type[BaseParser]]]  # engine → {ext → ParserClass}
    _descriptions: Dict[str, str]                     # engine → 描述
    _check_available: Dict[str, Callable]             # engine → 可用性检查函数
    _unavailable_hint: Dict[str, str]                 # engine → 不可用提示

    def get_parser_class(self, engine, file_type) -> Type[BaseParser]:
        """引擎降级机制：指定引擎不支持该格式时，自动回退到 builtin"""
        ft = file_type.lower()
        if engine and engine in self._engines:
            cls = self._engines[engine].get(ft)
            if cls: return cls
            # 自动降级
        # 使用 builtin 引擎
        cls = self._engines[BUILTIN_ENGINE].get(ft)
        if cls: return cls
        raise ValueError(f"Unsupported file type: {file_type}")
```

### 4.2 已注册引擎

**builtin 引擎**（内置，无需额外配置）：

| 文件类型                         | 解析器           | 说明                                         |
| -------------------------------- | ---------------- | -------------------------------------------- |
| `docx`                           | `Docx2Parser`    | Word 2007+ 格式                              |
| `doc`                            | `DocParser`      | Word 97-2003 格式（需 antiword/LibreOffice） |
| `pdf`                            | `PDFParser`      | PDF 文档                                     |
| `md`, `markdown`                 | `MarkdownParser` | Markdown 文本                                |
| `xlsx`, `xls`                    | `ExcelParser`    | Excel 表格                                   |
| `jpg/jpeg/png/gif/bmp/tiff/webp` | `ImageParser`    | 图片（OCR/VLM）                              |

**markitdown 引擎**（微软 MarkItDown 库）：

| 文件类型             | 说明                |
| -------------------- | ------------------- |
| `pdf`, `docx`, `doc` | 通用文档转 Markdown |
| `pptx`, `ppt`        | PowerPoint 演示文稿 |
| `xlsx`, `xls`, `csv` | 表格文件            |
| `md`, `markdown`     | Markdown            |

> **降级策略**：markitdown 引擎指定但不支持某格式（如 `jpg`）时，自动降级到 builtin 引擎处理。

---

## 5. 各格式解析器详解

### 5.1 DocParser（.doc 格式）

DOC 格式解析是最复杂的，采用**责任链模式**，三种方法按优先级尝试：

```python
# docreader/parser/doc_parser.py
class DocParser(Docx2Parser):
    def parse_into_text(self, content: bytes) -> Document:
        handle_chain = [
            self._parse_with_docx,    # 1. LibreOffice 转 DOCX（支持图片提取）
            self._parse_with_antiword, # 2. antiword 提取纯文本
            # self._parse_with_textract  # 3. 已禁用（SSRF 漏洞）
        ]
        with TempFileContext(content, ".doc") as temp_file_path:
            for handle in handle_chain:
                try:
                    document = handle(temp_file_path)
                    if document: return document
                except Exception as e:
                    logger.warning(f"Failed with {handle.__name__}: {e}")
        return Document(content="")
```

**方法1：LibreOffice 转换（_parse_with_docx）**

```python
def _try_convert_doc_to_docx(self, doc_path: str) -> Optional[bytes]:
    soffice_path = self._try_find_soffice()
    # 搜索路径：
    # Linux:   /usr/bin/soffice, /usr/lib/libreoffice/program/soffice
    # macOS:   /Applications/LibreOffice.app/Contents/MacOS/soffice
    # Windows: C:\Program Files\LibreOffice\program\soffice.exe
    # 环境变量: LIBREOFFICE_PATH

    with TempDirContext() as temp_dir:
        cmd = [soffice_path, "--headless", "--convert-to", "docx", "--outdir", temp_dir, doc_path]
        stdout, stderr, returncode = sandbox_executor.execute_in_sandbox(cmd)
        # 60s 超时，代理配置（防外网访问）
```

**方法2：antiword（_parse_with_antiword）**

```python
def _parse_with_antiword(self, temp_file_path: str) -> Document:
    antiword_path = self._try_find_antiword()
    # 搜索路径：/usr/bin/antiword, /usr/local/bin/antiword
    # 环境变量: ANTIWORD_PATH

    cmd = [antiword_path, temp_file_path]
    stdout, stderr, returncode = sandbox_executor.execute_in_sandbox(cmd)
    text = stdout.decode("utf-8", errors="ignore")
    return Document(content=text)
```

**SandboxExecutor 代理机制**：
```python
class SandboxExecutor:
    proxy = proxy or CONFIG.external_https_proxy or "http://128.0.0.1:1"
    # 默认代理 128.0.0.1:1 = 无效地址 → 强制禁止外网访问

    def execute_in_sandbox(self, cmd):
        env = os.environ.copy()
        env["http_proxy"] = self.proxy
        env["https_proxy"] = self.proxy
        process = subprocess.Popen(cmd, env=env, ...)
        stdout, stderr = process.communicate(timeout=60)  # 60s 超时
```

### 5.2 Docx2Parser（.docx 格式）

使用 `python-docx2txt` 或专用 DOCX 解析库，支持：
- 正文文本提取
- 表格提取（转 Markdown 格式）
- 图片提取（内嵌图片 → base64 编码）

### 5.3 PDFParser（.pdf 格式）

使用 `pymupdf`（fitz）或 `pdfplumber`：
- 文本层提取（有文字层的 PDF）
- 页面级图片提取
- 表格识别（结合坐标信息）
- 扫描版 PDF → OCR（ImageParser 处理）

### 5.4 MarkdownParser（.md 格式）

`docreader/parser/markdown_parser.py`（13.9KB）：
- Markdown 结构解析（标题层级、代码块、表格、链接）
- 图片引用替换（`![alt](path)` → 内联存储）
- Front Matter（YAML 头部）处理
- 数学公式保护（LaTeX `$` 和 `$$`）

### 5.5 ExcelParser（.xlsx/.xls 格式）

`docreader/parser/excel_parser.py`：
- 使用 `openpyxl` 读取 Excel
- 多 Sheet 处理（每个 Sheet 转为 Markdown 表格）
- 单元格格式保留（合并单元格处理）

### 5.6 ImageParser（图片格式）

`docreader/parser/image_parser.py`：
- 支持 jpg/jpeg/png/gif/bmp/tiff/webp
- OCR 文字识别（Tesseract / PaddleOCR）
- VLM 图像描述（多模态模型）
- 返回 `Document{content: OCR结果或描述文本}`

### 5.7 MarkitdownParser（markitdown 引擎）

`docreader/parser/markitdown_parser.py`：
- 使用微软开源 `markitdown` 库
- 支持 PDF、DOCX、PPTX、Excel、CSV 等格式
- 统一输出 Markdown

### 5.8 WebParser（URL 模式）

`docreader/parser/web_parser.py`（4.9KB）：
- 使用 `requests` + `beautifulsoup4` 抓取网页
- HTML → Markdown 转换（html2text 库）
- 图片 URL 提取
- 内容清洗（导航栏、广告等噪声过滤）

---

## 6. Document / Chunk 数据模型

### 6.1 Document 模型

```python
# docreader/models/document.py
class Document(BaseModel):
    content: str = ""                    # Markdown 文本内容
    images: Dict[str, str] = {}          # {原始引用路径: base64数据}
    chunks: List[Chunk] = []             # 切分后的片段列表
    metadata: Dict[str, Any] = {}        # 文档元数据
```

### 6.2 Chunk 模型

```python
class Chunk(BaseModel):
    content: str = ""             # 切片文本内容
    seq: int = 0                  # 切片序号（文档内位置）
    start: int = 0                # 在原文中的起始字符位置
    end: int                      # 在原文中的结束字符位置
    images: List[Dict] = []       # 切片内包含的图片引用
    metadata: Dict[str, Any] = {} # 元数据（标题路径等）
```

---

## 7. TextSplitter 文本切分引擎

### 7.1 核心参数

```python
# docreader/splitter/splitter.py
class TextSplitter(BaseModel):
    chunk_size: int = 512    # 默认每切片最大字符数
    chunk_overlap: int = 100 # 默认切片间重叠字符数
    separators: List[str] = ["\n", "。", " "]  # 分隔符（优先级从高到低）
    protected_regex: List[str] = [...]  # 受保护的正则模式（不切断）
```

### 7.2 受保护的内容模式（不跨切片切断）

| 类型          | 正则               | 说明                                        |
| ------------- | ------------------ | ------------------------------------------- |
| LaTeX 公式    | `\$\$[\s\S]*?\$\$` | `$$..$$` 包围的数学公式                     |
| Markdown 图片 | `!\[.*?\]\(.*?\)`  | `![alt](url)` 格式                          |
| Markdown 链接 | `\[.*?\]\(.*?\)`   | `[text](url)` 格式                          |
| 表格头行      | `(?:\|[^           | \n]*)+\|[\r\n]+\s*(?:\|\s*:?-{3,}:?\s*)+\|` | 表格标题行+分隔行 |
| 表格数据行    | `(?:\|[^           | \n]*)+\|[\r\n]+`                            | 表格数据行        |
| 代码块头      | ` ```lang\n... `   | 代码块开始行                                |

### 7.3 split_text 完整流程

```
split_text(text)
    ↓
Step 1: _split(text)
  → 按分隔符递归切分（\n → 。→ 空格 → 字符级别）
  → 每个 split 长度 ≤ chunk_size
  → 结果：List[str]（含分隔符）
    ↓
Step 2: _split_protected(text)
  → 匹配所有受保护内容的位置区间
  → 去重/去重叠（按 start 升序，按 length 降序）
  → 结果：List[(start_pos, protected_text)]
    ↓
Step 3: _join(splits, protect)
  → 遍历 splits，对受保护内容所在 split 进行特殊处理
  → 确保受保护内容不被拆分，前后文本与保护内容独立
  → 结果：List[str]（重组后的 splits）
  → 断言：''.join(result) == 原始 text（完整性保证）
    ↓
Step 4: _merge(splits) → 生成最终切片
  → 逐个累积 splits 到当前 chunk
  → 当 chunk_len + split_len + headers_len > chunk_size:
      → 保存当前 chunk
      → 保留 chunk_overlap 长度的内容作为下一 chunk 的开头
  → Header 追踪（HeaderTracker）：
      → 遇到 Markdown 标题（# ## ### 等）时记录
      → 新 chunk 开始时自动前置当前标题路径（保持上下文）
  → 结果：List[(start, end, chunk_text)]
    ↓
返回：List[Tuple[int, int, str]]  ← (start_pos, end_pos, chunk_text)
```

### 7.4 HeaderTracker（标题追踪）

```python
# docreader/splitter/header_hook.py
class HeaderTracker:
    """跟踪 Markdown 标题层级，为每个 chunk 提供上下文"""

    def update(self, split: str):
        """检测 split 中是否包含 Markdown 标题"""
        # 识别 # 一级标题, ## 二级标题, ### 三级标题
        # 更新内部标题栈

    def get_headers(self) -> str:
        """返回当前标题路径（用于 chunk 前置）"""
        # 例如: "# 产品文档\n## 安装指南\n"
```

**作用**：新 chunk 开始时自动注入当前标题上下文，使每个切片都包含完整的标题路径，提升检索召回质量。

### 7.5 切片还原（restore_text）

```python
def restore_text(self, chunks) -> str:
    """从切片还原原始文本（用于调试验证）"""
    # 按 end 位置排序
    # 累积每个 chunk 中未被前一 chunk 覆盖的新内容
    # 还原后与原始文本完全一致
```

---

## 8. 图片处理与存储

### 8.1 图片提取流程

```
文档解析（PDF/DOCX/DOC）
    ↓
提取内嵌图片 → base64 编码
    ↓
Document.images = {"图1.png": "base64数据...", ...}
    ↓
gRPC 响应（inline 传输）
    ↓
Go 主应用接收 ImageRef.image_data（字节数据）
    ↓
Go 侧持久化：上传到对象存储（MinIO/COS/本地）
    ↓
将存储 URL 替换文档 Markdown 中的图片引用
```

### 8.2 Python 侧存储实现（storage.py）

docreader Python 侧保留了存储实现（供独立运行模式使用），但在 gRPC 模式下，图片以内联字节形式传输给 Go 应用处理：

| 存储类型    | 类              | 环境变量                                                                                |
| ----------- | --------------- | --------------------------------------------------------------------------------------- |
| MinIO       | `MinioStorage`  | `MINIO_ENDPOINT`, `MINIO_ACCESS_KEY_ID`, `MINIO_SECRET_ACCESS_KEY`, `MINIO_BUCKET_NAME` |
| 腾讯云 COS  | `CosStorage`    | `COS_SECRET_ID`, `COS_SECRET_KEY`, `COS_REGION`, `COS_BUCKET_NAME`, `COS_APP_ID`        |
| 本地文件    | `LocalStorage`  | `LOCAL_STORAGE_BASE_DIR`（默认 `/data/files`）, `LOCAL_STORAGE_URL_PREFIX`              |
| Base64 内联 | `Base64Storage` | 无（直接 `data:image/png;base64,...`）                                                  |
| 空实现      | `DummyStorage`  | 所有上传返回空字符串                                                                    |

```python
def create_storage(storage_config=None):
    # 优先级: storage_config.provider → STORAGE_TYPE 环境变量 → local
    storage_type = ...
    if storage_type == "minio": return MinioStorage(storage_config)
    elif storage_type == "cos": return CosStorage(storage_config)
    elif storage_type == "local": return LocalStorage(storage_config)
    elif storage_type == "base64": return Base64Storage()
    return DummyStorage()
```

---

## 9. gRPC 服务配置

### 9.1 服务器配置（main.py）

```python
def main():
    server = grpc.server(
        futures.ThreadPoolExecutor(max_workers=CONFIG.grpc_max_workers),
        options=[
            ("grpc.max_send_message_length", CONFIG.grpc_max_file_size_mb),
            ("grpc.max_receive_message_length", CONFIG.grpc_max_file_size_mb),
        ],
    )
    server.add_insecure_port(f"[::]:{CONFIG.grpc_port}")
    server.start()
    server.wait_for_termination()
```

### 9.2 服务健康检查

使用 `grpc_health.v1` 标准健康检查协议，支持：
- `Check()`：单次健康状态查询
- Kubernetes liveness/readiness 探针集成

### 9.3 docreader 配置参数

| 环境变量                | 默认值 | 说明                       |
| ----------------------- | ------ | -------------------------- |
| `GRPC_PORT`             | 50051  | gRPC 监听端口              |
| `GRPC_MAX_WORKERS`      | 10     | 线程池大小                 |
| `GRPC_MAX_FILE_SIZE_MB` | 50MB   | 最大消息大小               |
| `LOG_LEVEL`             | INFO   | 日志级别                   |
| `EXTERNAL_HTTPS_PROXY`  | 空     | 外部 HTTPS 代理（防 SSRF） |
| `STORAGE_TYPE`          | local  | 存储类型                   |
| `MAX_FILE_SIZE_MB`      | 50     | Go 客户端最大消息大小      |

---

## 10. 完整文档导入流程

```
用户上传文件（Go HTTP 接口）
    ↓
1. Go 主应用接收文件字节数据
    ↓
2. 调用 docreaderClient.Read(ReadRequest{
       file_name: "xxx.pdf",
       file_type: "pdf",
       file_content: <bytes>,
       config: {parser_engine: "builtin"}
   })
    ↓
3. docreader gRPC Server 接收请求
    ↓
4. Parser.parse_file(file_name, file_type, content, parser_engine)
    ↓
5. ParserEngineRegistry.get_parser_class("builtin", "pdf") → PDFParser
    ↓
6. PDFParser.parse(content) → Document{content: markdown, images: {}}
    ↓
7. _resolve_images(result.images)
   → 图片 base64 解码 → ImageRef{filename, mime_type, image_data}
    ↓
8. gRPC 返回 ReadResponse{markdown_content, image_refs}
    ↓
9. Go 侧：接收 markdown_content（Markdown 文本）
    ↓
10. Go 侧：上传图片到 MinIO/COS，获取 URL
    ↓
11. 替换 markdown 中的图片引用为 URL
    ↓
12. TextSplitter.split_text(markdown)
    → chunk_size=512（可配置）
    → chunk_overlap=100（可配置）
    → 保护公式、表格、代码块不被拆分
    ↓
13. 每个 Chunk：
    a. 调用 Embedding 模型生成向量（halfvec，FP16）
    b. 提取关键词（用于 BM25 索引）
    c. 插入 embeddings 表（PostgreSQL/向量数据库）
    ↓
14. 文档状态更新：pending → ready
```

---

## 11. 解析引擎可用性检查

`list_engines()` 方法可检查各引擎的可用性，用于前端引擎选择器：

```python
def list_engines(self, overrides=None) -> List[Dict]:
    result = []
    for name, parsers in self._engines.items():
        available = True
        unavailable_reason = ""
        check = self._check_available.get(name)
        if check is not None:
            available, unavailable_reason = check(overrides)
        result.append({
            "name": name,
            "description": ...,
            "file_types": sorted(parsers.keys()),
            "available": available,
            "unavailable_reason": unavailable_reason,
        })
    return result
```

MinerU 引擎的可用性由 Go 侧的引擎注册表管理（`docparser.ListAllEngines`），Python 侧不再维护。

---

## 12. 错误处理与容错机制

### 12.1 解析级别容错

- DOC 文件：三步降级（LibreOffice → antiword → 空内容）
- 空内容检测：`if not result.content` → 记录警告，返回 `ReadResponse(error=...)`
- 代理机制：外部命令调用强制通过 `128.0.0.1:1`（无效代理），防止 SSRF

### 12.2 编码容错

```python
def to_valid_utf8_text(s: Optional[str]) -> str:
    """清理代理字符（surrogate pairs），确保合法 UTF-8 输出"""
    s = _SURROGATE_RE.sub("\ufffd", s)  # 替换 U+D800~U+DFFF
    return s.encode("utf-8", errors="replace").decode("utf-8")
```

### 12.3 切片验证

TextSplitter 提供 `_validate_chunks()` 验证方法（调试模式）：
- 验证切片起始位置递增顺序
- 验证切片还原结果与原文本完全一致
- 验证失败时：将详细信息保存到 `/tmp/chunk_error_YYYYMMDD_HHMMSS.md`

---

## 13. 支持格式汇总

| 格式         | 扩展名       | 引擎                 | 图片支持          | 表格支持 |
| ------------ | ------------ | -------------------- | ----------------- | -------- |
| Word 2007+   | .docx        | builtin              | ✅                 | ✅        |
| Word 97-2003 | .doc         | builtin              | ✅（LibreOffice）  | ✅        |
| PDF          | .pdf         | builtin / markitdown | ✅                 | ✅        |
| Markdown     | .md          | builtin / markitdown | ✅（URL 引用）     | ✅        |
| Excel        | .xlsx/.xls   | builtin / markitdown | ❌                 | ✅        |
| PowerPoint   | .pptx/.ppt   | markitdown           | ✅                 | ✅        |
| CSV          | .csv         | markitdown           | ❌                 | ✅        |
| 图片         | .jpg/.png 等 | builtin              | N/A（本身是图片） | ❌        |
| 网页 URL     | -            | builtin              | ✅（URL 保留）     | ✅        |
