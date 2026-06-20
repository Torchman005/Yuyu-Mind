package pipeline

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/yuyu-mind/backend/internal/ai/template"
)

// AgentGraphInput 是 Agent 图的输入。
type AgentGraphInput = template.AgentTemplateInput

// AgentGraphOutput 是 Agent 图的最终输出。
type AgentGraphOutput = *schema.Message

const (
	nodeTemplate     = "template"
	nodeChatModel    = "chat_model"
	nodeToolExec     = "tool_exec"
	nodeHasToolCalls = "has_tool_calls"
)

// BuildAgentGraph 创建带工具调用循环的 Graph Pipeline，采用 ReAct 模式。
//
// 流程：
//
//	ChatTemplate -> ChatModel -> 是否有工具调用
//	                            是 -> 执行工具 -> ChatModel（循环）
//	                            否 -> END
func BuildAgentGraph(
	chatModel model.ChatModel,
	tmpl *compose.Graph[AgentGraphInput, []*schema.Message],
	tools []tool.BaseTool,
	maxIterations int,
) (compose.Runnable[AgentGraphInput, AgentGraphOutput], error) {
	if maxIterations <= 0 {
		maxIterations = 10
	}

	graph := compose.NewGraph[AgentGraphInput, AgentGraphOutput]()

	// 模板节点。
	templateRunnable, err := tmpl.Compile(context.Background())
	if err != nil {
		return nil, fmt.Errorf("compile agent template: %w", err)
	}
	graph.AddNode(nodeTemplate, compose.AnyRunnable(templateRunnable))

	// 模型节点。
	graph.AddNode(nodeChatModel, compose.AnyRunnable(
		compose.InvokableFunc(func(ctx context.Context, input []*schema.Message, opts ...compose.Option) (*schema.Message, error) {
			return chatModel.Generate(ctx, input)
		}),
	))

	// 工具执行节点。
	graph.AddNode(nodeToolExec, compose.InvokableFunc(func(
		ctx context.Context,
		input *schema.Message,
		opts ...compose.Option,
	) ([]*schema.Message, error) {
		return executeToolCalls(ctx, input, tools)
	}))

	// 固定边。
	graph.AddEdge(compose.START, nodeTemplate)
	graph.AddEdge(nodeTemplate, nodeChatModel)

	// 条件分支：模型回复后判断是否需要调用工具。
	graph.AddBranch(nodeChatModel, compose.Branch{
		Condition: func(ctx context.Context, input *schema.Message) (string, error) {
			if len(input.ToolCalls) > 0 {
				return nodeToolExec, nil
			}
			return compose.END, nil
		},
		Branches: map[string]*compose.Graph[AgentGraphInput, AgentGraphOutput]{
			nodeToolExec: func() *compose.Graph[AgentGraphInput, AgentGraphOutput] {
				g := compose.NewGraph[AgentGraphInput, AgentGraphOutput]()
				g.AddNode("tool_then_model", compose.InvokableFunc(func(
					ctx context.Context,
					input *schema.Message,
					opts ...compose.Option,
				) (*schema.Message, error) {
					// 该路径由下方边定义承接。
					return input, nil
				}))
				return g
			}(),
			compose.END: func() *compose.Graph[AgentGraphInput, AgentGraphOutput] {
				return compose.NewGraph[AgentGraphInput, AgentGraphOutput]()
			}(),
		},
	})

	// 工具执行完成后回到模型节点。
	graph.AddEdge(nodeToolExec, nodeChatModel)

	runnable, err := graph.Compile(context.Background())
	if err != nil {
		return nil, fmt.Errorf("compile agent graph: %w", err)
	}

	return runnable, nil
}

// executeToolCalls 执行消息中的所有工具调用，并返回工具结果消息。
func executeToolCalls(ctx context.Context, msg *schema.Message, tools []tool.BaseTool) ([]*schema.Message, error) {
	// 构建工具名称到实例的索引。
	toolMap := make(map[string]tool.BaseTool)
	for _, t := range tools {
		info, err := t.Info(ctx)
		if err != nil {
			continue
		}
		toolMap[info.Name] = t
	}

	var results []*schema.Message
	results = append(results, msg) // 保留带工具调用的助手消息。

	for _, tc := range msg.ToolCalls {
		t, ok := toolMap[tc.Name]
		if !ok {
			results = append(results, &schema.Message{
				Role:       schema.Tool,
				Content:    fmt.Sprintf("Error: tool %q not found", tc.Name),
				ToolCallID: tc.ID,
			})
			continue
		}

		result, err := t.InvokableRun(ctx, tc.Function.Arguments)
		if err != nil {
			result = fmt.Sprintf("Error executing tool %q: %v", tc.Name, err)
		}

		results = append(results, &schema.Message{
			Role:       schema.Tool,
			Content:    result,
			ToolCallID: tc.ID,
		})
	}

	return results, nil
}
