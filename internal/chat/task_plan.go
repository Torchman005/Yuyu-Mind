package chat

import (
	"strings"

	"github.com/yuyu-mind/backend/internal/agent"
)

// ToTaskSpec 把 Planner 输出的任务包转换为异步任务系统的 TaskSpec。
// Workspace 为空时由宿主（TaskSubmitter）在提交时填充默认工作区。
func (p TaskPlan) ToTaskSpec(conversationID string) agent.TaskSpec {
	title := strings.TrimSpace(p.Title)
	if title == "" {
		title = strings.TrimSpace(p.Goal)
	}
	return agent.TaskSpec{
		ConversationID: conversationID,
		Title:          title,
		Goal:           strings.TrimSpace(p.Goal),
		Instructions:   strings.TrimSpace(p.Instructions),
		Workspace:      strings.TrimSpace(p.Workspace),
		Constraints:    p.Constraints,
		AllowedActions: p.AllowedActions,
	}
}
