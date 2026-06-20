package template

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

// AgentTemplateInput 是 Agent 模板输入。
type AgentTemplateInput struct {
	Query   string            `json:"query"`
	History []*schema.Message `json:"history"`
}

// NewAgentTemplate 创建 Agent 模板节点，并把工具说明追加到系统提示词。
func NewAgentTemplate(systemPrompt string, toolDescriptions []string) (*compose.Graph[AgentTemplateInput, []*schema.Message], error) {
	graph := compose.NewGraph[AgentTemplateInput, []*schema.Message]()

	// 构建带工具说明的完整系统提示词。
	fullSystemPrompt := buildAgentSystemPrompt(systemPrompt, toolDescriptions)

	graph.AddNode("template", compose.InvokableFunc(func(
		ctx context.Context,
		input AgentTemplateInput,
		opts ...compose.Option,
	) ([]*schema.Message, error) {
		messages := make([]*schema.Message, 0, len(input.History)+2)

		// 带工具说明的系统提示词。
		if fullSystemPrompt != "" {
			messages = append(messages, &schema.Message{
				Role:    schema.System,
				Content: fullSystemPrompt,
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

// buildAgentSystemPrompt 合并基础系统提示词和工具使用说明。
func buildAgentSystemPrompt(base string, toolDescs []string) string {
	var sb strings.Builder

	if base != "" {
		sb.WriteString(base)
		sb.WriteString("\n\n")
	}

	if len(toolDescs) > 0 {
		sb.WriteString("You have access to the following tools:\n\n")
		for i, desc := range toolDescs {
			sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, desc))
		}
		sb.WriteString("\nWhen you need to use a tool, respond with a tool call. ")
		sb.WriteString("When you have enough information to answer, respond directly.\n")
	}

	return sb.String()
}
