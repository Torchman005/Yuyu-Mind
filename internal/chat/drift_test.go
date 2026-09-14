package chat

import (
	"context"
	"strings"
	"testing"
)

// TestStyleDriftUnchangedBelowThreshold 未达阈值时必须**完全不影响**行为
// （applyStyleDrift 对空提示原样返回，保证老行为不变）。
func TestStyleDriftUnchangedBelowThreshold(t *testing.T) {
	state := RelationshipState{Interactions: 5, FamiliarityScore: 5}
	if hint := styleDriftHint(state); hint != "" {
		t.Fatalf("交互很少时不应有漂移提示，实际 %q", hint)
	}
	const notes = "说话简短，口语化"
	if got := applyStyleDrift(notes, ""); got != notes {
		t.Fatalf("空提示应原样返回，实际 %q", got)
	}
	// 空 style_notes + 空提示也不应产生内容。
	if got := applyStyleDrift("", ""); got != "" {
		t.Fatalf("都为空时应返回空串，实际 %q", got)
	}
}

// TestStyleDriftProgressesWithInteractions 漂移应随交互次数单调推进（档位只升不降）。
func TestStyleDriftProgressesWithInteractions(t *testing.T) {
	prev := ""
	for _, n := range []int{0, 10, 40, 80, 120, 500} {
		got := styleDriftHint(RelationshipState{Interactions: n})
		if len(got) < len(prev) {
			t.Fatalf("交互 %d 次时提示不应变短（档位不该回退）：prev=%q got=%q", n, prev, got)
		}
		prev = got
	}
	// 最高档要有内容。
	if styleDriftHint(RelationshipState{Interactions: 500}) == "" {
		t.Fatalf("大量交互后应产生漂移提示")
	}
}

// TestStyleDriftDirectionMatchesPersona 是这一项最重要的约束：
// 人设是"古灵精怪、调皮、**话少**"，所以漂移方向只能是"更放松/更省客套/更省字"，
// 绝不能出现"更话痨/更热情/更啰嗦"这种把人设推向反方向的措辞。
func TestStyleDriftDirectionMatchesPersona(t *testing.T) {
	// 逐档检查（含最高档），确保没有任何一档违反人设方向。
	for _, n := range []int{40, 120, 1000} {
		hint := styleDriftHint(RelationshipState{Interactions: n})
		if hint == "" {
			t.Fatalf("交互 %d 次应有漂移提示", n)
		}
		for _, forbidden := range []string{
			"话痨", "啰嗦", "多说", "长篇", "更热情", "更主动地表达", "滔滔不绝", "更活泼",
		} {
			if strings.Contains(hint, forbidden) {
				t.Fatalf("漂移方向违反人设（出现 %q）：%q", forbidden, hint)
			}
		}
		// 应当明确朝"更放松/更省字"方向。
		if !strings.Contains(hint, "熟") {
			t.Fatalf("漂移提示应说明关系变熟，实际 %q", hint)
		}
	}
}

// TestApplyStyleDriftAppends 漂移应**追加**在原有风格说明之后，而不是替换它
// （原有的人设风格要求必须保留）。
func TestApplyStyleDriftAppends(t *testing.T) {
	const notes = "说话简短，口语化，不要动作描写"
	hint := styleDriftHint(RelationshipState{Interactions: 200})
	got := applyStyleDrift(notes, hint)

	if !strings.HasPrefix(got, notes) {
		t.Fatalf("原有风格说明应被保留在开头，实际 %q", got)
	}
	if !strings.Contains(got, hint) {
		t.Fatalf("应包含漂移提示，实际 %q", got)
	}
	if !strings.Contains(got, "\n") {
		t.Fatalf("漂移应与原说明分行，实际 %q", got)
	}
}

// TestApplyStyleDriftWithoutNotes 没有人设风格时，漂移自身即全部内容。
func TestApplyStyleDriftWithoutNotes(t *testing.T) {
	hint := styleDriftHint(RelationshipState{Interactions: 200})
	if got := applyStyleDrift("", hint); got != hint {
		t.Fatalf("无原说明时应返回漂移本身，实际 %q", got)
	}
}

// TestStyleDriftLineReflectsRelationship 验证 Service 层读取关系状态给出漂移。
func TestStyleDriftLineReflectsRelationship(t *testing.T) {
	s, _ := newInterruptionService(t)
	s.relations = newRelationshipStore(nil)
	ctx := context.Background()

	if line := s.StyleDriftLine(ctx); line != "" {
		t.Fatalf("尚无交互时不应有漂移，实际 %q", line)
	}

	for i := 0; i < 130; i++ {
		s.ObserveUserTurn(ctx, "今天想聊聊最近在看的书，感觉挺有意思的")
	}
	line := s.StyleDriftLine(ctx)
	if line == "" {
		t.Fatalf("长期相处后应产生风格漂移")
	}
	if !strings.Contains(line, "熟") {
		t.Fatalf("漂移提示应体现关系变化，实际 %q", line)
	}
}
