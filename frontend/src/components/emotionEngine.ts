// 情绪系统 v2：连续 VAD（valence/arousal/dominance）→ FACS/AU 权重 → Live2D 参数。
//
// 参考 soullink-emotion-sdk 的四大支柱：
//   1. Continuous VAD emotion —— 连续效价/唤醒/支配，而非离散关键词。
//   2. FACS/AU synthesis —— 动作单元 AU1/AU4/AU6/AU12… 合成表情。
//   3. Layered animation —— idle + 情绪 + 手势分层混合（由 Live2DStage 的 apply* 层实现）。
//   4. Automatic model adaptation —— PARAM_REGISTRY 参数注册表，运行时按模型自动匹配参数 ID。
//
// 注意：Live2D 模型的「大表情」（惊讶睁眼/悲伤撇嘴等）由 expression 层（applyPixiEmotion）
// 承担；眨眼由 ParamEyeOpen* 承担；嘴张合由唇同步 ParamMouthOpenY/ParamJawOpen 承担。
// 本模块只负责「连续的微妙表情参数」（微笑/眉/嘴型），避免与上述层发生参数竞争。
// 若日后直接接入 @soullink-emotion/live2d-pixi，可把 computeExpressionTargets 替换为
// SDK 的 emotion→AU 输出，其余渲染层不变。

export type EmotionVector = {
    valence: number; // -1..1 消极↔积极
    arousal: number; // 0..1 平静↔激动（对应后端 energy）
    dominance: number; // -1..1 顺从↔自信
};

// FACS 动作单元权重（0..1）。
export type AUWeights = {
    au1: number; // 内眉上扬（关切/惊讶/悲伤）
    au2: number; // 外眉上扬
    au4: number; // 皱眉（愤怒/专注/悲伤）
    au5: number; // 上眼睑提升（睁大眼，主要由 expression 层承担）
    au6: number; // 脸颊提升 / 眯眼笑（真笑）
    au7: number; // 眼睑收紧（专注）
    au12: number; // 嘴角上拉（微笑）
    au15: number; // 嘴角下压（悲伤）
    au20: number; // 嘴角拉伸（紧张）
    au25: number; // 双唇分开（由唇同步承担）
    au26: number; // 下颌下垂（由唇同步承担）
};

function clamp(value: number, min: number, max: number) {
    return Math.max(min, Math.min(max, value));
}

function neutralAUs(): AUWeights {
    return {au1: 0, au2: 0, au4: 0, au5: 0, au6: 0, au7: 0, au12: 0, au15: 0, au20: 0, au25: 0, au26: 0};
}

// 离散 mood → AU 基线（保留旧 moodBoost 的观感，避免升级后突变）。
const moodAUBase: Record<string, Partial<AUWeights>> = {
    calm: {au6: 0.08, au12: 0.02},
    cheer: {au6: 0.48, au1: 0.14, au2: 0.14, au12: 0.18},
    curious: {au6: 0.16, au1: 0.28, au2: 0.28, au12: 0.05},
    confident: {au6: 0.28, au1: 0.05, au12: 0.1},
    comfort: {au6: 0.24, au4: 0.08, au12: 0.04},
    surprised: {au6: 0.02, au1: 0.42, au2: 0.42, au12: 0.16},
    playful: {au6: 0.4, au1: 0.18, au2: 0.18, au12: 0.2},
};

// computeAUWeights 把离散 emotion/mood 与连续 VAD 融合成 AU 权重。
export function computeAUWeights(vector: EmotionVector, emotion: string, mood: string): AUWeights {
    const au: AUWeights = {...neutralAUs(), ...(moodAUBase[mood] ?? {})};
    const v = clamp(vector.valence, -1, 1);
    const a = clamp(vector.arousal, 0, 1);
    const d = clamp(vector.dominance, -1, 1);

    // 效价：正 → 微笑（AU6+AU12），负 → 皱眉/嘴角下压（AU4+AU15）。
    if (v > 0) {
        au.au6 += v * 0.3;
        au.au12 += v * 0.35;
    } else {
        au.au4 += -v * 0.4;
        au.au15 += -v * 0.4;
        au.au1 += -v * 0.15;
    }

    // 唤醒：高唤醒 → 抬眉/睁眼（AU1+AU2+AU5）。
    au.au5 += a * 0.35;
    au.au1 += a * 0.15;
    au.au2 += a * 0.15;

    // 支配度：自信 → 压眉（AU4+AU7），顺从 → 抬眉（AU1+AU2）。
    if (d > 0) {
        au.au4 += d * 0.2;
        au.au7 += d * 0.15;
    } else {
        au.au1 += -d * 0.3;
        au.au2 += -d * 0.3;
    }

    // 离散 emotion 关键帧（更鲜明的表情）。
    switch (emotion) {
        case 'happy':
            au.au6 = Math.max(au.au6, 0.7);
            au.au12 = Math.max(au.au12, 0.75);
            break;
        case 'sad':
            au.au1 = Math.max(au.au1, 0.55);
            au.au4 = Math.max(au.au4, 0.3);
            au.au15 = Math.max(au.au15, 0.55);
            au.au6 = 0;
            au.au12 = 0;
            break;
        case 'surprised':
            au.au1 = Math.max(au.au1, 0.8);
            au.au2 = Math.max(au.au2, 0.8);
            au.au5 = Math.max(au.au5, 0.75);
            break;
        case 'focused':
            au.au4 = Math.max(au.au4, 0.5);
            au.au7 = Math.max(au.au7, 0.5);
            au.au12 += 0.12;
            break;
        case 'thinking':
            au.au1 += 0.25;
            au.au4 += 0.15;
            au.au20 += 0.15;
            break;
        default:
            break;
    }

    (Object.keys(au) as (keyof AUWeights)[]).forEach((key) => {
        au[key] = clamp(au[key], 0, 1);
    });
    return au;
}

// 连续表情目标：只包含 Live2DStage 连续层实际拥有的参数（见文件头说明）。
export type LogicalParamName = 'eyeSmile' | 'browRaise' | 'browForm' | 'mouthCorner';

export type ExpressionTargets = Record<LogicalParamName, number>;

// computeExpressionTargets：AU 权重 → 逻辑参数净目标。
// - eyeSmile    = AU6（眯眼笑）
// - browRaise   = (AU1+AU2) − AU4（净抬眉，负值即压眉）
// - browForm    = browRaise 的缩放版（沿用旧实现的双参数写法）
// - mouthCorner = AU12 − AU15（微笑减悲伤，-1..1）
export function computeExpressionTargets(vector: EmotionVector, emotion: string, mood: string): ExpressionTargets {
    const au = computeAUWeights(vector, emotion, mood);
    const browRaise = clamp((au.au1 + au.au2) - au.au4, -1, 1);
    return {
        eyeSmile: clamp(au.au6, 0, 1),
        browRaise,
        browForm: browRaise * 0.45,
        mouthCorner: clamp(au.au12 - au.au15, -1, 1),
    };
}

// —— 参数注册表（Automatic model adaptation）——
// 逻辑参数 → 候选 Live2D 参数 ID（当前模型在前，常见替代名在后，语义一致、不冲突）。
type ParamCandidates = {left?: string[]; right?: string[]; single?: string[]};

export const PARAM_REGISTRY: Record<LogicalParamName, ParamCandidates> = {
    eyeSmile: {left: ['ParamEyeSmileL', 'ParamEyeSmile'], right: ['ParamEyeSmileR']},
    browRaise: {left: ['ParamBrowYL', 'ParamBrowY'], right: ['ParamBrowYR']},
    browForm: {left: ['ParamBrowFormL', 'ParamBrowForm'], right: ['ParamBrowFormR']},
    mouthCorner: {single: ['ParamMouthForm', 'ParamMouthSmile']},
};

function coreModelOf(model: any) {
    return model?.internalModel?.coreModel;
}

// resolveParamId 返回候选里第一个模型支持的参数 ID；都不支持则回退候选首项（写入为 no-op）。
export function resolveParamId(model: any, candidates: string[]): string | null {
    const coreModel = coreModelOf(model);
    if (!coreModel) {
        return candidates[0] ?? null;
    }
    for (const id of candidates) {
        const index = coreModel.getParameterIndex?.(id);
        if (typeof index === 'number' && index >= 0) {
            return id;
        }
    }
    return candidates[0] ?? null;
}

function writeParameter(model: any, id: string, value: number, weight: number) {
    const coreModel = coreModelOf(model);
    coreModel?.setParameterValueById?.(id, value, weight);
    coreModel?.setParamFloat?.(id, value, weight);
    const index = coreModel?.getParameterIndex?.(id);
    if (typeof index === 'number' && index >= 0) {
        coreModel?.setParameterValueByIndex?.(index, value, weight);
    }
}

// applyExpressionTargets 按注册表把逻辑参数目标写到模型上。
export function applyExpressionTargets(model: any, targets: ExpressionTargets, weight = 1) {
    const apply = (name: LogicalParamName, value: number) => {
        const entry = PARAM_REGISTRY[name];
        if (!entry) {
            return;
        }
        if (entry.single) {
            const id = resolveParamId(model, entry.single);
            if (id) {
                writeParameter(model, id, value, weight);
            }
        }
        if (entry.left) {
            const id = resolveParamId(model, entry.left);
            if (id) {
                writeParameter(model, id, value, weight);
            }
        }
        if (entry.right) {
            const id = resolveParamId(model, entry.right);
            if (id) {
                writeParameter(model, id, value, weight);
            }
        }
    };

    (Object.keys(targets) as LogicalParamName[]).forEach((name) => apply(name, targets[name]));
}
