package chat

import "testing"

func TestTaskPlanToTaskSpec(t *testing.T) {
	plan := TaskPlan{
		Goal:           "创建说明文件",
		Instructions:   "创建 notes.md，内容说明项目架构。",
		Constraints:    []string{"不要改无关文件"},
		AllowedActions: []string{"write_file"},
	}
	spec := plan.ToTaskSpec("conv-1")

	if spec.ConversationID != "conv-1" {
		t.Fatalf("conversation id = %q", spec.ConversationID)
	}
	if spec.Title != "创建说明文件" {
		t.Fatalf("title should default to goal, got %q", spec.Title)
	}
	if spec.Goal != "创建说明文件" || spec.Instructions != "创建 notes.md，内容说明项目架构。" {
		t.Fatalf("goal/instructions mismatch: %+v", spec)
	}
	if spec.Workspace != "" {
		t.Fatalf("workspace should pass through empty, got %q", spec.Workspace)
	}
	if len(spec.AllowedActions) != 1 || spec.AllowedActions[0] != "write_file" {
		t.Fatalf("allowed actions mismatch: %v", spec.AllowedActions)
	}
	if len(spec.Constraints) != 1 || spec.Constraints[0] != "不要改无关文件" {
		t.Fatalf("constraints mismatch: %v", spec.Constraints)
	}
}

func TestTaskPlanToTaskSpecTitleOverride(t *testing.T) {
	spec := TaskPlan{Title: "自定义标题", Goal: "目标", Instructions: "步骤"}.ToTaskSpec("c")
	if spec.Title != "自定义标题" {
		t.Fatalf("title = %q", spec.Title)
	}
}
