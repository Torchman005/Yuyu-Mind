// Package loghub 提供一个既是 slog.Handler 又保留内存环形缓冲的日志中枢：
// 所有日志既写往 sink（默认 stderr），也会被缓存起来供前端「桌宠日志」页读取。
package loghub

import (
	"context"
	"log/slog"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// Entry 是一个可给前端展示的日志条目。
type Entry struct {
	Time    time.Time      `json:"time"`
	Level   string         `json:"level"`   // DEBUG|INFO|WARN|ERROR
	Message string         `json:"message"` // 已格式化/拼接的消息
	Attrs   map[string]any `json:"attrs,omitempty"`
	Source  string         `json:"source,omitempty"` // 调用位置 file:line（尽力而为）

	lvl slog.Level `json:"-"`
}

// buffer 是共享的环形缓冲（由 Hub 及其 WithAttrs/WithGroup 派生 handler 共用）。
type buffer struct {
	mu   sync.Mutex
	vals []Entry
	head int
	full bool
	max  int
}

// add 写入一条；超限时覆盖最旧一条。
func (b *buffer) add(e Entry) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.vals) < b.max {
		b.vals = append(b.vals, e)
		return
	}
	b.vals[b.head] = e
	b.head = (b.head + 1) % b.max
	b.full = true
}

// snapshot 返回按级别 >= level 过滤后的、最新 count 条（新→旧序）。
func (b *buffer) snapshot(level slog.Level, count int) []Entry {
	b.mu.Lock()
	defer b.mu.Unlock()
	if count <= 0 {
		count = b.max
	}
	var ordered []Entry
	if b.full {
		ordered = make([]Entry, 0, len(b.vals))
		ordered = append(ordered, b.vals[b.head:]...)
		ordered = append(ordered, b.vals[:b.head]...)
	} else {
		ordered = append(ordered, b.vals...)
	}
	// 过滤：收集 >= level 的条目（保持时间序），再取末尾 count 条，反转为新→旧。
	kept := make([]Entry, 0, len(ordered))
	for _, e := range ordered {
		if e.lvl >= level {
			kept = append(kept, e)
		}
	}
	if len(kept) > count {
		kept = kept[len(kept)-count:]
	}
	// 新→旧
	for i, j := 0, len(kept)-1; i < j; i, j = i+1, j-1 {
		kept[i], kept[j] = kept[j], kept[i]
	}
	return kept
}

// Hub 实现 slog.Handler：把记录转发给 sink，并缓存到共享 buffer。
type Hub struct {
	buf   *buffer
	sink  slog.Handler
	level atomic.Int64
}

var _ slog.Handler = (*Hub)(nil)

// NewHub 创建日志中枢。sink 为真正的输出 handler（可为 nil 只缓存）；level 为采集门限。
func NewHub(sink slog.Handler, level slog.Level, max int) *Hub {
	if max <= 0 {
		max = 2000
	}
	h := &Hub{
		buf:  &buffer{max: max},
		sink: sink,
	}
	h.level.Store(int64(level))
	return h
}

// SetLevel 动态调整采集门限。
func (h *Hub) SetLevel(level slog.Level) { h.level.Store(int64(level)) }

// Level 返回当前门限。
func (h *Hub) Level() slog.Level { return slog.Level(h.level.Load()) }

// Enabled 决定某级日志是否采集。
func (h *Hub) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= h.Level()
}

// Handle 缓存记录并转发给 sink。
func (h *Hub) Handle(ctx context.Context, r slog.Record) error {
	h.buf.add(entryFromRecord(r))
	if h.sink != nil {
		return h.sink.Handle(ctx, r)
	}
	return nil
}

// WithAttrs 返回共享同一缓冲、带额外属性的派生 handler。
func (h *Hub) WithAttrs(attrs []slog.Attr) slog.Handler {
	var sink slog.Handler
	if h.sink != nil {
		sink = h.sink.WithAttrs(attrs)
	}
	return h.cloneWith(sink)
}

// WithGroup 返回共享同一缓冲、带分组的派生 handler。
func (h *Hub) WithGroup(name string) slog.Handler {
	var sink slog.Handler
	if h.sink != nil {
		sink = h.sink.WithGroup(name)
	}
	return h.cloneWith(sink)
}

func (h *Hub) cloneWith(sink slog.Handler) *Hub {
	c := &Hub{buf: h.buf, sink: sink}
	c.level.Store(h.level.Load())
	return c
}

// Snapshot 返回最新日志（新→旧），`level` 为显示门槛（如 slog.LevelInfo），count 为条数上限。
func (h *Hub) Snapshot(level slog.Level, count int) []Entry {
	return h.buf.snapshot(level, count)
}

// entryFromRecord 把 slog.Record 转成 Entry。
func entryFromRecord(r slog.Record) Entry {
	attrs := map[string]any{}
	r.Attrs(func(a slog.Attr) bool {
		if a.Key != "" {
			attrs[a.Key] = slogValueToAny(a.Value)
		}
		return true
	})
	var src string
	if r.PC != 0 {
		if f := runtime.FuncForPC(r.PC); f != nil {
			file, line := f.FileLine(r.PC)
			src = trimPath(file) + ":" + itoa(line)
		}
	}
	return Entry{
		Time:    r.Time,
		Level:   r.Level.String(),
		Message: r.Message,
		Attrs:   attrs,
		Source:  src,
		lvl:     r.Level,
	}
}

func slogValueToAny(v slog.Value) any {
	switch v.Kind() {
	case slog.KindAny:
		return v.Any()
	case slog.KindGroup:
		g := map[string]any{}
		for _, a := range v.Group() {
			g[a.Key] = slogValueToAny(a.Value)
		}
		return g
	case slog.KindString:
		return v.String()
	case slog.KindBool:
		return v.Bool()
	case slog.KindInt64:
		return v.Int64()
	case slog.KindUint64:
		return v.Uint64()
	case slog.KindFloat64:
		return v.Float64()
	case slog.KindDuration:
		return v.Duration().String()
	case slog.KindLogValuer:
		return v.Resolve().Any()
	default:
		return v.String()
	}
}

func trimPath(p string) string {
	// 取包路径最后一个目录之前的名称，避免超长绝对路径刷屏。
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' || p[i] == '\\' {
			if i > 12 {
				p = p[i+1:]
			}
			break
		}
	}
	return p
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [24]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
