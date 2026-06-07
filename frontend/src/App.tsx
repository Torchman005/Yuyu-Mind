import {FormEvent, WheelEvent, useEffect, useMemo, useRef, useState} from 'react';
import './App.css';
import {
    ClearChat,
    GenerateProactiveMessage,
    GetState,
    ObserveScreen,
    ProbeFishLive,
    SendMessage,
    SynthesizeSpeech,
    SynthesizeSpeechStream,
    TranscribeAudio,
    UpdatePetHitTest,
} from '../wailsjs/go/main/App';
import {
    EventsOn,
    Quit,
    WindowCenter,
    WindowMinimise,
    WindowSetAlwaysOnTop,
    WindowSetBackgroundColour,
    WindowSetSize,
} from '../wailsjs/runtime/runtime';
import {AvatarPerformance, Live2DStage} from './components/Live2DStage';

type Message = {
    id: number;
    role: string;
    content: string;
    emotion: string;
    createdAt: string;
};

type ChatResponse = {
    messages?: Message[];
    reply?: Message;
    speechText?: string;
    emotion?: string;
    agentStatus?: string;
    agentProvider?: string;
    providerError?: string;
};

type SpeechStreamEvent = {
    sessionId?: string;
    audioBase64?: string;
    contentType?: string;
    error?: string;
    provider?: string;
    phase?: string;
    elapsedMs?: number;
    detail?: string;
};

type SpeechMetric = {
    phase: string;
    elapsedMs?: number;
    detail?: string;
};

type FishLiveProbeResult = {
    ok?: boolean;
    error?: string;
    events?: string[];
    elapsedMs?: number;
    audioSize?: number;
};

type ASRReply = {
    text?: string;
    provider?: string;
    language?: string;
    duration?: number;
    error?: string;
};

const emotionLabel: Record<string, string> = {
    neutral: '待机',
    happy: '开心',
    focused: '专注',
    thinking: '思考',
    sad: '低落',
    surprised: '惊讶',
};

const PET_MODE_KEY = 'yuyu.petMode';
const PET_SCALE_KEY = 'yuyu.petScale';
const CONTINUOUS_VOICE_KEY = 'yuyu.continuousVoice';
const CONVERSATION_MODE_KEY = 'yuyu.conversationMode';
const SPEECH_LANGUAGE_KEY = 'yuyu.speechLanguage';
const LEGACY_PET_MODE_KEY = 'mochi.petMode';
const LEGACY_PET_SCALE_KEY = 'mochi.petScale';
const LEGACY_CONTINUOUS_VOICE_KEY = 'mochi.continuousVoice';
const PET_CONTROLS_SHORTCUT = 'Ctrl + Shift + M';
const PET_BASE_WIDTH = 380;
const PET_BASE_HEIGHT = 560;
const PET_MIN_SCALE = 0.6;
const PET_MAX_SCALE = 1.8;
const PET_SCALE_STEP = 0.08;
const PET_HIT_INSET_X = 0.06;
const PET_HIT_INSET_TOP = 0.02;
const PET_HIT_INSET_BOTTOM = 0.02;
const DESKTOP_PET_NAME = (
    (import.meta.env.VITE_YUYU_DESKTOP_PET_NAME as string | undefined)
    || (import.meta.env.VITE_DESKTOP_PET_NAME as string | undefined)
    || ''
).trim() || 'Yuyu';
const SPEECH_OUTPUT_MODE = ((import.meta.env.VITE_SPEECH_OUTPUT_MODE as string | undefined) || 'cloud').trim().toLowerCase();
const ALLOW_SYSTEM_TTS_FALLBACK = ((import.meta.env.VITE_ALLOW_SYSTEM_TTS_FALLBACK as string | undefined) || 'false').trim().toLowerCase() === 'true';
const ENABLE_STREAMING_TTS = ((import.meta.env.VITE_ENABLE_STREAMING_TTS as string | undefined) || 'false').trim().toLowerCase() === 'true';
const ENABLE_REALTIME_SPEECH = ((import.meta.env.VITE_REALTIME_SPEECH as string | undefined) || 'false').trim().toLowerCase() === 'true';
const SHOW_SPEECH_DEBUG = ((import.meta.env.VITE_SHOW_SPEECH_DEBUG as string | undefined) || 'false').trim().toLowerCase() === 'true';
const ASR_PROVIDER = ((import.meta.env.VITE_ASR_PROVIDER as string | undefined) || 'browser').trim().toLowerCase();
const DEFAULT_SPEECH_LANGUAGE = ((import.meta.env.VITE_SPEECH_LANGUAGE as string | undefined) || 'ja').trim().toLowerCase().startsWith('zh') ? 'zh' : 'ja';
const PROACTIVE_ENABLED = ((import.meta.env.VITE_PROACTIVE_ENABLED as string | undefined) || 'true').trim().toLowerCase() === 'true';
const PROACTIVE_IDLE_MINUTES = Number((import.meta.env.VITE_PROACTIVE_IDLE_MINUTES as string | undefined) || '8');
const PROACTIVE_COOLDOWN_MINUTES = Number((import.meta.env.VITE_PROACTIVE_COOLDOWN_MINUTES as string | undefined) || '15');
const PROACTIVE_QUIET_HOURS = ((import.meta.env.VITE_PROACTIVE_QUIET_HOURS as string | undefined) || '01:00-09:00').trim();
const PROACTIVE_CHECK_SECONDS = Number((import.meta.env.VITE_PROACTIVE_CHECK_SECONDS as string | undefined) || '30');
const PROACTIVE_FREE_MODE_ENABLED = ((import.meta.env.VITE_PROACTIVE_FREE_MODE_ENABLED as string | undefined) || 'true').trim().toLowerCase() === 'true';
const PROACTIVE_PLUGIN_CONTEXT_HINT_MINUTES = Number((import.meta.env.VITE_PROACTIVE_PLUGIN_CONTEXT_HINT_MINUTES as string | undefined) || '20');
const PROACTIVE_CHANCE_PERCENT = clamp(Number((import.meta.env.VITE_PROACTIVE_CHANCE_PERCENT as string | undefined) || '35'), 0, 100);
const PROACTIVE_MAX_PER_HOUR = Number((import.meta.env.VITE_PROACTIVE_MAX_PER_HOUR as string | undefined) || '3');
const MAX_VISIBLE_CHAT_ROUNDS = Number((import.meta.env.VITE_MAX_VISIBLE_CHAT_ROUNDS as string | undefined) || '20');
const DEFAULT_CONVERSATION_MODE = ((import.meta.env.VITE_CONVERSATION_MODE as string | undefined) || 'manual').trim().toLowerCase() === 'free' ? 'free' : 'manual';
const VOICE_RELISTEN_DELAY_MS = 280;
const VOICE_LOOP_MAX_EMPTY_TURNS = 4;
const VOICE_LOOP_EMPTY_BACKOFF_MS = 900;
const VOICE_LOOP_MAX_BACKOFF_MS = 3500;
const VOICE_MIN_CHARS = 2;
const VOICE_LOOP_MIN_CHARS = 3;
const VOICE_LOW_CONFIDENCE = 0.35;
const VOICE_GATE_ENABLED = ((import.meta.env.VITE_VOICE_GATE_ENABLED as string | undefined) || 'true').trim().toLowerCase() === 'true';
const VOICE_GATE_THRESHOLD = Number((import.meta.env.VITE_VOICE_GATE_THRESHOLD as string | undefined) || '0.035');
const VOICE_GATE_HOLD_MS = Number((import.meta.env.VITE_VOICE_GATE_HOLD_MS as string | undefined) || '160');
const VOICE_GATE_TIMEOUT_MS = Number((import.meta.env.VITE_VOICE_GATE_TIMEOUT_MS as string | undefined) || '12000');
const VOICE_AUTO_SUBMIT_SILENCE_MS = Number((import.meta.env.VITE_VOICE_AUTO_SUBMIT_SILENCE_MS as string | undefined) || '900');
const VOICE_MAX_UTTERANCE_MS = Number((import.meta.env.VITE_VOICE_MAX_UTTERANCE_MS as string | undefined) || '15000');
const BARGE_IN_MIN_CHARS = 2;
const BARGE_IN_ECHO_SIMILARITY = 0.68;

function canUseWailsRuntime() {
    return Boolean((window as any).runtime);
}

function clamp(value: number, min: number, max: number) {
    return Math.min(max, Math.max(min, value));
}

function readStoredPetScale() {
    const value = Number(localStorage.getItem(PET_SCALE_KEY) ?? localStorage.getItem(LEGACY_PET_SCALE_KEY));
    if (!Number.isFinite(value)) {
        return 1;
    }
    return clamp(value, PET_MIN_SCALE, PET_MAX_SCALE);
}

function isEditableTarget(target: EventTarget | null) {
    const element = target as HTMLElement | null;
    return Boolean(element?.closest('input, textarea, [contenteditable="true"]'));
}

function visibleChatMessages(items: Message[], maxRounds: number) {
    if (!Number.isFinite(maxRounds) || maxRounds <= 0) {
        return items;
    }

    const userIndexes = items
        .map((message, index) => message.role === 'user' ? index : -1)
        .filter((index) => index >= 0);
    if (userIndexes.length <= maxRounds) {
        return items;
    }

    return items.slice(userIndexes[userIndexes.length - maxRounds]);
}

function decodeBase64Audio(base64: string) {
    const binary = window.atob(base64);
    const bytes = new Uint8Array(binary.length);
    for (let index = 0; index < binary.length; index += 1) {
        bytes[index] = binary.charCodeAt(index);
    }
    return bytes;
}

function blobToBase64(blob: Blob) {
    return new Promise<string>((resolve, reject) => {
        const reader = new FileReader();
        reader.onload = () => {
            const result = String(reader.result || '');
            resolve(result.includes(',') ? result.split(',').pop() || '' : result);
        };
        reader.onerror = () => reject(reader.error || new Error('Failed to read audio blob'));
        reader.readAsDataURL(blob);
    });
}

function normalizeSpeechLanguage(value: string | null | undefined) {
    return String(value || '').trim().toLowerCase().startsWith('zh') ? 'zh' : 'ja';
}

function speechRecognitionLang(language: string) {
    return normalizeSpeechLanguage(language) === 'zh' ? 'zh-CN' : 'ja-JP';
}

function isInterruptedPlaybackError(reason: unknown) {
    const message = String(reason instanceof Error ? reason.message : reason);
    return reason instanceof DOMException && reason.name === 'AbortError'
        || message.includes('play() request was interrupted')
        || message.includes('AbortError');
}

function isWithinQuietHours(value: string) {
    const match = value.match(/^(\d{1,2}):(\d{2})-(\d{1,2}):(\d{2})$/);
    if (!match) {
        return false;
    }
    const [, startHour, startMinute, endHour, endMinute] = match;
    const start = Number(startHour) * 60 + Number(startMinute);
    const end = Number(endHour) * 60 + Number(endMinute);
    const now = new Date();
    const current = now.getHours() * 60 + now.getMinutes();
    if (start === end) {
        return false;
    }
    if (start < end) {
        return current >= start && current < end;
    }
    return current >= start || current < end;
}

function normalizeSpeechText(value: string) {
    return value
        .trim()
        .toLowerCase()
        .replace(/[\s，。！？、,.!?~～"'“”‘’：:；;（）()[\]{}<>《》]/g, '');
}

function textSimilarity(left: string, right: string) {
    const a = normalizeSpeechText(left);
    const b = normalizeSpeechText(right);
    if (!a || !b) {
        return 0;
    }
    let matches = 0;
    const used = new Set<number>();
    for (const char of a) {
        const index = [...b].findIndex((candidate, candidateIndex) => candidate === char && !used.has(candidateIndex));
        if (index >= 0) {
            used.add(index);
            matches += 1;
        }
    }
    return matches / Math.max(a.length, b.length);
}

function isLikelyNoiseTranscript(text: string) {
    const normalized = normalizeSpeechText(text);
    if (!normalized) {
        return true;
    }
    const noiseWords = new Set([
        '啊', '呃', '额', '嗯', '唔', '哦', '喔', '诶', '欸', '哎', '唉',
        '哈', '哈哈', '呵呵', '嗯嗯', '啊啊', '呃呃',
        'um', 'uh', 'hmm', 'mmm', 'ah', 'oh',
    ]);
    if (noiseWords.has(normalized)) {
        return true;
    }
    if (/^[啊呃额嗯唔哦喔诶欸哎唉哈呵]+$/.test(normalized) && normalized.length <= 4) {
        return true;
    }
    return false;
}

function isUsableVoiceTranscript(text: string, confidence = 0, options: {fromLoop?: boolean; assistantLine?: string} = {}) {
    const normalized = normalizeSpeechText(text);
    const minChars = options.fromLoop ? VOICE_LOOP_MIN_CHARS : VOICE_MIN_CHARS;
    if (normalized.length < minChars || isLikelyNoiseTranscript(text)) {
        return false;
    }
    if (confidence > 0 && confidence < VOICE_LOW_CONFIDENCE && normalized.length < 8) {
        return false;
    }
    if (options.assistantLine && textSimilarity(text, options.assistantLine) >= BARGE_IN_ECHO_SIMILARITY) {
        return false;
    }
    return true;
}

function inferAvatarPerformance(text: string, emotion: string, isSpeaking: boolean): AvatarPerformance {
    const line = text.trim();
    const lower = line.toLowerCase();
    const hasQuestion = /[?？]|\bwhy\b|\bhow\b|为什么|怎么|如何|什么/.test(lower);
    const hasExcitement = /[!！]{1,}|太好了|好耶|厉害|不错|开心|喜欢|かわいい|すごい/.test(lower);
    const hasComfort = /没事|别急|慢慢|辛苦|抱抱|安心|大丈夫|そばに/.test(lower);
    const hasTechnical = /代码|报错|配置|接口|模型|延迟|tts|api|bug|日志|构建|测试/i.test(line);
    const hasPlayful = /嘿|哼|欸|诶|嘛|呀|哦|ふふ|えへ|にゃ|喵|～|~/.test(line);
    const hasSad = /抱歉|难过|低落|失败|崩|痛|泪|ごめん|かなしい/.test(lower);
    const hasSurprise = /欸|诶|哇|竟然|真的|突然|surprise|えっ|びっくり/.test(lower);

    let mood: AvatarPerformance['mood'] = 'calm';
    if (emotion === 'surprised' || hasSurprise) {
        mood = 'surprised';
    } else if (emotion === 'happy' || hasExcitement) {
        mood = 'cheer';
    } else if (hasComfort || emotion === 'sad') {
        mood = 'comfort';
    } else if (hasQuestion || emotion === 'thinking') {
        mood = 'curious';
    } else if (hasTechnical || emotion === 'focused') {
        mood = 'confident';
    } else if (hasPlayful) {
        mood = 'playful';
    }

    const energyBase = isSpeaking ? 0.42 : 0.2;
    const energy = clamp(
        energyBase +
        (hasExcitement ? 0.22 : 0) +
        (hasPlayful ? 0.16 : 0) +
        (hasTechnical ? 0.08 : 0) +
        (hasSad ? -0.08 : 0),
        0.12,
        0.92,
    );
    const tiltSeed = ((line.length % 7) - 3) / 3;

    return {
        key: `${line.slice(0, 18)}:${line.length}:${emotion}`,
        mood,
        energy,
        lean: hasTechnical ? 0.32 : hasComfort ? -0.12 : hasPlayful ? 0.22 : 0,
        headTilt: mood === 'curious' ? 0.42 : mood === 'playful' ? tiltSeed * 0.34 : mood === 'surprised' ? -0.18 : tiltSeed * 0.12,
        eyeSmile: mood === 'cheer' || mood === 'playful' ? 0.55 : mood === 'comfort' ? 0.25 : 0.08,
        sparkle: mood === 'cheer' || hasExcitement ? 0.8 : 0,
        blush: mood === 'cheer' || mood === 'playful' ? 0.35 : 0,
        tears: hasSad ? 0.55 : 0,
        puff: mood === 'playful' && /哼|不嘛|才不|む/.test(lower) ? 0.45 : 0,
        hand: hasExcitement ? 'left' : hasTechnical ? 'right' : hasComfort ? 'left' : 'none',
    };
}

function App() {
    const [messages, setMessages] = useState<Message[]>([]);
    const [draft, setDraft] = useState('');
    const [emotion, setEmotion] = useState('neutral');
    const [agentStatus, setAgentStatus] = useState('offline');
    const [agentProvider, setAgentProvider] = useState('unknown');
    const [providerError, setProviderError] = useState('');
    const [isSending, setIsSending] = useState(false);
    const [isObservingScreen, setIsObservingScreen] = useState(false);
    const [error, setError] = useState('');
    const [voiceStatus, setVoiceStatus] = useState('idle');
    const [voiceError, setVoiceError] = useState('');
    const [mouthLevel, setMouthLevel] = useState(0);
    const [speechMetrics, setSpeechMetrics] = useState<SpeechMetric[]>([]);
    const [isPetMode, setIsPetMode] = useState(() => (localStorage.getItem(PET_MODE_KEY) ?? localStorage.getItem(LEGACY_PET_MODE_KEY)) === 'true');
    const [petScale, setPetScale] = useState(readStoredPetScale);
    const [isPetControlsOpen, setIsPetControlsOpen] = useState(false);
    const [isTextInputOpen, setIsTextInputOpen] = useState(false);
    const [continuousVoiceMode, setContinuousVoiceMode] = useState(() => (localStorage.getItem(CONTINUOUS_VOICE_KEY) ?? localStorage.getItem(LEGACY_CONTINUOUS_VOICE_KEY)) === 'true');
    const [conversationMode, setConversationMode] = useState(() => localStorage.getItem(CONVERSATION_MODE_KEY) === 'free' ? 'free' : DEFAULT_CONVERSATION_MODE);
    const [speechLanguage, setSpeechLanguage] = useState(() => normalizeSpeechLanguage(localStorage.getItem(SPEECH_LANGUAGE_KEY) || DEFAULT_SPEECH_LANGUAGE));
    const freeConversationMode = conversationMode === 'free';
    const effectiveContinuousVoiceMode = continuousVoiceMode || freeConversationMode;
    const feedRef = useRef<HTMLDivElement>(null);
    const composerInputRef = useRef<HTMLInputElement>(null);
    const recognitionRef = useRef<any>(null);
    const bargeRecognitionRef = useRef<any>(null);
    const audioRef = useRef<HTMLAudioElement | null>(null);
    const playbackIdRef = useRef(0);
    const audioContextRef = useRef<AudioContext | null>(null);
    const lipSyncCleanupRef = useRef<(() => void) | null>(null);
    const isSendingRef = useRef(false);
    const voiceStatusRef = useRef('idle');
    const voiceLoopRef = useRef(false);
    const voiceEmptyTurnsRef = useRef(0);
    const relistenTimerRef = useRef<number | null>(null);
    const voiceGateCancelRef = useRef<(() => void) | null>(null);
    const voiceGateStreamRef = useRef<MediaStream | null>(null);
    const lastUserActivityRef = useRef(Date.now());
    const lastProactiveAtRef = useRef(0);
    const lastProactiveContextHintAtRef = useRef(0);
    const proactiveSpeechTimestampsRef = useRef<number[]>([]);
    const proactiveInFlightRef = useRef(false);

    const assistantLine = useMemo(() => {
        if (isSending || voiceStatus === 'thinking') {
            return '让我想想...';
        }

        const last = [...messages].reverse().find((message) => message.role === 'assistant');
        return last?.content ?? `你好，我是 ${DESKTOP_PET_NAME}。现在可以通过文字聊天和你互动。`;
    }, [isSending, messages, voiceStatus]);
    const avatarPerformance = useMemo(
        () => inferAvatarPerformance(assistantLine, emotion, voiceStatus === 'speaking'),
        [assistantLine, emotion, voiceStatus],
    );
    const displayedMessages = useMemo(
        () => visibleChatMessages(messages, MAX_VISIBLE_CHAT_ROUNDS),
        [messages],
    );

    useEffect(() => {
        GetState()
            .then((state) => {
                setMessages(state.messages ?? []);
                setEmotion(state.emotion || 'neutral');
                setAgentStatus(state.agentStatus || 'offline');
                setAgentProvider(state.agentProvider || 'unknown');
                setProviderError(state.providerError || '');
            })
            .catch((reason) => setError(String(reason)));
    }, []);

    useEffect(() => {
        const markActivity = () => {
            lastUserActivityRef.current = Date.now();
        };
        window.addEventListener('keydown', markActivity);
        window.addEventListener('pointerdown', markActivity);
        window.addEventListener('wheel', markActivity);
        return () => {
            window.removeEventListener('keydown', markActivity);
            window.removeEventListener('pointerdown', markActivity);
            window.removeEventListener('wheel', markActivity);
        };
    }, []);

    useEffect(() => {
        const canSpeakProactively = isPetMode || (PROACTIVE_FREE_MODE_ENABLED && freeConversationMode);
        if (!PROACTIVE_ENABLED || !canSpeakProactively) {
            return;
        }

        const idleMs = Math.max(1, PROACTIVE_IDLE_MINUTES) * 60 * 1000;
        const cooldownMs = Math.max(1, PROACTIVE_COOLDOWN_MINUTES) * 60 * 1000;
        const contextHintMs = Math.max(1, PROACTIVE_PLUGIN_CONTEXT_HINT_MINUTES) * 60 * 1000;
        const intervalMs = Math.max(5, PROACTIVE_CHECK_SECONDS) * 1000;
        const maxPerHour = Math.floor(PROACTIVE_MAX_PER_HOUR);
        const interval = window.setInterval(() => {
            const now = Date.now();
            const hasPendingVoiceLoop = effectiveContinuousVoiceMode && (
                Boolean(recognitionRef.current) ||
                Boolean(voiceGateStreamRef.current) ||
                relistenTimerRef.current !== null
            );
            if (
                proactiveInFlightRef.current ||
                isSending ||
                isObservingScreen ||
                isTextInputOpen ||
                voiceStatus !== 'idle' ||
                hasPendingVoiceLoop ||
                now - lastUserActivityRef.current < idleMs ||
                now - lastProactiveAtRef.current < cooldownMs ||
                isWithinQuietHours(PROACTIVE_QUIET_HOURS)
            ) {
                return;
            }

            if (maxPerHour <= 0) {
                return;
            }
            proactiveSpeechTimestampsRef.current = proactiveSpeechTimestampsRef.current.filter((timestamp) => now - timestamp < 60 * 60 * 1000);
            if (proactiveSpeechTimestampsRef.current.length >= maxPerHour) {
                return;
            }

            lastProactiveAtRef.current = now;
            if (Math.random() * 100 >= PROACTIVE_CHANCE_PERCENT) {
                return;
            }

            const baseTrigger = isPetMode ? 'pet-idle' : 'free-idle';
            const shouldHintPluginContext = now - lastProactiveContextHintAtRef.current >= contextHintMs;
            const trigger = shouldHintPluginContext ? `${baseTrigger}:screen-context` : baseTrigger;
            proactiveInFlightRef.current = true;
            proactiveSpeechTimestampsRef.current = [...proactiveSpeechTimestampsRef.current, now];
            if (shouldHintPluginContext) {
                lastProactiveContextHintAtRef.current = now;
            }
            GenerateProactiveMessage(trigger)
                .then((response: ChatResponse) => {
                    setMessages(response.messages ?? []);
                    setEmotion(response.emotion || response.reply?.emotion || 'neutral');
                    setAgentStatus(response.agentStatus || 'offline');
                    setAgentProvider(response.agentProvider || 'unknown');
                    setProviderError(response.providerError || '');
                    speakResponse(response);
                })
                .catch((reason) => {
                    console.warn('Proactive message failed:', reason);
                })
                .finally(() => {
                    proactiveInFlightRef.current = false;
                });
        }, intervalMs);

        return () => window.clearInterval(interval);
    }, [effectiveContinuousVoiceMode, freeConversationMode, isObservingScreen, isPetMode, isSending, isTextInputOpen, voiceStatus]);

    useEffect(() => {
        feedRef.current?.scrollTo({
            top: feedRef.current.scrollHeight,
            behavior: 'smooth',
        });
    }, [displayedMessages]);

    function addSpeechMetric(metric: SpeechMetric) {
        setSpeechMetrics((items) => [...items.slice(-11), metric]);
    }

    function clearRelistenTimer() {
        if (relistenTimerRef.current) {
            window.clearTimeout(relistenTimerRef.current);
            relistenTimerRef.current = null;
        }
    }

    function cancelVoiceGate() {
        voiceGateCancelRef.current?.();
        voiceGateCancelRef.current = null;
        voiceGateStreamRef.current?.getTracks().forEach((track) => track.stop());
        voiceGateStreamRef.current = null;
    }

    function resetVoiceEmptyTurns() {
        voiceEmptyTurnsRef.current = 0;
    }

    function registerEmptyVoiceTurn(reason: string) {
        voiceEmptyTurnsRef.current += 1;
        const count = voiceEmptyTurnsRef.current;
        addSpeechMetric({
            phase: 'voice-empty',
            elapsedMs: 0,
            detail: `${reason} ${count}/${VOICE_LOOP_MAX_EMPTY_TURNS}`,
        });
        if (freeConversationMode) {
            voiceEmptyTurnsRef.current = Math.min(count, VOICE_LOOP_MAX_EMPTY_TURNS);
            return true;
        }
        if (count < VOICE_LOOP_MAX_EMPTY_TURNS) {
            return true;
        }

        voiceLoopRef.current = false;
        clearRelistenTimer();
        setVoiceStatus('idle');
        setEmotion('neutral');
        addSpeechMetric({
            phase: 'voice-loop-paused',
            elapsedMs: 0,
            detail: 'too many empty turns',
        });
        return false;
    }

    function nextRelistenDelay() {
        if (voiceEmptyTurnsRef.current <= 0) {
            return VOICE_RELISTEN_DELAY_MS;
        }
        return Math.min(
            VOICE_LOOP_MAX_BACKOFF_MS,
            VOICE_RELISTEN_DELAY_MS + voiceEmptyTurnsRef.current * VOICE_LOOP_EMPTY_BACKOFF_MS,
        );
    }

    async function waitForVoiceGate() {
        if (!VOICE_GATE_ENABLED || !navigator.mediaDevices?.getUserMedia) {
            return true;
        }

        cancelVoiceGate();
        addSpeechMetric({
            phase: 'voice-gate-start',
            elapsedMs: 0,
            detail: `threshold=${VOICE_GATE_THRESHOLD}`,
        });

        let stream: MediaStream;
        try {
            stream = await navigator.mediaDevices.getUserMedia({
                audio: {
                    echoCancellation: true,
                    noiseSuppression: true,
                    autoGainControl: true,
                },
            });
        } catch (reason) {
            addSpeechMetric({
                phase: 'voice-gate-bypass',
                elapsedMs: 0,
                detail: String(reason),
            });
            return true;
        }

        voiceGateStreamRef.current = stream;
        const startedAt = performance.now();
        const AudioContextClass = window.AudioContext || (window as any).webkitAudioContext;
        if (!AudioContextClass) {
            stream.getTracks().forEach((track) => track.stop());
            voiceGateStreamRef.current = null;
            return true;
        }

        const context = new AudioContextClass();
        const source = context.createMediaStreamSource(stream);
        const analyser = context.createAnalyser();
        analyser.fftSize = 1024;
        source.connect(analyser);
        const data = new Uint8Array(analyser.fftSize);

        return new Promise<boolean>((resolve) => {
            let frame = 0;
            let activeSince = 0;
            let settled = false;
            let timeout = 0;

            const finish = (ok: boolean, phase: string, detail: string) => {
                if (settled) {
                    return;
                }
                settled = true;
                window.clearTimeout(timeout);
                window.cancelAnimationFrame(frame);
                source.disconnect();
                analyser.disconnect();
                stream.getTracks().forEach((track) => track.stop());
                if (voiceGateStreamRef.current === stream) {
                    voiceGateStreamRef.current = null;
                }
                voiceGateCancelRef.current = null;
                void context.close();
                addSpeechMetric({
                    phase,
                    elapsedMs: Math.round(performance.now() - startedAt),
                    detail,
                });
                resolve(ok);
            };

            voiceGateCancelRef.current = () => finish(false, 'voice-gate-cancelled', 'cancelled');
            timeout = window.setTimeout(() => finish(false, 'voice-gate-timeout', 'no voice activity'), VOICE_GATE_TIMEOUT_MS);

            const sample = (now: number) => {
                if (!voiceLoopRef.current || recognitionRef.current || isSendingRef.current || voiceStatusRef.current === 'speaking') {
                    finish(false, 'voice-gate-cancelled', 'state changed');
                    return;
                }

                analyser.getByteTimeDomainData(data);
                let sum = 0;
                for (let index = 0; index < data.length; index += 1) {
                    const value = (data[index] - 128) / 128;
                    sum += value * value;
                }
                const rms = Math.sqrt(sum / data.length);
                if (rms >= VOICE_GATE_THRESHOLD) {
                    activeSince = activeSince || now;
                    if (now - activeSince >= VOICE_GATE_HOLD_MS) {
                        finish(true, 'voice-gate-open', `rms=${rms.toFixed(3)}`);
                        return;
                    }
                } else {
                    activeSince = 0;
                }
                frame = window.requestAnimationFrame(sample);
            };

            frame = window.requestAnimationFrame(sample);
        });
    }

    useEffect(() => {
        localStorage.setItem(PET_MODE_KEY, String(isPetMode));
        localStorage.setItem(PET_SCALE_KEY, String(petScale));
        document.documentElement.classList.toggle('pet-window', isPetMode);
        document.body.classList.toggle('pet-window', isPetMode);

        if (!isPetMode) {
            setIsPetControlsOpen(false);
        }

        if (!canUseWailsRuntime()) {
            return;
        }

        WindowSetAlwaysOnTop(isPetMode);
        WindowSetBackgroundColour(isPetMode ? 0 : 27, isPetMode ? 0 : 38, isPetMode ? 0 : 54, isPetMode ? 0 : 255);
        WindowSetSize(
            isPetMode ? Math.round(PET_BASE_WIDTH * petScale) : 1024,
            isPetMode ? Math.round(PET_BASE_HEIGHT * petScale) : 768,
        );
        if (!isPetMode) {
            WindowCenter();
        }
    }, [isPetMode, petScale]);

    useEffect(() => {
        localStorage.setItem(CONTINUOUS_VOICE_KEY, String(continuousVoiceMode));
        if (!effectiveContinuousVoiceMode) {
            voiceLoopRef.current = false;
        }
    }, [continuousVoiceMode, effectiveContinuousVoiceMode]);

    useEffect(() => {
        localStorage.setItem(CONVERSATION_MODE_KEY, conversationMode);
    }, [conversationMode]);

    useEffect(() => {
        localStorage.setItem(SPEECH_LANGUAGE_KEY, speechLanguage);
    }, [speechLanguage]);

    useEffect(() => {
        voiceStatusRef.current = voiceStatus;
    }, [voiceStatus]);

    useEffect(() => {
        if (!freeConversationMode) {
            return;
        }

        voiceLoopRef.current = true;
        if (
            voiceStatus === 'idle' &&
            !isSending &&
            !recognitionRef.current &&
            !voiceGateStreamRef.current &&
            relistenTimerRef.current === null
        ) {
            void startVoiceInput(true);
        }
    }, [freeConversationMode, isSending, voiceStatus]);

    useEffect(() => {
        if (!canUseWailsRuntime()) {
            return;
        }

        let isDisposed = false;
        let frame = 0;

        const reportPetHitTest = () => {
            window.cancelAnimationFrame(frame);
            frame = window.requestAnimationFrame(() => {
                if (isDisposed) {
                    return;
                }
                if (!isPetMode) {
                    void UpdatePetHitTest({enabled: false, controlsOpen: false, x: 0, y: 0, width: 0, height: 0});
                    return;
                }

                const stage = document.querySelector<HTMLElement>('.live2d-stage');
                const rect = stage?.getBoundingClientRect();
                if (!rect || rect.width <= 0 || rect.height <= 0) {
                    return;
                }

                const x = rect.left + rect.width * PET_HIT_INSET_X;
                const y = rect.top + rect.height * PET_HIT_INSET_TOP;
                const width = rect.width * (1 - PET_HIT_INSET_X * 2);
                const height = rect.height * (1 - PET_HIT_INSET_TOP - PET_HIT_INSET_BOTTOM);
                void UpdatePetHitTest({
                    enabled: true,
                    controlsOpen: isPetControlsOpen,
                    x,
                    y,
                    width,
                    height,
                });
            });
        };

        reportPetHitTest();
        const interval = window.setInterval(reportPetHitTest, 250);
        window.addEventListener('resize', reportPetHitTest);
        return () => {
            isDisposed = true;
            window.cancelAnimationFrame(frame);
            window.clearInterval(interval);
            window.removeEventListener('resize', reportPetHitTest);
            void UpdatePetHitTest({enabled: false, controlsOpen: false, x: 0, y: 0, width: 0, height: 0});
        };
    }, [isPetMode, petScale, isPetControlsOpen]);

    useEffect(() => {
        function onKeyDown(event: KeyboardEvent) {
            if (event.key === 'Escape') {
                voiceLoopRef.current = false;
                if (relistenTimerRef.current) {
                    window.clearTimeout(relistenTimerRef.current);
                }
                cancelVoiceGate();
                recognitionRef.current?.abort?.();
                bargeRecognitionRef.current?.abort?.();
                bargeRecognitionRef.current = null;
                audioRef.current?.pause();
                window.speechSynthesis?.cancel?.();
                setVoiceStatus('idle');
                setIsPetControlsOpen(false);
                setIsTextInputOpen(false);
                return;
            }

            if (
                isPetMode &&
                !event.ctrlKey &&
                !event.metaKey &&
                !event.altKey &&
                event.key.toLowerCase() === 'v' &&
                !isEditableTarget(event.target)
            ) {
                event.preventDefault();
                if (voiceStatus === 'listening') {
                    voiceLoopRef.current = false;
                    cancelVoiceGate();
                    recognitionRef.current?.stop?.();
                    setVoiceStatus('idle');
                    return;
                }
                if (voiceStatus === 'speaking') {
                    audioRef.current?.pause();
                    window.speechSynthesis?.cancel?.();
                    setVoiceStatus('idle');
                    void startVoiceInput();
                    return;
                }
                if (!isSending && voiceStatus !== 'thinking') {
                    void startVoiceInput();
                }
                return;
            }

            if (!event.ctrlKey || !event.shiftKey || event.key.toLowerCase() !== 'm') {
                return;
            }

            event.preventDefault();
            if (isPetMode) {
                setIsPetControlsOpen((value) => !value);
                return;
            }

            setIsPetMode(true);
            setIsPetControlsOpen(false);
        }

        window.addEventListener('keydown', onKeyDown);
        return () => window.removeEventListener('keydown', onKeyDown);
    }, [effectiveContinuousVoiceMode, isPetMode, isSending, voiceStatus]);

    useEffect(() => {
        if (isTextInputOpen) {
            composerInputRef.current?.focus();
        }
    }, [isTextInputOpen]);

    useEffect(() => {
        return () => {
            recognitionRef.current?.abort?.();
            bargeRecognitionRef.current?.abort?.();
            cancelVoiceGate();
            if (relistenTimerRef.current) {
                window.clearTimeout(relistenTimerRef.current);
            }
            audioRef.current?.pause();
            window.speechSynthesis?.cancel?.();
            document.documentElement.classList.remove('pet-window');
            document.body.classList.remove('pet-window');
        };
    }, []);

    function finishSpeaking(playbackId?: number) {
        if (playbackId !== undefined && playbackId !== playbackIdRef.current) {
            return;
        }
        bargeRecognitionRef.current?.abort?.();
        bargeRecognitionRef.current = null;
        lipSyncCleanupRef.current?.();
        lipSyncCleanupRef.current = null;
        setMouthLevel(0);
        setVoiceStatus('idle');
        if (!voiceLoopRef.current) {
            return;
        }

        clearRelistenTimer();
        const delay = nextRelistenDelay();
        relistenTimerRef.current = window.setTimeout(() => {
            relistenTimerRef.current = null;
            if (!voiceLoopRef.current || recognitionRef.current) {
                return;
            }
            if (isSendingRef.current || voiceStatusRef.current === 'thinking' || voiceStatusRef.current === 'speaking') {
                finishSpeaking();
                return;
            }
            void startVoiceInput(true);
        }, delay);
    }

    function stopCurrentAudio() {
        playbackIdRef.current += 1;
        cancelVoiceGate();
        bargeRecognitionRef.current?.abort?.();
        bargeRecognitionRef.current = null;
        lipSyncCleanupRef.current?.();
        lipSyncCleanupRef.current = null;
        setMouthLevel(0);
        const audio = audioRef.current;
        if (audio) {
            audio.onended = null;
            audio.onerror = null;
            audio.pause();
            audio.removeAttribute('src');
            audio.load();
            audioRef.current = null;
        }
        window.speechSynthesis?.cancel?.();
    }

    function isBargeInCandidate(text: string, confidence = 0) {
        if (normalizeSpeechText(text).length < BARGE_IN_MIN_CHARS) {
            return false;
        }
        return isUsableVoiceTranscript(text, confidence, {fromLoop: true, assistantLine});
    }

    function interruptWithBargeIn(text: string, playbackId: number) {
        const content = text.trim();
        if (!content || playbackId !== playbackIdRef.current || isSendingRef.current) {
            return;
        }
        setSpeechMetrics((items) => [...items.slice(-11), {
            phase: 'barge-in',
            elapsedMs: 0,
            detail: content,
        }]);
        stopCurrentAudio();
        clearRelistenTimer();
        setVoiceStatus('thinking');
        setEmotion('focused');
        setDraft('');
        void sendContent(content);
    }

    function startBargeInListening(playbackId: number) {
        if (!effectiveContinuousVoiceMode || !voiceLoopRef.current || bargeRecognitionRef.current || recognitionRef.current) {
            return;
        }
        const SpeechRecognition = (window as any).SpeechRecognition || (window as any).webkitSpeechRecognition;
        if (!SpeechRecognition) {
            return;
        }

        const recognition = new SpeechRecognition();
        bargeRecognitionRef.current = recognition;
        recognition.lang = speechRecognitionLang(speechLanguage);
        recognition.continuous = true;
        recognition.interimResults = true;

        let finalTranscript = '';
        let latestTranscript = '';
        let interrupted = false;

        const maybeInterrupt = (value: string, confidence = 0) => {
            const content = value.trim();
            if (interrupted || !isBargeInCandidate(content, confidence)) {
                return;
            }
            interrupted = true;
            bargeRecognitionRef.current = null;
            recognition.abort?.();
            interruptWithBargeIn(content, playbackId);
        };

        recognition.onresult = (event: any) => {
            let interimTranscript = '';
            let bestConfidence = 0;
            for (let index = event.resultIndex; index < event.results.length; index += 1) {
                const alternative = event.results[index][0];
                const transcript = alternative?.transcript ?? '';
                bestConfidence = Math.max(bestConfidence, Number(alternative?.confidence || 0));
                if (event.results[index].isFinal) {
                    finalTranscript += transcript;
                } else {
                    interimTranscript += transcript;
                }
            }
            latestTranscript = (finalTranscript || interimTranscript).trim();
            if (finalTranscript.trim()) {
                maybeInterrupt(finalTranscript, bestConfidence);
            }
        };

        recognition.onerror = () => {
            if (bargeRecognitionRef.current === recognition) {
                bargeRecognitionRef.current = null;
            }
        };

        recognition.onend = () => {
            if (bargeRecognitionRef.current === recognition) {
                bargeRecognitionRef.current = null;
            }
            if (!interrupted && finalTranscript.trim()) {
                maybeInterrupt(finalTranscript);
                return;
            }
            if (
                !interrupted &&
                playbackId === playbackIdRef.current &&
                voiceStatusRef.current === 'speaking' &&
                voiceLoopRef.current &&
                !recognitionRef.current &&
                latestTranscript.trim()
            ) {
                maybeInterrupt(latestTranscript);
            }
        };

        try {
            recognition.start();
            setSpeechMetrics((items) => [...items.slice(-11), {
                phase: 'barge-listening',
                elapsedMs: 0,
                detail: 'listening during speech',
            }]);
        } catch {
            bargeRecognitionRef.current = null;
        }
    }

    function attachLipSync(audio: HTMLAudioElement, playbackId: number) {
        const AudioContextConstructor = window.AudioContext || (window as any).webkitAudioContext;
        if (!AudioContextConstructor) {
            return;
        }

        try {
            const audioContext = audioContextRef.current ?? new AudioContextConstructor();
            audioContextRef.current = audioContext;
            const source = audioContext.createMediaElementSource(audio);
            const analyser = audioContext.createAnalyser();
            analyser.fftSize = 512;
            analyser.smoothingTimeConstant = 0.45;
            source.connect(analyser);
            analyser.connect(audioContext.destination);

            const samples = new Uint8Array(analyser.fftSize);
            let frame = 0;
            let active = true;
            let lastLevel = 0;
            const tick = () => {
                if (!active || playbackId !== playbackIdRef.current) {
                    return;
                }
                analyser.getByteTimeDomainData(samples);
                let sum = 0;
                for (const sample of samples) {
                    const centered = (sample - 128) / 128;
                    sum += centered * centered;
                }
                const rms = Math.sqrt(sum / samples.length);
                const nextLevel = Math.min(1, Math.max(0, rms * 4.8));
                lastLevel += (nextLevel - lastLevel) * 0.55;
                setMouthLevel(lastLevel < 0.018 ? 0 : lastLevel);
                frame = window.requestAnimationFrame(tick);
            };

            lipSyncCleanupRef.current?.();
            lipSyncCleanupRef.current = () => {
                active = false;
                if (frame) {
                    window.cancelAnimationFrame(frame);
                }
                setMouthLevel(0);
                source.disconnect();
                analyser.disconnect();
            };

            void audioContext.resume?.();
            tick();
        } catch (error) {
            console.warn('Lip sync analyser unavailable:', error);
        }
    }

    function speakWithSystemVoice(text: string, playbackId = playbackIdRef.current) {
        if (!('speechSynthesis' in window) || !text.trim()) {
            finishSpeaking();
            return;
        }

        window.speechSynthesis.cancel();

        const utterance = new SpeechSynthesisUtterance(text);
        utterance.lang = 'zh-CN';
        utterance.rate = 1;
        utterance.pitch = 1.05;
        utterance.volume = 1;

        const voices = window.speechSynthesis.getVoices();
        const chineseVoice = voices.find((voice) => voice.lang.toLowerCase().startsWith('zh'));
        if (chineseVoice) {
            utterance.voice = chineseVoice;
        }

        utterance.onstart = () => {
            setVoiceStatus('speaking');
            startBargeInListening(playbackId);
        };
        utterance.onend = () => finishSpeaking();
        utterance.onerror = () => finishSpeaking();
        window.speechSynthesis.speak(utterance);
    }

    function cloudVoiceFallback(message: string, content: string) {
        if (ALLOW_SYSTEM_TTS_FALLBACK) {
            speakWithSystemVoice(content);
            return;
        }
        void speakWithBufferedCloudVoice(content, message);
    }

    async function speakWithBufferedCloudVoice(content: string, fallbackMessage?: string) {
        const playbackId = playbackIdRef.current;
        audioRef.current?.pause();
        const startedAt = performance.now();
        addSpeechMetric({
            phase: 'buffered-request-start',
            elapsedMs: 0,
            detail: 'Fish ordinary TTS',
        });

        try {
            const speech = await SynthesizeSpeech(content);
            addSpeechMetric({
                phase: 'buffered-audio-ready',
                elapsedMs: Math.round(performance.now() - startedAt),
                detail: speech.provider || speech.contentType || '',
            });
            if (playbackId !== playbackIdRef.current) {
                return true;
            }
            const audio = new Audio(`data:${speech.contentType || 'audio/mpeg'};base64,${speech.audioBase64}`);
            audioRef.current = audio;
            attachLipSync(audio, playbackId);
            audio.onended = () => finishSpeaking(playbackId);
            audio.onerror = () => {
                if (playbackId !== playbackIdRef.current) {
                    return;
                }
                if (!ALLOW_SYSTEM_TTS_FALLBACK) {
                    setVoiceError('云端 TTS 音频播放失败，已停止播放。');
                    finishSpeaking(playbackId);
                    return;
                }
                setVoiceError('云端 TTS 音频播放失败，已切换到系统朗读。');
                speakWithSystemVoice(content, playbackId);
            };
            await audio.play();
            addSpeechMetric({
                phase: 'buffered-play-started',
                elapsedMs: Math.round(performance.now() - startedAt),
            });
            startBargeInListening(playbackId);
            return true;
        } catch (reason) {
            if (playbackId !== playbackIdRef.current) {
                return true;
            }
            if (isInterruptedPlaybackError(reason)) {
                finishSpeaking(playbackId);
                return true;
            }
            if (!ALLOW_SYSTEM_TTS_FALLBACK) {
                console.warn('Cloud TTS failed:', reason);
                setVoiceError(fallbackMessage || '云端 TTS 暂时连不上，已停止播放。请检查 TTS_PROVIDER、API Key 或网络。');
                finishSpeaking(playbackId);
                return false;
            }
            setVoiceError(`云端 TTS 失败：${String(reason)}`);
            speakWithSystemVoice(content, playbackId);
            return true;
        }
    }

    async function speakWithStreamedCloudVoice(
        text: string,
        startStream: () => Promise<any> = () => SynthesizeSpeechStream(text),
        allowBufferedFallback = true,
    ) {
        const playbackId = playbackIdRef.current;
        const frontendStartedAt = performance.now();
        const MediaSourceConstructor = (window as any).MediaSource;
        if (!MediaSourceConstructor || !MediaSourceConstructor.isTypeSupported?.('audio/mpeg')) {
            return false;
        }

        const mediaSource = new MediaSourceConstructor();
        const audio = new Audio(URL.createObjectURL(mediaSource));
        audioRef.current = audio;
        attachLipSync(audio, playbackId);

        let streamSessionId = '';
        let sourceBuffer: SourceBuffer | null = null;
        let streamDone = false;
        let failed = false;
        let firstChunkSeen = false;
        const queue: Uint8Array[] = [];
        const unsubs: Array<() => void> = [];
        let resolveFirstChunkOrFailure: ((started: boolean) => void) | null = null;
        const firstChunkOrFailure = new Promise<boolean>((resolve) => {
            resolveFirstChunkOrFailure = resolve;
        });

        const addSpeechMetric = (metric: SpeechMetric) => {
            setSpeechMetrics((items) => [...items.slice(-11), metric]);
        };

        const resolveStreamStarted = (started: boolean) => {
            if (!resolveFirstChunkOrFailure) {
                return;
            }
            resolveFirstChunkOrFailure(started);
            resolveFirstChunkOrFailure = null;
        };

        const handleStreamFailure = (message: string) => {
            if (playbackId !== playbackIdRef.current) {
                return;
            }
            if (!firstChunkSeen) {
                resolveStreamStarted(false);
            }
            if (allowBufferedFallback && !firstChunkSeen) {
                console.warn('Fish Audio streaming failed before audio, falling back to buffered TTS:', message);
                return;
            }
            console.warn('Fish Audio streaming ended after audio started:', message);
            finishSpeaking(playbackId);
        };

        const cleanup = () => {
            unsubs.forEach((unsubscribe) => unsubscribe());
            URL.revokeObjectURL(audio.src);
        };

        const appendNext = () => {
            if (playbackId !== playbackIdRef.current) {
                queue.length = 0;
                return;
            }
            if (!sourceBuffer || sourceBuffer.updating || queue.length === 0) {
                if (streamDone && sourceBuffer && !sourceBuffer.updating && mediaSource.readyState === 'open') {
                    try {
                        mediaSource.endOfStream();
                    } catch {
                        // MediaSource may already be closed by the browser.
                    }
                }
                return;
            }

            const next = queue.shift();
            if (!next) {
                return;
            }
            try {
                sourceBuffer.appendBuffer(next);
            } catch {
                failed = true;
                cleanup();
                handleStreamFailure('Fish Audio 流式音频拼接失败。');
            }
        };

        mediaSource.addEventListener('sourceopen', () => {
            try {
                const buffer = mediaSource.addSourceBuffer('audio/mpeg');
                sourceBuffer = buffer;
                buffer.addEventListener('updateend', appendNext);
                appendNext();
            } catch {
                failed = true;
                cleanup();
                handleStreamFailure('当前 WebView 不支持 Fish Audio 流式播放。');
            }
        }, {once: true});

        audio.onended = () => {
            cleanup();
            finishSpeaking(playbackId);
        };
        audio.onerror = () => {
            if (playbackId !== playbackIdRef.current) {
                return;
            }
            if (failed) {
                return;
            }
            failed = true;
            cleanup();
            handleStreamFailure('Fish Audio 流式音频播放失败。');
        };

        unsubs.push(EventsOn('mochi:speech:start', (event: SpeechStreamEvent) => {
            if (playbackId !== playbackIdRef.current) {
                return;
            }
            if (streamSessionId || !event?.sessionId) {
                return;
            }
            streamSessionId = event.sessionId;
        }));
        unsubs.push(EventsOn('mochi:speech:chunk', (event: SpeechStreamEvent) => {
            if (playbackId !== playbackIdRef.current) {
                return;
            }
            if (!event?.audioBase64 || (streamSessionId && event.sessionId !== streamSessionId)) {
                return;
            }
            if (!streamSessionId && event.sessionId) {
                streamSessionId = event.sessionId;
            }
            if (!firstChunkSeen) {
                firstChunkSeen = true;
                resolveStreamStarted(true);
                addSpeechMetric({
                    phase: 'frontend-first-chunk',
                    elapsedMs: Math.round(performance.now() - frontendStartedAt),
                    detail: event.provider,
                });
            }
            queue.push(decodeBase64Audio(event.audioBase64));
            appendNext();
        }));
        unsubs.push(EventsOn('mochi:speech:metric', (event: SpeechStreamEvent) => {
            if (playbackId !== playbackIdRef.current) {
                return;
            }
            if (streamSessionId && event?.sessionId !== streamSessionId) {
                return;
            }
            addSpeechMetric({
                phase: event?.phase || 'unknown',
                elapsedMs: event?.elapsedMs,
                detail: event?.detail,
            });
        }));
        unsubs.push(EventsOn('mochi:speech:done', (event: SpeechStreamEvent) => {
            if (playbackId !== playbackIdRef.current) {
                return;
            }
            if (streamSessionId && event?.sessionId !== streamSessionId) {
                return;
            }
            streamDone = true;
            if (!firstChunkSeen) {
                resolveStreamStarted(false);
            }
            appendNext();
        }));
        unsubs.push(EventsOn('mochi:speech:error', (event: SpeechStreamEvent) => {
            if (playbackId !== playbackIdRef.current) {
                return;
            }
            if (streamSessionId && event?.sessionId !== streamSessionId) {
                return;
            }
            failed = true;
            cleanup();
            console.warn('Fish Audio streaming failed:', event?.error || 'unknown error');
            handleStreamFailure(`Fish Audio 流式 TTS 失败：${event?.error || '请检查网络或 Fish 配置。'}`);
        }));

        try {
            const stream = await startStream();
            if (playbackId !== playbackIdRef.current) {
                cleanup();
                return true;
            }
            streamSessionId = stream.sessionId;
            await audio.play();
            setVoiceStatus('speaking');
            startBargeInListening(playbackId);
            return await firstChunkOrFailure;
        } catch (error) {
            cleanup();
            if (playbackId !== playbackIdRef.current) {
                return true;
            }
            if (isInterruptedPlaybackError(error)) {
                finishSpeaking(playbackId);
                return true;
            }
            resolveStreamStarted(false);
            throw error;
        }
    }

    async function speakText(text: string) {
        const content = text.trim();
        if (!content) {
            finishSpeaking();
            return;
        }

        stopCurrentAudio();
        const playbackId = playbackIdRef.current;
        setVoiceStatus('speaking');

        if (SPEECH_OUTPUT_MODE === 'system') {
            speakWithSystemVoice(content, playbackId);
            return;
        }

        try {
            if (SPEECH_OUTPUT_MODE === 'cloud' && ENABLE_STREAMING_TTS) {
                const streamingStarted = await speakWithStreamedCloudVoice(content);
                if (streamingStarted) {
                    return;
                }
            }
            await speakWithBufferedCloudVoice(content);
        } catch (reason) {
            if (playbackId !== playbackIdRef.current) {
                return;
            }
            if (isInterruptedPlaybackError(reason)) {
                finishSpeaking(playbackId);
                return;
            }
            if (!ALLOW_SYSTEM_TTS_FALLBACK) {
                console.warn('Cloud TTS failed:', reason);
                setVoiceError('云端 TTS 暂时连不上，已停止播放。请检查 TTS_PROVIDER、API Key 或网络。');
                finishSpeaking(playbackId);
                return;
            }
            setVoiceError(`云端 TTS 失败：${String(reason)}`);
            speakWithSystemVoice(content, playbackId);
        }
    }

    function speechContentForResponse(response: ChatResponse) {
        const text = response.reply?.content?.trim() || '';
        const speechText = response.speechText?.trim() || '';
        if (speechLanguage === 'zh') {
            return text || speechText;
        }
        return speechText || text;
    }

    function speakResponse(response: ChatResponse) {
        void speakText(speechContentForResponse(response));
    }

    async function sendContent(rawContent: string) {
        const content = rawContent.trim();
        if (!content || isSendingRef.current) {
            return;
        }

        resetVoiceEmptyTurns();
        lastUserActivityRef.current = Date.now();
        setDraft('');
        setIsTextInputOpen(false);
        setIsPetControlsOpen(false);
        isSendingRef.current = true;
        setIsSending(true);
        setError('');
        setVoiceError('');
        setSpeechMetrics([{
            phase: 'frontend-mode',
            elapsedMs: 0,
            detail: `mode=${SPEECH_OUTPUT_MODE}, streaming=${ENABLE_STREAMING_TTS}, realtime=${ENABLE_REALTIME_SPEECH}`,
        }]);
        setVoiceStatus('thinking');
        setEmotion('focused');

        setSpeechMetrics((items) => [...items, {
            phase: 'realtime-disabled',
            elapsedMs: 0,
            detail: `mode=${SPEECH_OUTPUT_MODE}, streaming=${ENABLE_STREAMING_TTS}, realtime=${ENABLE_REALTIME_SPEECH}; speechLanguage=${speechLanguage}`,
        }]);

        try {
            const response = await SendMessage(content) as ChatResponse;
            setMessages(response.messages ?? []);
            setEmotion(response.emotion || response.reply?.emotion || 'neutral');
            setAgentStatus(response.agentStatus || 'offline');
            setAgentProvider(response.agentProvider || 'unknown');
            setProviderError(response.providerError || '');
            speakResponse(response);
        } catch (reason) {
            setError(String(reason));
            finishSpeaking();
        } finally {
            isSendingRef.current = false;
            setIsSending(false);
        }
    }

    function sendMessage(event: FormEvent) {
        event.preventDefault();
        void sendContent(draft);
    }

    function resizePetWithWheel(event: WheelEvent<HTMLElement>) {
        if (!isPetMode) {
            return;
        }

        event.preventDefault();
        setPetScale((scale) => {
            const direction = event.deltaY < 0 ? 1 : -1;
            return Number(clamp(scale + direction * PET_SCALE_STEP, PET_MIN_SCALE, PET_MAX_SCALE).toFixed(2));
        });
    }

    async function clearChat() {
        if (isSending || voiceStatus === 'speaking') {
            return;
        }

        setError('');
        setVoiceError('');
        try {
            const state = await ClearChat();
            setMessages(state.messages ?? []);
            setEmotion(state.emotion || 'neutral');
            setAgentStatus(state.agentStatus || 'offline');
            setAgentProvider(state.agentProvider || 'unknown');
            setProviderError(state.providerError || '');
        } catch (reason) {
            setError(String(reason));
        }
    }

    async function observeScreen() {
        if (isSendingRef.current || isObservingScreen || voiceStatus === 'thinking') {
            return;
        }

        lastUserActivityRef.current = Date.now();
        isSendingRef.current = true;
        setIsSending(true);
        setIsObservingScreen(true);
        setError('');
        setVoiceError('');
        setVoiceStatus('thinking');
        setEmotion('focused');
        try {
            const response = await ObserveScreen('Look at the current screen and tell me what you notice.') as ChatResponse;
            setMessages(response.messages ?? []);
            setEmotion(response.emotion || response.reply?.emotion || 'thinking');
            setAgentStatus(response.agentStatus || 'offline');
            setAgentProvider(response.agentProvider || 'unknown');
            setProviderError(response.providerError || '');
            speakResponse(response);
        } catch (reason) {
            setError(String(reason));
            finishSpeaking();
        } finally {
            isSendingRef.current = false;
            setIsSending(false);
            setIsObservingScreen(false);
        }
    }

    async function probeFishLive() {
        setVoiceError('');
        setSpeechMetrics([{phase: 'probe-start', elapsedMs: 0, detail: 'Fish live minimal probe'}]);
        try {
            const result = await ProbeFishLive() as FishLiveProbeResult;
            const rows: SpeechMetric[] = [
                {
                    phase: result.ok ? 'probe-ok' : 'probe-failed',
                    elapsedMs: result.elapsedMs,
                    detail: result.ok ? `audio=${result.audioSize || 0} bytes` : (result.error || 'unknown error'),
                },
                ...((result.events || []).slice(-10).map((event) => ({
                    phase: 'probe-event',
                    detail: event,
                }))),
            ];
            setSpeechMetrics(rows);
            if (!result.ok && result.error) {
                setVoiceError(`Fish live probe failed: ${result.error}`);
            }
        } catch (reason) {
            setVoiceError(String(reason));
            setSpeechMetrics((items) => [...items, {phase: 'probe-error', detail: String(reason)}]);
        }
    }

    async function startModelASRVoiceInput(fromLoop = false) {
        if (!fromLoop) {
            voiceLoopRef.current = effectiveContinuousVoiceMode;
            resetVoiceEmptyTurns();
        }
        clearRelistenTimer();

        if (fromLoop && voiceLoopRef.current) {
            const gateOpen = await waitForVoiceGate();
            if (!gateOpen) {
                setVoiceStatus('idle');
                setEmotion('neutral');
                if (voiceLoopRef.current && !isSendingRef.current && voiceStatusRef.current !== 'speaking') {
                    finishSpeaking();
                }
                return;
            }
        }

        if (!navigator.mediaDevices?.getUserMedia || typeof MediaRecorder === 'undefined') {
            setVoiceError('当前 WebView 不支持录音 ASR，已回退到浏览器语音识别。');
            await startBrowserVoiceInput(fromLoop);
            return;
        }

        let stream: MediaStream;
        try {
            stream = await navigator.mediaDevices.getUserMedia({
                audio: {
                    echoCancellation: true,
                    noiseSuppression: true,
                    autoGainControl: true,
                },
            });
        } catch (reason) {
            setVoiceStatus('idle');
            setVoiceError(`麦克风启动失败：${String(reason)}`);
            if (voiceLoopRef.current && registerEmptyVoiceTurn('mic-failed')) {
                finishSpeaking();
            }
            return;
        }

        const AudioContextClass = window.AudioContext || (window as any).webkitAudioContext;
        const audioContext = AudioContextClass ? new AudioContextClass() : null;
        const source = audioContext?.createMediaStreamSource(stream);
        const analyser = audioContext?.createAnalyser();
        if (analyser) {
            analyser.fftSize = 1024;
            source?.connect(analyser);
        }

        const contentType = MediaRecorder.isTypeSupported?.('audio/webm;codecs=opus') ? 'audio/webm;codecs=opus' : 'audio/webm';
        const recorder = new MediaRecorder(stream, {mimeType: contentType});
        const chunks: Blob[] = [];
        const startedAt = performance.now();
        let frame = 0;
        let stopped = false;
        let silentSince = 0;
        const data = new Uint8Array(analyser?.fftSize || 1024);

        const cleanup = () => {
            if (frame) {
                window.cancelAnimationFrame(frame);
            }
            source?.disconnect();
            analyser?.disconnect();
            void audioContext?.close?.();
            stream.getTracks().forEach((track) => track.stop());
        };

        const stopRecording = (reason: string) => {
            if (stopped) {
                return;
            }
            stopped = true;
            addSpeechMetric({
                phase: 'asr-record-stop',
                elapsedMs: Math.round(performance.now() - startedAt),
                detail: reason,
            });
            recorder.stop();
        };

        recorder.ondataavailable = (event) => {
            if (event.data?.size) {
                chunks.push(event.data);
            }
        };

        recorder.onstop = async () => {
            cleanup();
            if (!chunks.length) {
                setVoiceStatus('idle');
                if (voiceLoopRef.current && registerEmptyVoiceTurn('asr-empty-audio')) {
                    finishSpeaking();
                }
                return;
            }

            setVoiceStatus('thinking');
            try {
                const blob = new Blob(chunks, {type: contentType});
                const audioBase64 = await blobToBase64(blob);
                const reply = await TranscribeAudio(audioBase64, contentType, speechLanguage) as ASRReply;
                const content = String(reply.text || '').trim();
                addSpeechMetric({
                    phase: 'asr-transcribed',
                    elapsedMs: Math.round(performance.now() - startedAt),
                    detail: `${reply.provider || ASR_PROVIDER}: ${content || 'empty'}`,
                });
                if (!content || !isUsableVoiceTranscript(content, 1, {fromLoop, assistantLine})) {
                    setVoiceStatus('idle');
                    setEmotion('neutral');
                    setDraft('');
                    if (voiceLoopRef.current && registerEmptyVoiceTurn(content ? 'asr-filtered' : 'asr-empty')) {
                        finishSpeaking();
                    }
                    return;
                }
                resetVoiceEmptyTurns();
                setDraft(content);
                void sendContent(content);
            } catch (reason) {
                setVoiceStatus('idle');
                setVoiceError(`ASR 识别失败：${String(reason)}`);
                if (voiceLoopRef.current && registerEmptyVoiceTurn('asr-failed')) {
                    finishSpeaking();
                }
            }
        };

        setVoiceError('');
        setVoiceStatus('listening');
        setEmotion('thinking');
        addSpeechMetric({phase: 'asr-record-start', elapsedMs: 0, detail: ASR_PROVIDER});
        recorder.start();

        const sample = (now: number) => {
            if (stopped) {
                return;
            }
            if (!voiceLoopRef.current && fromLoop) {
                stopRecording('loop stopped');
                return;
            }
            if (performance.now() - startedAt >= VOICE_MAX_UTTERANCE_MS) {
                stopRecording('max-utterance');
                return;
            }
            if (!analyser) {
                frame = window.requestAnimationFrame(sample);
                return;
            }

            analyser.getByteTimeDomainData(data);
            let sum = 0;
            for (let index = 0; index < data.length; index += 1) {
                const value = (data[index] - 128) / 128;
                sum += value * value;
            }
            const rms = Math.sqrt(sum / data.length);
            const elapsed = performance.now() - startedAt;
            if (elapsed > 450 && rms < VOICE_GATE_THRESHOLD * 0.65) {
                silentSince = silentSince || now;
                if (now - silentSince >= VOICE_AUTO_SUBMIT_SILENCE_MS) {
                    stopRecording(`silence rms=${rms.toFixed(3)}`);
                    return;
                }
            } else {
                silentSince = 0;
            }
            frame = window.requestAnimationFrame(sample);
        };
        frame = window.requestAnimationFrame(sample);
    }

    async function startVoiceInput(fromLoop = false) {
        if (ASR_PROVIDER !== 'browser' && ASR_PROVIDER !== 'webspeech') {
            await startModelASRVoiceInput(fromLoop);
            return;
        }
        await startBrowserVoiceInput(fromLoop);
    }

    async function startBrowserVoiceInput(fromLoop = false) {
        if (!fromLoop) {
            voiceLoopRef.current = effectiveContinuousVoiceMode;
            resetVoiceEmptyTurns();
        }
        clearRelistenTimer();

        if (fromLoop && voiceLoopRef.current) {
            const gateOpen = await waitForVoiceGate();
            if (!gateOpen) {
                setVoiceStatus('idle');
                setEmotion('neutral');
                if (voiceLoopRef.current && !recognitionRef.current && !isSendingRef.current && voiceStatusRef.current !== 'speaking') {
                    finishSpeaking();
                }
                return;
            }
        }

        const SpeechRecognition = (window as any).SpeechRecognition || (window as any).webkitSpeechRecognition;
        if (!SpeechRecognition) {
            setVoiceError('当前 WebView 不支持浏览器语音识别，可以先继续使用文字输入。');
            setIsTextInputOpen(true);
            voiceLoopRef.current = false;
            return;
        }

        if (voiceStatus === 'listening') {
            if (!fromLoop) {
                voiceLoopRef.current = false;
            }
            recognitionRef.current?.stop?.();
            return;
        }

        window.speechSynthesis?.cancel?.();
        const recognition = new SpeechRecognition();
        recognitionRef.current = recognition;
        recognition.lang = speechRecognitionLang(speechLanguage);
        recognition.continuous = Boolean(fromLoop && voiceLoopRef.current);
        recognition.interimResults = true;

        let finalTranscript = '';
        let latestTranscript = '';
        let bestConfidence = 0;
        let recognitionFailed = false;
        let silenceTimer = 0;
        let maxUtteranceTimer = 0;
        let autoStopping = false;
        setVoiceError('');
        setVoiceStatus('listening');
        setEmotion('thinking');

        const clearVoiceTimers = () => {
            if (silenceTimer) {
                window.clearTimeout(silenceTimer);
                silenceTimer = 0;
            }
            if (maxUtteranceTimer) {
                window.clearTimeout(maxUtteranceTimer);
                maxUtteranceTimer = 0;
            }
        };

        const stopAfterSpeechPause = (reason: string) => {
            if (!fromLoop || autoStopping || !latestTranscript.trim()) {
                return;
            }
            autoStopping = true;
            addSpeechMetric({
                phase: 'voice-auto-submit',
                elapsedMs: 0,
                detail: reason,
            });
            recognition.stop?.();
        };

        const scheduleAutoSubmit = () => {
            if (!fromLoop || !latestTranscript.trim()) {
                return;
            }
            if (silenceTimer) {
                window.clearTimeout(silenceTimer);
            }
            silenceTimer = window.setTimeout(() => stopAfterSpeechPause('silence'), VOICE_AUTO_SUBMIT_SILENCE_MS);
        };

        if (fromLoop) {
            maxUtteranceTimer = window.setTimeout(() => stopAfterSpeechPause('max-utterance'), VOICE_MAX_UTTERANCE_MS);
        }

        recognition.onresult = (event: any) => {
            let interimTranscript = '';
            for (let index = event.resultIndex; index < event.results.length; index += 1) {
                const alternative = event.results[index][0];
                const transcript = alternative?.transcript ?? '';
                bestConfidence = Math.max(bestConfidence, Number(alternative?.confidence || 0));
                if (event.results[index].isFinal) {
                    finalTranscript += transcript;
                } else {
                    interimTranscript += transcript;
                }
            }
            latestTranscript = (finalTranscript || interimTranscript).trim();
            setDraft(latestTranscript);
            scheduleAutoSubmit();
            if (latestTranscript && !isPetMode) {
                setIsTextInputOpen(true);
            }
        };

        recognition.onerror = (event: any) => {
            clearVoiceTimers();
            recognitionFailed = true;
            recognitionRef.current = null;
            const errorName = String(event.error || '');
            setVoiceStatus('idle');
            if (voiceLoopRef.current && (errorName === 'no-speech' || errorName === 'aborted')) {
                if (registerEmptyVoiceTurn(errorName || 'recognition-error')) {
                    finishSpeaking();
                }
                return;
            }
            voiceLoopRef.current = false;
            setVoiceError(errorName ? `语音识别失败：${errorName}` : '语音识别失败。');
        };

        recognition.onend = () => {
            clearVoiceTimers();
            if (recognitionFailed) {
                return;
            }
            const content = (finalTranscript || latestTranscript).trim();
            recognitionRef.current = null;
            if (!content || !isUsableVoiceTranscript(content, bestConfidence, {fromLoop, assistantLine})) {
                setVoiceStatus('idle');
                setEmotion('neutral');
                setDraft('');
                if (voiceLoopRef.current) {
                    addSpeechMetric({
                        phase: 'voice-filtered',
                        elapsedMs: 0,
                        detail: content || 'empty',
                    });
                    if (registerEmptyVoiceTurn(content ? 'filtered' : 'empty')) {
                        finishSpeaking();
                    }
                }
                return;
            }
            resetVoiceEmptyTurns();
            void sendContent(content);
        };

        try {
            recognition.start();
        } catch (reason) {
            clearVoiceTimers();
            recognitionRef.current = null;
            setVoiceStatus('idle');
            if (voiceLoopRef.current) {
                if (registerEmptyVoiceTurn('start-failed')) {
                    finishSpeaking();
                }
                return;
            }
            setVoiceError(`语音识别启动失败：${String(reason)}`);
        }
    }

    const composer = (
        <form className={isTextInputOpen ? 'composer composer-open' : 'composer'} onSubmit={sendMessage}>
            {isTextInputOpen && (
                <input
                    ref={composerInputRef}
                    value={draft}
                    onChange={(event) => setDraft(event.target.value)}
                    placeholder={`和 ${DESKTOP_PET_NAME} 说点什么...`}
                    autoComplete="off"
                />
            )}
            <button
                type="button"
                className="text-button"
                onClick={() => setIsTextInputOpen((value) => !value)}
                aria-pressed={isTextInputOpen}
            >
                文字
            </button>
            <button
                type="button"
                className={`voice-button voice-${voiceStatus}`}
                onClick={() => {
                    if (voiceStatus === 'speaking') {
                        audioRef.current?.pause();
                        window.speechSynthesis?.cancel?.();
                        setVoiceStatus('idle');
                    }
                    void startVoiceInput();
                }}
                disabled={isSending || voiceStatus === 'thinking'}
            >
                {voiceStatus === 'listening' ? '停止' : (freeConversationMode ? '自由' : effectiveContinuousVoiceMode ? '连续' : '语音')}
            </button>
            {isTextInputOpen && (
                <button type="submit" disabled={isSending || !draft.trim()}>
                    {isSending ? '发送中' : '发送'}
                </button>
            )}
        </form>
    );

    return (
        <main className={isPetMode ? 'app-shell pet-mode' : 'app-shell'}>
            <section className="stage" aria-label="Yuyu Live2D 舞台" onWheel={resizePetWithWheel}>
                {!isPetMode && (
                    <div className="status-bar">
                        <span>{DESKTOP_PET_NAME} AI</span>
                        <span>{emotionLabel[emotion] ?? emotion} · Agent {agentStatus} · {agentProvider}</span>
                    </div>
                )}

                <Live2DStage
                    emotion={emotion}
                    isSpeaking={voiceStatus === 'speaking'}
                    mouthLevel={mouthLevel}
                    petScale={petScale}
                    performance={avatarPerformance}
                />

                {SHOW_SPEECH_DEBUG && speechMetrics.length > 0 && (
                    <div className="speech-metrics" aria-label="Speech timing log">
                        <strong>Speech timing</strong>
                        {speechMetrics.map((metric, index) => (
                            <div className="speech-metric-row" key={`${metric.phase}-${index}`}>
                                <span>{metric.phase}</span>
                                <b>{typeof metric.elapsedMs === 'number' ? `${metric.elapsedMs}ms` : '-'}</b>
                                {metric.detail && <small>{metric.detail}</small>}
                            </div>
                        ))}
                    </div>
                )}

                {isPetMode && voiceStatus === 'speaking' && assistantLine.trim() && (
                    <div className="pet-subtitle" aria-live="polite">
                        <p>{assistantLine}</p>
                    </div>
                )}

                {!isPetMode && (
                    <div className="speech-bubble">
                        <p>{assistantLine}</p>
                    </div>
                )}

                {isPetMode && isPetControlsOpen ? (
                    <div className="pet-controls" aria-label="桌宠控制">
                        {composer}
                        <div className="pet-mode-actions">
                            <button type="button" className="pet-mode-toggle" onClick={() => void observeScreen()} disabled={isSending || isObservingScreen}>
                                看屏幕
                            </button>
                            <button type="button" className="pet-mode-toggle" onClick={() => setIsPetControlsOpen(false)}>
                                收起
                            </button>
                            <button type="button" className="pet-mode-toggle" onClick={() => setIsPetMode(false)}>
                                完整模式
                            </button>
                        </div>
                        <span className="pet-shortcut">{PET_CONTROLS_SHORTCUT} · V 语音输入</span>
                    </div>
                ) : !isPetMode && (
                    <div className="stage-tools">
                        <button type="button" onClick={() => void observeScreen()} disabled={isSending || isObservingScreen}>
                            {isObservingScreen ? '观察中' : '看屏幕'}
                        </button>
                        <button type="button">Live2D</button>
                        <button type="button">记忆</button>
                        <button type="button">插件</button>
                        <button type="button">屏幕</button>
                    </div>
                )}
            </section>

            {!isPetMode && (
                <section className="chat-panel" aria-label={`${DESKTOP_PET_NAME} chat`}>
                    <header>
                        <div>
                            <p className="eyebrow">Desktop Companion MVP</p>
                            <h1>文字聊天与本地记忆</h1>
                        </div>
                        <div className="header-actions">
                            <button
                                type="button"
                                className="window-button"
                                onClick={WindowMinimise}
                                aria-label="最小化"
                            >
                                -
                            </button>
                            <button
                                type="button"
                                className="window-button window-close"
                                onClick={Quit}
                                aria-label="关闭"
                            >
                                ×
                            </button>
                            <button
                                type="button"
                                className="ghost-button"
                                onClick={() => setIsPetMode(true)}
                            >
                                桌宠模式
                            </button>
                            <button
                                type="button"
                                className="ghost-button"
                                aria-pressed={freeConversationMode}
                                onClick={() => setConversationMode((value) => value === 'free' ? 'manual' : 'free')}
                            >
                                自由聊天 {freeConversationMode ? '开' : '关'}
                            </button>
                            <button
                                type="button"
                                className="ghost-button"
                                aria-pressed={effectiveContinuousVoiceMode}
                                onClick={() => setContinuousVoiceMode((value) => !value)}
                                disabled={freeConversationMode}
                            >
                                持续对话 {effectiveContinuousVoiceMode ? '开' : '关'}
                            </button>
                            <button
                                type="button"
                                className="ghost-button"
                                aria-pressed={speechLanguage === 'ja'}
                                onClick={() => setSpeechLanguage((value) => value === 'zh' ? 'ja' : 'zh')}
                            >
                                语音 {speechLanguage === 'zh' ? '中文' : '日语'}
                            </button>
                            {SHOW_SPEECH_DEBUG && (
                                <button
                                    type="button"
                                    className="ghost-button"
                                    onClick={probeFishLive}
                                    disabled={isSending || voiceStatus === 'speaking'}
                                >
                                    Fish Live Test
                                </button>
                            )}
                            <button
                                type="button"
                                className="ghost-button"
                                onClick={clearChat}
                                disabled={isSending || voiceStatus === 'speaking' || messages.length === 0}
                            >
                                清空聊天
                            </button>
                            <span className={`pill agent-${agentStatus}`}>Agent {agentStatus} · {agentProvider}</span>
                        </div>
                    </header>

                    <div className="message-feed" ref={feedRef}>
                        {displayedMessages.length === 0 && (
                            <div className="empty-state">
                                <strong>第一步已经就位。</strong>
                                <span>试试输入“你好”或“记住我喜欢中文简洁回复”。</span>
                            </div>
                        )}

                        {displayedMessages.map((message) => (
                            <article className={`message ${message.role}`} key={message.id}>
                            <span className="message-role">{message.role === 'user' ? '你' : DESKTOP_PET_NAME}</span>
                                <p>{message.content}</p>
                            </article>
                        ))}
                    </div>

                    {providerError && <div className="error">Provider fallback: {providerError}</div>}
                    {error && <div className="error">{error}</div>}
                    {voiceError && <div className="error">{voiceError}</div>}

                    {composer}
                </section>
            )}
        </main>
    );
}

export default App;
