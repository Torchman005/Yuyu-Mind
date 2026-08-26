package loghub

import (
	"log/slog"
	"testing"
)

func TestHubSnapshotFilterAndOrder(t *testing.T) {
	h := NewHub(nil, slog.LevelDebug, 50) // 无 sink，只缓存
	logger := slog.New(h)
	logger.Debug("d1")
	logger.Info("i1")
	logger.Warn("w1")
	logger.Error("e1")

	// 全部：DEBUG 4 条，新→旧 = e1,w1,i1,d1
	all := h.Snapshot(slog.LevelDebug, 10)
	if len(all) != 4 {
		t.Fatalf("expected 4 entries, got %d", len(all))
	}
	want := []string{"e1", "w1", "i1", "d1"}
	for i, m := range want {
		if all[i].Message != m {
			t.Fatalf("order[%d]=%s, want %s", i, all[i].Message, m)
		}
	}

	// 级别过滤：>= INFO → i1,w1,e1
	info := h.Snapshot(slog.LevelInfo, 10)
	if len(info) != 3 {
		t.Fatalf("expected 3 info+ entries, got %d: %+v", len(info), info)
	}

	// count 限制
	one := h.Snapshot(slog.LevelDebug, 1)
	if len(one) != 1 || one[0].Message != "e1" {
		t.Fatalf("expected [e1], got %+v", one)
	}
}

func TestHubRingBufferWrap(t *testing.T) {
	h := NewHub(nil, slog.LevelDebug, 3)
	logger := slog.New(h)
	for i := 1; i <= 6; i++ {
		logger.Info("m" + string(rune('0'+i)))
	}
	got := h.Snapshot(slog.LevelDebug, 10)
	// 环形容量 3，保留最后 3 条：m4,m5,m6（新→旧：m6,m5,m4）
	if len(got) != 3 {
		t.Fatalf("expected 3 after wrap, got %d", len(got))
	}
	want := []string{"m6", "m5", "m4"}
	for i, m := range want {
		if got[i].Message != m {
			t.Fatalf("wrap[%d]=%s, want %s", i, got[i].Message, m)
		}
	}
}
