package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

type fakeTool struct {
	name string
	out  string
}

func (f *fakeTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{Name: f.name, Desc: "fake tool"}, nil
}

func (f *fakeTool) InvokableRun(ctx context.Context, argumentsJSON string, opts ...tool.Option) (string, error) {
	return f.out, nil
}

type fakeToolCallingModel struct {
	calls      int
	toolResult string
}

func (m *fakeToolCallingModel) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	m.calls++
	if m.calls == 1 {
		return &schema.Message{
			Role: schema.Assistant,
			ToolCalls: []schema.ToolCall{
				{ID: "call-1", Type: "function", Function: schema.FunctionCall{Name: "read_file", Arguments: `{"path":"a.txt"}`}},
			},
		}, nil
	}
	return &schema.Message{Role: schema.Assistant, Content: "任务完成：读取了文件。"}, nil
}

func (m *fakeToolCallingModel) WithTools(tools []*schema.ToolInfo) (ToolCallingModel, error) {
	return m, nil
}

type scriptedToolCallingModel struct {
	calls   int
	replies []*schema.Message
}

func (m *scriptedToolCallingModel) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	if m.calls >= len(m.replies) {
		return &schema.Message{Role: schema.Assistant, Content: "done"}, nil
	}
	reply := m.replies[m.calls]
	m.calls++
	return reply, nil
}

func (m *scriptedToolCallingModel) WithTools(tools []*schema.ToolInfo) (ToolCallingModel, error) {
	return m, nil
}

type fakeRuntime struct {
	events []string
}

func (r *fakeRuntime) Emit(ctx context.Context, eventType, level, message string, payload any) error {
	r.events = append(r.events, eventType+":"+message)
	return nil
}

func (r *fakeRuntime) LogOperation(ctx context.Context, kind, target, summary, status string, payload any) error {
	r.events = append(r.events, "op:"+summary)
	return nil
}

func (r *fakeRuntime) WaitForInput(ctx context.Context, question QuestionPayload) (string, error) {
	return "", errTaskWaitingInput
}

func (r *fakeRuntime) RequestApproval(ctx context.Context, request ApprovalRequest) (bool, error) {
	return true, nil
}

func (r *fakeRuntime) CheckCancelled(ctx context.Context) error { return nil }

func TestLLMExecutorExecute(t *testing.T) {
	fakeModel := &fakeToolCallingModel{}
	tools := []tool.BaseTool{&fakeTool{name: "read_file", out: "file content"}}
	exec := NewLLMExecutor(func(ctx context.Context) (ToolCallingModel, error) { return fakeModel, nil }, func() []tool.BaseTool { return tools })
	rt := &fakeRuntime{}

	result, err := exec.Execute(context.Background(), TaskSpec{
		Goal:           "read a file",
		Instructions:   "read a.txt",
		Workspace:      "/tmp",
		AllowedActions: []string{"read_file"},
	}, rt)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Summary != "任务完成：读取了文件。" {
		t.Fatalf("unexpected summary %q", result.Summary)
	}
	if fakeModel.calls != 2 {
		t.Fatalf("expected 2 model calls, got %d", fakeModel.calls)
	}
	if len(rt.events) < 2 {
		t.Fatalf("expected events to be emitted, got %d", len(rt.events))
	}
}

func TestFilterToolsByActions(t *testing.T) {
	tools := []tool.BaseTool{
		&fakeTool{name: "write_file"},
		&fakeTool{name: "read_file"},
		&fakeTool{name: "list_files"},
	}

	got := filterToolsByActions(tools, []string{"read_file", "list_files"})
	if len(got) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(got))
	}
	first, _ := got[0].Info(context.Background())
	second, _ := got[1].Info(context.Background())
	if first.Name != "list_files" || second.Name != "read_file" {
		t.Fatalf("unexpected order %s, %s", first.Name, second.Name)
	}

	if len(filterToolsByActions(tools, nil)) != 0 {
		t.Fatalf("expected empty tool set for empty allowed actions")
	}
}

func TestExecuteWorkerToolCalls(t *testing.T) {
	tools := []tool.BaseTool{
		&fakeTool{name: "read_file", out: "file content"},
	}
	msg := &schema.Message{
		Role: schema.Assistant,
		ToolCalls: []schema.ToolCall{
			{ID: "call-1", Type: "function", Function: schema.FunctionCall{Name: "read_file", Arguments: `{"path":"a.txt"}`}},
			{ID: "call-2", Type: "function", Function: schema.FunctionCall{Name: "write_file", Arguments: `{}`}},
		},
	}

	msgs, err := executeWorkerToolCalls(context.Background(), &fakeRuntime{}, msg, tools)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages (original + 2 tool results), got %d", len(msgs))
	}
	if msgs[1].Content != "file content" {
		t.Fatalf("unexpected tool result %q", msgs[1].Content)
	}
	if !strings.Contains(msgs[2].Content, "not allowed") {
		t.Fatalf("expected not-allowed error, got %q", msgs[2].Content)
	}
}

func TestExtractCodeReviewFromRunAgentResult(t *testing.T) {
	reply := &schema.Message{
		Role: schema.Assistant,
		ToolCalls: []schema.ToolCall{
			{ID: "call-1", Type: "function", Function: schema.FunctionCall{Name: "run_agent", Arguments: `{"task":"edit"}`}},
		},
	}
	toolMessages := []*schema.Message{
		{Role: schema.Tool, ToolCallID: "call-1", Content: `{"ok":true,"cwd":"C:\\repo","branch":"yuyu/agent-1","files":[{"path":"src/App.tsx","kind":"modified"}],"diff":"diff --git a/src/App.tsx b/src/App.tsx"}`},
	}

	review := extractCodeReviewFromToolMessages(reply, toolMessages)
	if review == nil {
		t.Fatalf("expected review metadata")
	}
	if review["plugin"] != "code-assistant" || review["branch"] != "yuyu/agent-1" {
		t.Fatalf("unexpected review metadata: %#v", review)
	}
	files, ok := review["files"].([]any)
	if !ok || len(files) != 1 {
		t.Fatalf("expected files metadata, got %#v", review["files"])
	}
}

func TestLLMExecutorMarksCodeReviewTaskResult(t *testing.T) {
	tools := []tool.BaseTool{&fakeTool{name: "run_agent", out: `{"ok":true,"cwd":"C:\\repo","branch":"yuyu/agent-2","files":[{"path":"main.go","kind":"modified"}],"diff":"diff --git a/main.go b/main.go"}`}}
	model := &scriptedToolCallingModel{
		replies: []*schema.Message{
			{
				Role: schema.Assistant,
				ToolCalls: []schema.ToolCall{
					{ID: "call-1", Type: "function", Function: schema.FunctionCall{Name: "run_agent", Arguments: `{"task":"edit"}`}},
				},
			},
			{Role: schema.Assistant, Content: "代码已生成，等待评审。"},
		},
	}
	exec := NewLLMExecutor(func(ctx context.Context) (ToolCallingModel, error) { return model, nil }, func() []tool.BaseTool { return tools })
	result, err := exec.Execute(context.Background(), TaskSpec{
		Goal:           "edit",
		Instructions:   "edit",
		Workspace:      "C:\\repo",
		AllowedActions: []string{"run_agent"},
	}, &fakeRuntime{})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.NeedReview {
		t.Fatalf("expected NeedReview")
	}
	review, ok := result.Metadata["code_review"].(map[string]any)
	if !ok || review["branch"] != "yuyu/agent-2" {
		t.Fatalf("unexpected metadata: %#v", result.Metadata)
	}
}

func TestBuildTaskMessages(t *testing.T) {
	task := TaskSpec{
		Goal:           "给当前目录添加一个说明文件",
		Instructions:   "创建 notes.md，内容用中文说明项目架构。",
		Workspace:      "C:\\workspace",
		Constraints:    []string{"不要修改无关文件", "注释用中文"},
		AllowedActions: []string{"read_file", "write_file"},
	}
	msgs := buildTaskMessages(task, task.Instructions)
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Role != schema.System {
		t.Fatalf("expected system role, got %s", msgs[0].Role)
	}
	if !strings.Contains(msgs[0].Content, "write_file") || !strings.Contains(msgs[0].Content, "C:\\workspace") {
		t.Fatalf("system prompt missing goal/workspace/actions: %s", msgs[0].Content)
	}
	if msgs[1].Role != schema.User || !strings.Contains(msgs[1].Content, "notes.md") {
		t.Fatalf("user message missing instructions: %s", msgs[1].Content)
	}
}
