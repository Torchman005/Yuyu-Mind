package usage

import (
	"context"
	"io"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// TrackedChatModel 包装真实模型，在不改变调用方 API 的前提下收集 token 用量。
type TrackedChatModel struct {
	inner     model.ChatModel
	collector *Collector
}

// NewTrackedChatModel 创建带 token 追踪能力的模型包装器。
func NewTrackedChatModel(inner model.ChatModel, collector *Collector) *TrackedChatModel {
	return &TrackedChatModel{inner: inner, collector: collector}
}

// Generate 调用真实模型，并在响应返回后记录 token 用量。
func (m *TrackedChatModel) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	msg, err := m.inner.Generate(ctx, input, opts...)
	if err == nil {
		m.collector.AddMessage(msg)
	}
	return msg, err
}

// Stream 代理真实流式输出，并在流结束时聚合 chunk 中携带的 token 用量。
func (m *TrackedChatModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	reader, err := m.inner.Stream(ctx, input, opts...)
	if err != nil {
		return nil, err
	}

	outReader, outWriter := schema.Pipe[*schema.Message](8)
	go func() {
		defer outWriter.Close()
		defer reader.Close()

		chunks := make([]*schema.Message, 0, 32)
		for {
			chunk, recvErr := reader.Recv()
			if recvErr == io.EOF {
				m.collectStreamUsage(chunks)
				return
			}
			if recvErr != nil {
				outWriter.Send(nil, recvErr)
				return
			}

			chunks = append(chunks, chunk)
			if closed := outWriter.Send(chunk, nil); closed {
				return
			}
		}
	}()

	return outReader, nil
}

// BindTools 透传工具绑定，保持与原模型一致的能力。
func (m *TrackedChatModel) BindTools(tools []*schema.ToolInfo) error {
	return m.inner.BindTools(tools)
}

func (m *TrackedChatModel) collectStreamUsage(chunks []*schema.Message) {
	if len(chunks) == 0 {
		return
	}

	msg, err := schema.ConcatMessages(chunks)
	if err != nil {
		return
	}
	m.collector.AddMessage(msg)
}
