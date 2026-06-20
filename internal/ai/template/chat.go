package template

import (
	"context"

	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

// ChatTemplateInput 是聊天模板输入。
type ChatTemplateInput struct {
	Query   string            `json:"query"`
	History []*schema.Message `json:"history"`
}

// NewChatTemplate 创建普通聊天模板节点，不包含工具说明。
// 输出顺序为：系统提示词、会话历史、当前用户消息。
func NewChatTemplate(systemPrompt string) (*compose.Graph[ChatTemplateInput, []*schema.Message], error) {
	graph := compose.NewGraph[ChatTemplateInput, []*schema.Message]()

	// 构建消息模板函数。
	graph.AddNode("template", compose.InvokableFunc(func(
		ctx context.Context,
		input ChatTemplateInput,
		opts ...compose.Option,
	) ([]*schema.Message, error) {
		messages := make([]*schema.Message, 0, len(input.History)+2)

		// 系统提示词。
		if systemPrompt != "" {
			messages = append(messages, &schema.Message{
				Role:    schema.System,
				Content: systemPrompt,
			})
		}

		// 历史消息。
		messages = append(messages, input.History...)

		// 当前用户问题。
		messages = append(messages, &schema.Message{
			Role:    schema.User,
			Content: input.Query,
		})

		return messages, nil
	}))

	graph.AddEdge(compose.START, "template")
	graph.AddEdge("template", compose.END)

	return graph, nil
}
