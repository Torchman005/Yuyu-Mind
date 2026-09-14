package chat

import (
	"context"
	"fmt"
	"strings"
)

// 本文件实现「性格随关系漂移」（见 docs/REALISM-ANALYSIS.md P3 可选实验）。
//
// 动机：上一轮做完关系状态后，它只影响一句"你们已经很熟了"的模糊描述——能改变语气松紧，
// 但**角色本身没有变化**。真人相处久了，说话方式会变：更放松、更省客套、更少解释、
// 玩笑更随便。让关系驱动这些行为，长期陪伴才有"时间感"。
//
// 设计约束（重要）：
//   - 漂移方向必须与人设**同向**，不能矛盾。人设是"古灵精怪、调皮、话少"，
//     所以漂移只做"更放松/更简短随意/玩笑更随口"这类**单调**变化；
//     绝不出现"越熟越话痨"或"越熟越热情"——那会把人设推向反方向。
//   - 只影响**风格**，不影响底线：称呼（主人）、不说动作描写、不暴露 AI 身份等属于角色设定，
//     任何阶段都不变。

// styleDrift 是一档风格偏移。
type styleDrift struct {
	// MinInteractions 是进入该档所需的最少交互次数。
	MinInteractions int
	// Hint 是注入 Replyer 的中文风格提示（贴近生成点，比 Planner 更有效）。
	Hint string
}

// styleDriftLevels 按交互次数给出风格偏移，由少到多。
//
// 阈值刻意取"比较多"（40+ / 120+）：要让用户真的感到"相处久了"，而不是聊十句就变个人。
var styleDriftLevels = []styleDrift{
	{
		MinInteractions: 0,
		Hint:            "",
	},
	{
		MinInteractions: 40,
		Hint:            "你和主人已经挺熟了，可以少一点客套、更随口一些；偶尔可以偷懒只答一句。",
	},
	{
		MinInteractions: 120,
		Hint:            "你和主人很熟了：说话更放松、更省字，玩笑可以更随口，不必每次都解释清楚；有时一句吐槽就够了。",
	},
}

// styleDriftHint 返回当前关系下应注入的**风格**提示（可为空表示不偏移）。
func styleDriftHint(state RelationshipState) string {
	hint := ""
	for _, level := range styleDriftLevels {
		if state.Interactions >= level.MinInteractions {
			hint = level.Hint
		}
	}
	return hint
}

// StyleDriftLine 返回当前会话的风格偏移提示，供注入 Replyer。
func (s *Service) StyleDriftLine(ctx context.Context) string {
	if s == nil || s.relations == nil {
		return ""
	}
	return styleDriftHint(s.relations.Snapshot(ctx))
}

// applyStyleDrift 把风格偏移追加到 StyleNotes 之后（保持原有内容不变，只在末尾附加偏移）。
// 空提示时原样返回，确保"未达到漂移阈值"时行为与之前完全一致。
func applyStyleDrift(styleNotes, drift string) string {
	drift = strings.TrimSpace(drift)
	if drift == "" {
		return styleNotes
	}
	base := strings.TrimSpace(styleNotes)
	if base == "" {
		return drift
	}
	return base + "\n" + drift
}

// driftSummaryForLog 便于诊断：描述当前处于哪一档。
func driftSummaryForLog(state RelationshipState) string {
	idx := 0
	for i, level := range styleDriftLevels {
		if state.Interactions >= level.MinInteractions {
			idx = i
		}
	}
	return fmt.Sprintf("style_drift_level=%d interactions=%d", idx, state.Interactions)
}
