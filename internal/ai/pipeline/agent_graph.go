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

type AgentGraphInput = template.AgentTemplateInput
type AgentGraphOutput = *schema.Message

func BuildAgentGraph(
	chatModel model.ToolCallingChatModel,
	tmpl *compose.Graph[AgentGraphInput, []*schema.Message],
	tools []tool.BaseTool,
	maxIterations int,
) (compose.Runnable[AgentGraphInput, AgentGraphOutput], error) {
	if maxIterations <= 0 {
		maxIterations = 10
	}

	templateRunnable, err := tmpl.Compile(context.Background())
	if err != nil {
		return nil, fmt.Errorf("compile agent template: %w", err)
	}

	toolInfos, err := collectToolInfos(context.Background(), tools)
	if err != nil {
		return nil, err
	}
	toolModel, err := chatModel.WithTools(toolInfos)
	if err != nil {
		return nil, fmt.Errorf("bind tools to chat model: %w", err)
	}

	chain := compose.NewChain[AgentGraphInput, AgentGraphOutput]()
	chain.AppendLambda(compose.InvokableLambda(func(ctx context.Context, input AgentGraphInput) (AgentGraphOutput, error) {
		messages, err := templateRunnable.Invoke(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("run agent template: %w", err)
		}

		var reply *schema.Message
		for i := 0; i < maxIterations; i++ {
			reply, err = toolModel.Generate(ctx, messages)
			if err != nil {
				return nil, fmt.Errorf("generate agent reply: %w", err)
			}
			if len(reply.ToolCalls) == 0 {
				return reply, nil
			}

			toolMessages, err := executeToolCalls(ctx, reply, tools)
			if err != nil {
				return nil, err
			}
			messages = append(messages, toolMessages...)
		}

		return reply, fmt.Errorf("agent reached max tool iterations: %d", maxIterations)
	}))

	runnable, err := chain.Compile(context.Background())
	if err != nil {
		return nil, fmt.Errorf("compile agent graph: %w", err)
	}
	return runnable, nil
}

func collectToolInfos(ctx context.Context, tools []tool.BaseTool) ([]*schema.ToolInfo, error) {
	toolInfos := make([]*schema.ToolInfo, 0, len(tools))
	for _, t := range tools {
		info, err := t.Info(ctx)
		if err != nil {
			return nil, fmt.Errorf("get tool info: %w", err)
		}
		toolInfos = append(toolInfos, info)
	}
	return toolInfos, nil
}

func executeToolCalls(ctx context.Context, msg *schema.Message, tools []tool.BaseTool) ([]*schema.Message, error) {
	toolMap := make(map[string]tool.BaseTool)
	for _, t := range tools {
		info, err := t.Info(ctx)
		if err != nil {
			continue
		}
		toolMap[info.Name] = t
	}

	results := []*schema.Message{msg}
	for _, tc := range msg.ToolCalls {
		t, ok := toolMap[tc.Function.Name]
		if !ok {
			results = append(results, &schema.Message{
				Role:       schema.Tool,
				Content:    fmt.Sprintf("Error: tool %q not found", tc.Function.Name),
				ToolCallID: tc.ID,
			})
			continue
		}

		invokable, ok := t.(tool.InvokableTool)
		if !ok {
			results = append(results, &schema.Message{
				Role:       schema.Tool,
				Content:    fmt.Sprintf("Error: tool %q is not invokable", tc.Function.Name),
				ToolCallID: tc.ID,
			})
			continue
		}

		result, err := invokable.InvokableRun(ctx, tc.Function.Arguments)
		if err != nil {
			result = fmt.Sprintf("Error executing tool %q: %v", tc.Function.Name, err)
		}

		results = append(results, &schema.Message{
			Role:       schema.Tool,
			Content:    result,
			ToolCallID: tc.ID,
		})
	}
	return results, nil
}
