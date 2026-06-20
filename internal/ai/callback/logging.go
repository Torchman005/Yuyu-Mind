package callback

import (
	"context"
	"log/slog"

	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/schema"
)

// LoggingHandler 实现 Eino 回调处理器，用于应用级日志。
type LoggingHandler struct {
	logger *slog.Logger
}

// NewLoggingHandler 创建日志回调处理器。
func NewLoggingHandler(logger *slog.Logger) *LoggingHandler {
	return &LoggingHandler{logger: logger}
}

// Register 注册全局 Eino 日志回调。
func Register(logger *slog.Logger) {
	handler := NewLoggingHandler(logger)
	callbacks.AppendGlobalHandlers(handler)
}

// 以下方法实现 callbacks.Handler 接口，会在 Pipeline 执行的不同阶段被 Eino 调用。

func (h *LoggingHandler) OnStart(ctx context.Context, info *callbacks.RunInfo, input callbacks.CallbackInput) context.Context {
	h.logger.Debug("eino node started",
		"component", info.Component,
		"type", info.Type,
		"name", info.Name,
	)
	return ctx
}

func (h *LoggingHandler) OnEnd(ctx context.Context, info *callbacks.RunInfo, output callbacks.CallbackOutput) context.Context {
	h.logger.Debug("eino node finished",
		"component", info.Component,
		"type", info.Type,
		"name", info.Name,
	)
	return ctx
}

func (h *LoggingHandler) OnError(ctx context.Context, info *callbacks.RunInfo, err error) context.Context {
	h.logger.Error("eino node error",
		"component", info.Component,
		"type", info.Type,
		"name", info.Name,
		"error", err,
	)
	return ctx
}

func (h *LoggingHandler) OnStartWithStreamInput(ctx context.Context, info *callbacks.RunInfo, input *schema.StreamReader[callbacks.CallbackInput]) context.Context {
	h.logger.Debug("eino node started (stream input)",
		"component", info.Component,
		"name", info.Name,
	)
	return ctx
}

func (h *LoggingHandler) OnEndWithStreamOutput(ctx context.Context, info *callbacks.RunInfo, output *schema.StreamReader[callbacks.CallbackOutput]) context.Context {
	h.logger.Debug("eino node finished (stream output)",
		"component", info.Component,
		"name", info.Name,
	)
	return ctx
}
