// 学习模式情绪适配：将学习状态映射到情绪参数
import { EmotionVector } from './emotionEngine';

export type StudyModeType = 'idle' | 'study' | 'break';

// 学习模式 → 情绪向量映射
export function getStudyModeEmotion(mode: StudyModeType): EmotionVector {
  switch (mode) {
    case 'study':
      // 学习中：专注、平静、自信
      return {
        valence: 0.3,     // 轻微积极（专注的满足感）
        arousal: 0.4,     // 中等激活（保持注意力）
        dominance: 0.5,   // 自信掌控
      };

    case 'break':
      // 休息中：放松、愉悦
      return {
        valence: 0.6,     // 积极愉悦
        arousal: 0.2,     // 低激活（放松）
        dominance: 0.3,   // 轻松随意
      };

    case 'idle':
    default:
      // 空闲：中性、等待
      return {
        valence: 0.2,     // 轻微积极
        arousal: 0.3,     // 低激活
        dominance: 0.4,   // 平和
      };
  }
}

// 学习模式 → 离散情绪标签
export function getStudyModeEmotionLabel(mode: StudyModeType): string {
  switch (mode) {
    case 'study':
      return 'focused';   // 专注
    case 'break':
      return 'calm';      // 平静
    case 'idle':
    default:
      return 'curious';   // 好奇/等待
  }
}

// 学习模式 → mood 标签
export function getStudyModeMood(mode: StudyModeType): string {
  switch (mode) {
    case 'study':
      return 'confident'; // 自信专注
    case 'break':
      return 'comfort';   // 舒适放松
    case 'idle':
    default:
      return 'calm';      // 平静
  }
}

// 学习事件 → 情绪反馈
export function getStudyEventEmotion(eventType: string): EmotionVector | null {
  switch (eventType) {
    case 'session_start':
      // 开始学习：积极、激活
      return {
        valence: 0.5,
        arousal: 0.6,
        dominance: 0.6,
      };

    case 'session_complete':
      // 完成一节：愉悦、成就感
      return {
        valence: 0.8,
        arousal: 0.7,
        dominance: 0.7,
      };

    case 'session_paused':
      // 暂停：轻微消极、低激活
      return {
        valence: 0.1,
        arousal: 0.3,
        dominance: 0.4,
      };

    case 'reminder':
      // 提醒：中性、温和
      return {
        valence: 0.4,
        arousal: 0.5,
        dominance: 0.5,
      };

    default:
      return null;
  }
}
