// Package assistant 提供 AI 学习助手核心功能
package assistant

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino-ext/components/model/ark"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"

	"github.com/Jackreceive/ai-learning-agent/callback"
	"github.com/Jackreceive/ai-learning-agent/config"
	"github.com/Jackreceive/ai-learning-agent/model"
	"github.com/Jackreceive/ai-learning-agent/rag"
	"github.com/Jackreceive/ai-learning-agent/tools"
)

// Assistant AI 学习助手
type Assistant struct {
	chatModel *ark.ChatModel
	rag       *rag.Components
	tools     []tool.BaseTool
	history   []*schema.Message
}

// New 创建助手实例
func New(ctx context.Context, cfg *config.Config) (*Assistant, error) {
	// 创建对话模型
	chatModel, err := model.NewChatModel(ctx, &cfg.ARK)
	if err != nil {
		return nil, fmt.Errorf("创建模型失败: %w", err)
	}

	components, err := rag.New(ctx, cfg)
	if err != nil {
		return nil, err
	}

	// 获取工具
	allTools := tools.GetAllTools()

	return &Assistant{
		chatModel: chatModel,
		rag:       components,
		tools:     allTools,
		history:   make([]*schema.Message, 0),
	}, nil
}

// Close 关闭助手
func (a *Assistant) Close() {
	_ = a.rag.Client.Close(context.Background())
}

// ChatStream 流式对话
func (a *Assistant) ChatStream(ctx context.Context, query string) error {
	messages := []*schema.Message{
		schema.SystemMessage("你是一个友好的 AI 学习助手，帮助用户解答各种问题。回答要简洁清晰。"),
	}
	messages = append(messages, a.history...)
	messages = append(messages, schema.UserMessage(query))

	stream, err := a.chatModel.Stream(ctx, messages)
	if err != nil {
		return err
	}
	defer stream.Close()

	fullContent, err := callback.CollectStream(stream, &callback.StreamCallback{
		OnChunk: func(content string) {
			fmt.Print(content)
		},
	})
	if err != nil {
		return err
	}

	// 更新历史
	a.addHistory(query, fullContent)
	return nil
}

// KnowledgeQA 知识库问答
func (a *Assistant) KnowledgeQA(ctx context.Context, query string) error {
	// 检索相关知识
	docs, err := a.rag.Retriever.Retrieve(ctx, query)
	ragContext := rag.BuildContext(docs)
	if err != nil {
		return err
	}

	// 构建消息
	systemPrompt := `你是一个基于知识库的问答助手。请根据提供的参考资料回答问题。

参考资料：
%s

回答原则：
1. 优先使用参考资料中的信息
2. 如果参考资料不足以回答，请明确说明
3. 回答要简洁准确`

	messages := []*schema.Message{
		schema.SystemMessage(fmt.Sprintf(systemPrompt, ragContext)),
		schema.UserMessage(query),
	}

	// 流式输出
	stream, err := a.chatModel.Stream(ctx, messages)
	if err != nil {
		return err
	}
	defer stream.Close()

	_, err = callback.CollectStream(stream, &callback.StreamCallback{
		OnChunk: func(content string) {
			fmt.Print(content)
		},
	})

	// 显示引用来源
	if len(docs) > 0 {
		fmt.Printf("\n\n📎 参考了 %d 条知识", len(docs))
	}

	return err
}

// AddKnowledge 添加知识
func (a *Assistant) AddKnowledge(ctx context.Context, content string) error {
	if strings.TrimSpace(content) == "" {
		return fmt.Errorf("内容不能为空")
	}
	docs, err := a.rag.Transformer.Transform(ctx, []*schema.Document{{ID: uuid.NewString(), Content: content, MetaData: map[string]any{"source": "user"}}})
	if err != nil {
		return err
	}
	_, err = a.rag.Indexer.Store(ctx, docs)
	return err
}

// ClearKnowledge 清空知识库
func (a *Assistant) ClearKnowledge(ctx context.Context) error {
	return a.rag.Clear(ctx)
}

func (a *Assistant) KnowledgeCount(ctx context.Context) (int64, error) {
	return a.rag.Count(ctx)
}

// UseTool 使用工具
func (a *Assistant) UseTool(ctx context.Context, query string) (string, error) {
	agent, err := react.NewAgent(ctx, &react.AgentConfig{
		ToolCallingModel: a.chatModel, ToolsConfig: compose.ToolsNodeConfig{Tools: a.tools}, MaxStep: 10,
	})
	if err != nil {
		return "", err
	}
	response, err := agent.Generate(ctx, []*schema.Message{
		schema.SystemMessage("根据用户需求调用工具，并用工具结果回答问题。"), schema.UserMessage(query),
	})
	if err != nil {
		return "", err
	}
	return response.Content, nil
}

// CodeAssist 代码助手
func (a *Assistant) CodeAssist(ctx context.Context, query string) error {
	messages := []*schema.Message{
		schema.SystemMessage(`你是一个专业的编程助手，精通多种编程语言。
回答要求：
1. 代码示例使用 markdown 代码块
2. 解释要简洁清晰
3. 注意代码的正确性和最佳实践`),
		schema.UserMessage(query),
	}

	stream, err := a.chatModel.Stream(ctx, messages)
	if err != nil {
		return err
	}
	defer stream.Close()

	_, err = callback.CollectStream(stream, &callback.StreamCallback{
		OnChunk: func(content string) {
			fmt.Print(content)
		},
	})
	return err
}

// Translate 翻译
func (a *Assistant) Translate(ctx context.Context, text string) error {
	messages := []*schema.Message{
		schema.SystemMessage(`你是一个专业的翻译助手。
规则：
1. 如果输入是中文，翻译成英文
2. 如果输入是英文，翻译成中文
3. 只输出翻译结果，不要解释`),
		schema.UserMessage(text),
	}

	stream, err := a.chatModel.Stream(ctx, messages)
	if err != nil {
		return err
	}
	defer stream.Close()

	_, err = callback.CollectStream(stream, &callback.StreamCallback{
		OnChunk: func(content string) {
			fmt.Print(content)
		},
	})
	return err
}

func (a *Assistant) addHistory(query, response string) {
	a.history = append(a.history, schema.UserMessage(query))
	a.history = append(a.history, schema.AssistantMessage(response, nil))

	// 限制历史长度
	if len(a.history) > 10 {
		a.history = a.history[len(a.history)-10:]
	}
}
