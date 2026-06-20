package memory

import (
	"github.com/cloudwego/eino/schema"
)

// Window 按配置裁剪会话历史。
type Window struct {
	MaxTurns int // 最大用户-助手对话轮数，0 表示不限制
}

// NewWindow 创建滑动窗口。
func NewWindow(maxTurns int) *Window {
	return &Window{MaxTurns: maxTurns}
}

// Truncate 保留系统消息和最近 N 轮历史。
// 一轮指用户消息到助手消息的交换，工具消息归入其父助手消息所在轮次。
func (w *Window) Truncate(messages []*schema.Message) []*schema.Message {
	if w.MaxTurns <= 0 || len(messages) == 0 {
		return messages
	}

	// 将系统消息和普通消息拆开处理。
	var systemMsgs []*schema.Message
	var rest []*schema.Message
	for _, m := range messages {
		if m.Role == schema.System {
			systemMsgs = append(systemMsgs, m)
		} else {
			rest = append(rest, m)
		}
	}

	// 从后向前统计最近轮次，一条用户消息视为新一轮开始。
	turns := 0
	turnStartIdx := 0
	for i := len(rest) - 1; i >= 0; i-- {
		if rest[i].Role == schema.User {
			turns++
			if turns == w.MaxTurns {
				turnStartIdx = i
				break
			}
		}
		if i == 0 {
			turnStartIdx = 0
		}
	}

	// 未超过最大轮数时直接返回原始消息。
	if turns < w.MaxTurns {
		return messages
	}

	// 重新组装：系统消息 + 最近轮次。
	result := make([]*schema.Message, 0, len(systemMsgs)+len(rest)-turnStartIdx)
	result = append(result, systemMsgs...)
	result = append(result, rest[turnStartIdx:]...)

	return result
}
