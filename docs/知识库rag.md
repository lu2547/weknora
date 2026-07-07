# 目标
复刻weknora的rag能力和react能力
# 步骤：
1、ark-agentic 打个包，然后新建个项目叫knowledge_agent,引用ark-agentic包去做这个agent
2、主体是复刻weknora里面的4个tools，在tools目录里
{Name: ToolGrepChunks, Label: "关键词搜索", Description: "快速定位包含特定关键词的文档和分块"},
{Name: ToolKnowledgeSearch, Label: "语义搜索", Description: "理解问题并查找语义相关内容"},
{Name: ToolListKnowledgeChunks, Label: "查看文档分块", Description: "获取文档完整分块内容"},
{Name: ToolGetDocumentInfo, Label: "获取文档信息", Description: "查看文档元数据"},
3、基于weknora的react和rag出2个skills，注意仔细读weknora的源码和提示词复刻
4、模型调用需要注意，复用create_chat_model能力，这个create_chat_model方法支持PA_JT、PA_ZQ、PA_SX还有普通的openai4种，用普通openai的模式，下面是embbeding、rerank、text模型信息，所有配置文件外置到.env
## 1、emebdding：
模型名：text-embedding-v4
url:https://dashscope.aliyuncs.com/compatible-mode/v1
api-key:sk-b1b08054a3274a3198998bee08ab598f
## 2、rerank:
模型名：qwen3-vl-rerank
url：https://dashscope.aliyuncs.com/api/v1/services/rerank/text-rerank/text-rerank
api-key:sk-b1b08054a3274a3198998bee08ab598f
## 3、text：
模型名：qwen3.5-122b-a10b
url:https://dashscope.aliyuncs.com/compatible-mode/v1
api-key:sk-b1b08054a3274a3198998bee08ab598f

# 要求：
1、都在intelligent_qa_agent目录下实现，结构参考agents/insurance
2、1:1复刻，就是说底层的表和milvus都是复用weknora的，不能重新造，我要实现的是weknora里录入知识，在这个agent里能复用weknora的rag逻辑去做rag
3、抽象下要引用的数据库、milvus等工具，做成内置工具类，配置文件外置到.env里面
4、未来我要把tools部分封装成标准工具包的，skills也是2个通用skills
5、这里有个非常重要的点，用的是weknora+milvus，不是用的pg做向量库


我需要出一个知识库管理的规划，具体如下：
# 目标
出一个完整的知识库管理规划，需要一个月内完成开发并上线第一版。
需要包含需求、功能设计、表结构设计、架构设计等开发必备设计，并生成故事拆分到前后端，然后排期。

# 功能
包括权限管理、树状层级管理、知识管理、知识质量校验、agent管理、基于agent的智能问答
- 树状层级管理
用户可以自定义树状结构（tag_tree），用来做知识的层级，每个层级都要定义知识资产属主，如果在一级定义了属主，那么它的子节点就都是这个属主
- 知识管理
  每个层级下都可以有知识，主要提供知识录入和管理功能，逻辑其实和weknora一样，只是多了层级概念
- 知识文档质检
  基于ragas设计一下
- agent管理
  这个其实就是复刻ark-agentic里面的studio的内容
- 基于agent的智能问答
  这个其实就是weknora的对话功能，复刻一下，不用配置agent
- 权限管理
  um账号和角色绑定、角色需要和这个树状结构的tag绑定

# 技术选型
前端vue3+ts、 后端web springboot + nacos
数据库 pg，向量库 milvus、通讯 rocketmq、缓存redis


# 人员安排
4个前端、4个web开发、2个rag开发、2个文档解析入库开发

文档都给出md的，需要流程图的用mm图