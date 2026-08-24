package chat

import "strings"

// shouldSkipPlanner 判断是否可跳过 Planner 直接流式回复（「快速通道」）。
//
// 目的：把「首字延迟」从 Planner(全文 JSON 一轮) + Replyer(首字) 压缩到只剩 Replyer。
// 原则：保守——只有明显是「简单闲聊」时才跳过；任何可能涉及记忆/工具/任务/复杂意图的信号都走 Planner。
//
// 命中信号 → 返回 false（走 Planner，安全）；否则且长度较短 → 返回 true（快速通道）。
func shouldSkipPlanner(content string) bool {
	text := strings.TrimSpace(content)
	if text == "" {
		return false
	}

	lower := strings.ToLower(text)
	signals := []string{
		// 任务/写代码/文件
		"帮我", "任务", "生成", "创建", "制作", "写一个", "做一个", "代码", "文件", "文档", "ppt", "幻灯片",
		"下载", "安装", "编译", "修复", "执行", "运行", "脚本", "程序",
		// 工具/信息查询
		"查一下", "查询", "搜索", "搜一", "天气", "翻译", "计算", "截图", "看屏幕", "打开", "关闭", "播放", "搜索",
		// 记忆/上下文
		"记得", "回忆", "之前", "上次", "我说过", "你知道", "计划", "安排", "提醒",
	}
	for _, signal := range signals {
		if strings.Contains(lower, signal) {
			return false
		}
	}

	// 过长消息更可能含复杂意图，保守起见走 Planner。
	// 阈值设为 40 个字符（原 24）：语音闲聊常见的 1-2 句话也能走快速通道，进一步压缩首字延迟；
	// 复杂意图（任务/工具/记忆）已由上方信号词拦截，仍有较大概率命中 Planner。
	return len([]rune(text)) <= 40
}
