package chat

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yuyu-mind/backend/internal/db"
	"github.com/yuyu-mind/backend/internal/memory"
)

// TestParseExtractedMemories 覆盖记忆抽取的解析：正常 JSON、markdown 包裹、
// 非法 kind 过滤、缺字段过滤、条数上限、以及"无值得记的内容"。
func TestParseExtractedMemories(t *testing.T) {
	t.Run("标准 JSON", func(t *testing.T) {
		raw := `{"memories":[{"kind":"preference","key":"drink","text":"用户喜欢喝美式咖啡"}]}`
		got := parseExtractedMemories(raw)
		if len(got) != 1 {
			t.Fatalf("应解析出 1 条，实际 %d", len(got))
		}
		if got[0].Key != "drink" || got[0].Kind != memory.MemoryKindPreference {
			t.Fatalf("字段解析错误：%+v", got[0])
		}
	})

	t.Run("markdown 包裹", func(t *testing.T) {
		raw := "```json\n{\"memories\":[{\"kind\":\"fact\",\"key\":\"job\",\"text\":\"用户是后端工程师\"}]}\n```"
		if got := parseExtractedMemories(raw); len(got) != 1 {
			t.Fatalf("应容忍 markdown 代码块，实际 %d 条", len(got))
		}
	})

	t.Run("前后有多余文字", func(t *testing.T) {
		raw := `好的，结果如下：{"memories":[{"kind":"instruction","key":"language","text":"用户要求以后都用中文"}]} 完毕`
		if got := parseExtractedMemories(raw); len(got) != 1 {
			t.Fatalf("应提取出 JSON 对象，实际 %d 条", len(got))
		}
	})

	t.Run("无需记忆", func(t *testing.T) {
		if got := parseExtractedMemories(`{"memories":[]}`); len(got) != 0 {
			t.Fatalf("空列表应返回 0 条，实际 %d", len(got))
		}
	})

	t.Run("非法 kind 被过滤", func(t *testing.T) {
		raw := `{"memories":[{"kind":"episode","key":"x","text":"某件事"},{"kind":"preference","key":"y","text":"用户喜欢猫"}]}`
		got := parseExtractedMemories(raw)
		if len(got) != 1 || got[0].Key != "y" {
			t.Fatalf("应只保留合法 kind，实际 %+v", got)
		}
	})

	t.Run("缺字段被过滤", func(t *testing.T) {
		raw := `{"memories":[{"kind":"preference","text":"没有 key"},{"kind":"preference","key":"ok","text":"有 key"}]}`
		got := parseExtractedMemories(raw)
		if len(got) != 1 || got[0].Key != "ok" {
			t.Fatalf("缺 key 的条目应被丢弃，实际 %+v", got)
		}
	})

	t.Run("条数上限", func(t *testing.T) {
		raw := `{"memories":[
			{"kind":"fact","key":"a","text":"甲"},
			{"kind":"fact","key":"b","text":"乙"},
			{"kind":"fact","key":"c","text":"丙"},
			{"kind":"fact","key":"d","text":"丁"}]}`
		if got := parseExtractedMemories(raw); len(got) != memoryExtractMaxItems {
			t.Fatalf("应受上限 %d 限制，实际 %d", memoryExtractMaxItems, len(got))
		}
	})

	t.Run("非法输入不 panic", func(t *testing.T) {
		for _, raw := range []string{"", "not json", "{", "[]", "null"} {
			if got := parseExtractedMemories(raw); len(got) != 0 {
				t.Fatalf("(%q) 应返回空，实际 %+v", raw, got)
			}
		}
	})
}

// TestExtractAndStoreMemoriesSkipsShortInput 过短的用户消息不值得抽取（避免"嗯"也去调模型）。
func TestExtractAndStoreMemoriesSkipsShortInput(t *testing.T) {
	s, convID := newInterruptionService(t)
	s.longMemory = memory.NewServiceMemory(nil) // 只验证"不会走到写入"
	if n := s.ExtractAndStoreMemories(context.Background(), convID, "嗯", "好的"); n != 0 {
		t.Fatalf("过短输入应直接返回 0，实际 %d", n)
	}
}

// TestRelationshipObserveAccumulates 验证关系随交互累积，且单次增量有上限。
func TestRelationshipObserveAccumulates(t *testing.T) {
	store := newRelationshipStore(nil)
	ctx := context.Background()

	first := store.Observe(ctx, "你好")
	if first.Interactions != 1 {
		t.Fatalf("首次交互计数应为 1，实际 %d", first.Interactions)
	}
	if first.FirstSeenAt.IsZero() {
		t.Fatalf("应记录首次互动时间")
	}

	// 带礼貌 + 情绪 + 长内容的普通消息：增量更大，但不得超过单次上限 3。
	store.Observe(ctx, "谢谢你帮我这么多，我今天真的有点累，不过还好有你")
	third := store.Observe(ctx, "麻烦你了")

	if third.Interactions != 3 {
		t.Fatalf("交互次数应为 3，实际 %d", third.Interactions)
	}
	if third.FamiliarityScore <= first.FamiliarityScore {
		t.Fatalf("亲密度应随交互增长：first=%v third=%v", first.FamiliarityScore, third.FamiliarityScore)
	}
	if third.FamiliarityScore > 9 {
		t.Fatalf("3 次交互的亲密度不应超过 9（单次上限 3），实际 %v", third.FamiliarityScore)
	}
}

// TestRelationshipPersistsAcrossStoreInstances 关系必须持久化（跨重启不应"初次见面"）。
func TestRelationshipPersistsAcrossStoreInstances(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "relationship.db"))
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	defer database.Close()
	ctx := context.Background()

	// 第一个实例：攒够 relationshipSaveEvery 次交互触发落库。
	first := newRelationshipStore(database.Settings)
	for i := 0; i < relationshipSaveEvery; i++ {
		first.Observe(ctx, "今天想和你聊聊最近看的那本书")
	}

	// 第二个实例（模拟重启）：应读到之前的关系，而不是从零开始。
	second := newRelationshipStore(database.Settings)
	got := second.Snapshot(ctx)
	if got.Interactions != relationshipSaveEvery {
		t.Fatalf("重启后应恢复 %d 次交互，实际 %d", relationshipSaveEvery, got.Interactions)
	}
	if got.FamiliarityScore <= 0 {
		t.Fatalf("重启后亲密度应保留，实际 %v", got.FamiliarityScore)
	}
}

// TestRelationshipPromptLineIsStaged 关系描述应随阶段变化，且不暴露数值。
func TestRelationshipPromptLineIsStaged(t *testing.T) {
	cases := []struct {
		name  string
		state RelationshipState
		want  string
	}{
		{"无互动", RelationshipState{}, ""},
		{"刚认识", RelationshipState{Interactions: 1, FamiliarityScore: 1}, "不太熟"},
		{"聊过一阵", RelationshipState{Interactions: 6, FamiliarityScore: 6}, "已经聊过一阵子"},
		{"比较熟", RelationshipState{Interactions: 25, FamiliarityScore: 30}, "比较熟"},
		{"很熟", RelationshipState{Interactions: 80, FamiliarityScore: 70}, "已经很熟"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := relationshipPromptLine(tc.state)
			if tc.want == "" {
				if got != "" {
					t.Fatalf("无互动时不应产生描述，实际 %q", got)
				}
				return
			}
			if !strings.Contains(got, tc.want) {
				t.Fatalf("描述应包含 %q，实际 %q", tc.want, got)
			}
			// 与情绪底色同样的原则：不暴露数值。
			for _, bad := range []string{"0.", "1.", "分", "%"} {
				if strings.Contains(got, bad) {
					t.Fatalf("不应泄露数值 %q，实际 %q", bad, got)
				}
			}
		})
	}
}

// TestObserveUserTurnReturnsLine 验证 Service 层的接入：每轮返回关系描述并累积交互。
func TestObserveUserTurnReturnsLine(t *testing.T) {
	s, _ := newInterruptionService(t)
	s.relations = newRelationshipStore(nil)
	ctx := context.Background()

	if line := s.ObserveUserTurn(ctx, "你好呀"); line == "" {
		t.Fatalf("首次交互也应给出关系描述")
	}
	for i := 0; i < 30; i++ {
		s.ObserveUserTurn(ctx, "今天想和你聊聊最近看的那本书，感觉挺有意思的")
	}
	line := s.ObserveUserTurn(ctx, "还是想继续说说那本书")
	if !strings.Contains(line, "很熟") && !strings.Contains(line, "比较熟") {
		t.Fatalf("多次交互后关系描述应升级，实际 %q", line)
	}
}

var _ = time.Now
