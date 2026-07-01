package pipeline

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/yuyu-mind/backend/internal/ai/template"
)

type ChatChainInput = template.ChatTemplateInput

func BuildChatChain(chatModel model.BaseChatModel, tmpl *compose.Graph[ChatChainInput, []*schema.Message]) (compose.Runnable[ChatChainInput, *schema.Message], error) {
	chain := compose.NewChain[ChatChainInput, *schema.Message]()
	chain.AppendGraph(tmpl)
	chain.AppendChatModel(chatModel)

	runnable, err := chain.Compile(context.Background())
	if err != nil {
		return nil, fmt.Errorf("compile chat chain: %w", err)
	}
	return runnable, nil
}
