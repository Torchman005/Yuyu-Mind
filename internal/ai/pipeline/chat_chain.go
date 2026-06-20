package pipeline

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/yuyu-mind/backend/internal/ai/template"
)

// ChatChainInput 是普通聊天链路的输入。
type ChatChainInput = template.ChatTemplateInput

// BuildChatChain 创建简单链路：ChatTemplate -> ChatModel。
// 该链路用于不启用工具调用的基础聊天。
func BuildChatChain(chatModel model.ChatModel, tmpl *compose.Graph[ChatChainInput, []*schema.Message]) (compose.Runnable[ChatChainInput, *schema.Message], error) {
	chain := compose.NewChain[ChatChainInput, *schema.Message]()

	// 节点 1：模板负责组装系统提示词、历史消息和当前问题。
	templateRunnable, err := tmpl.Compile(context.Background())
	if err != nil {
		return nil, fmt.Errorf("compile chat template: %w", err)
	}
	chain = chain.AppendRunnable(templateRunnable)

	// 节点 2：模型负责生成助手回复。
	chain = chain.AppendChatModel(chatModel)

	runnable, err := chain.Compile(context.Background())
	if err != nil {
		return nil, fmt.Errorf("compile chat chain: %w", err)
	}

	return runnable, nil
}
