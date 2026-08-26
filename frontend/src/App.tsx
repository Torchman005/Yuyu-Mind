import {FormEvent, WheelEvent, useEffect, useMemo, useRef, useState} from 'react';
import './App.css';
import {
    AnswerAgentTaskQuestion,
    CancelAgentTask,
    ClearChat,
    CreateConversation,
    DisablePlugin,
    EnablePlugin,
    GenerateProactiveMessage,
    GetConfigJSON,
    GetLogLevel,
    GetLogs,
    GetMessages,
    GetPluginConfig,
    GetState,
    GetSpeechStreamUrl,
    InvokePluginAction,
    ListAgentTasks,
    ListConversations,
    ListPlugins,
    ObserveScreen,
    ProbeFishLive,
    ReloadPlugins,
    SaveConfigJSON,
    SendAgentTaskControl,
    SetLogLevel,
    SetPluginConfig,
    StreamChat,
    SynthesizeSpeech,
    SynthesizeSpeechStream,
    TranscribeAudio,
    UpdatePetHitTest,
} from '../wailsjs/go/app/App';
import {
    EventsOn,
    Quit,
    WindowCenter,
    WindowMinimise,
    WindowSetAlwaysOnTop,
    WindowSetBackgroundColour,
    WindowSetSize,
} from '../wailsjs/runtime/runtime';
import {app, chat, db, loghub} from '../wailsjs/go/models';
import {AvatarPerformance, Live2DStage} from './components/Live2DStage';
import {ChatComposer, ChatView, ModelView, PetModeView, SkinsView, WebSidebar} from './components/AppShell';
import {
    ASR_PROVIDER,
    ALLOW_SYSTEM_TTS_FALLBACK,
    BARGE_IN_MIN_CHARS,
    CONTINUOUS_VOICE_KEY,
    CONVERSATION_MODE_KEY,
    DEFAULT_CONVERSATION_MODE,
    DEFAULT_SPEECH_LANGUAGE,
    DESKTOP_COMPANION_CONVERSATION_ID,
    DESKTOP_PET_NAME,
    ENABLE_GPT_SOVITS_STREAMING,
    ENABLE_REALTIME_SPEECH,
    ENABLE_STREAMING_TTS,
    FOLLOW_UP_CHANCE_PERCENT,
    FOLLOW_UP_COOLDOWN_MS,
    FOLLOW_UP_DELAY_MS,
    FOLLOW_UP_ENABLED,
    LEGACY_CONTINUOUS_VOICE_KEY,
    LEGACY_PET_MODE_KEY,
    MAX_VISIBLE_CHAT_ROUNDS,
    PET_BASE_HEIGHT,
    PET_BASE_WIDTH,
    PET_CONTROLS_SHORTCUT,
    PET_HIT_INSET_BOTTOM,
    PET_HIT_INSET_TOP,
    PET_HIT_INSET_X,
    PET_MAX_SCALE,
    PET_MIN_SCALE,
    PET_MODE_KEY,
    PET_SCALE_KEY,
    PET_SCALE_STEP,
    PROACTIVE_CHANCE_PERCENT,
    PROACTIVE_CHECK_SECONDS,
    PROACTIVE_COOLDOWN_MINUTES,
    PROACTIVE_ENABLED,
    PROACTIVE_FREE_MODE_ENABLED,
    PROACTIVE_IDLE_MINUTES,
    PROACTIVE_MAX_PER_HOUR,
    PROACTIVE_PLUGIN_CONTEXT_HINT_MINUTES,
    PROACTIVE_QUIET_HOURS,
    REVIEW_ACTION_NAMES,
    SHOW_SPEECH_DEBUG,
    SPEECH_LANGUAGE_KEY,
    SPEECH_OUTPUT_MODE,
    VOICE_AUTO_SUBMIT_SILENCE_MS,
    VOICE_GATE_ENABLED,
    VOICE_GATE_HOLD_MS,
    VOICE_GATE_THRESHOLD,
    VOICE_GATE_TIMEOUT_MS,
    VOICE_LOOP_EMPTY_BACKOFF_MS,
    VOICE_LOOP_MAX_BACKOFF_MS,
    VOICE_LOOP_MAX_EMPTY_TURNS,
    VOICE_MAX_UTTERANCE_MS,
    VOICE_RELISTEN_DELAY_MS,
    taskStatusLabel,
} from './appConfig';
import {
    ASRReply,
    ChatResponse,
    ChatStreamEvent,
    FishLiveProbeResult,
    Message,
    PerformanceHint,
    PluginChange,
    SpeechMetric,
    SpeechStreamEvent,
    ViewKey,
} from './appTypes';
import {
    asrProviderLanguage,
    asrRecognitionLanguage,
    blobToBase64,
    canUseWailsRuntime,
    clamp,
    decodeBase64Audio,
    inferAvatarPerformance,
    isEditableTarget,
    isInterruptedPlaybackError,
    isUsableVoiceTranscript,
    isWithinQuietHours,
    normalizeSpeechLanguage,
    normalizeSpeechText,
    readStoredPetScale,
    speechRecognitionLang,
    textSimilarity,
    visibleChatMessages,
    webmToWav16kMono,
} from './utils';

function App() {
    const [messages, setMessages] = useState<Message[]>([]);
    const [draft, setDraft] = useState('');
    const [emotion, setEmotion] = useState('neutral');
    const [agentStatus, setAgentStatus] = useState('offline');
    const [agentProvider, setAgentProvider] = useState('unknown');
    const [providerError, setProviderError] = useState('');
    const [plugins, setPlugins] = useState<app.PluginInfo[]>([]);
    const [pluginsOpen, setPluginsOpen] = useState(false);
    const [pluginResult, setPluginResult] = useState('');
    const [pluginActionInput, setPluginActionInput] = useState('');
    const [pluginDetail, setPluginDetail] = useState<app.PluginInfo | null>(null);
    const [pluginConfigText, setPluginConfigText] = useState('');
    const [pluginConfigOpen, setPluginConfigOpen] = useState(false);
    const [tasks, setTasks] = useState<db.AgentTask[]>([]);
    const [tasksOpen, setTasksOpen] = useState(false);
    const [taskAnswer, setTaskAnswer] = useState<Record<string, string>>({});
    const [taskReviewDiffOpen, setTaskReviewDiffOpen] = useState<Record<string, boolean>>({});
    const [logs, setLogs] = useState<loghub.Entry[]>([]);
    const [logLevel, setLogLevel] = useState('DEBUG');
    const [logFilter, setLogFilter] = useState('');
    const [logLoading, setLogLoading] = useState(false);
    const [performanceHint, setPerformanceHint] = useState<PerformanceHint | null>(null);
    const [isSending, setIsSending] = useState(false);
    const [isObservingScreen, setIsObservingScreen] = useState(false);
    const [error, setError] = useState('');
    const [voiceStatus, setVoiceStatus] = useState('idle');
    const [voiceError, setVoiceError] = useState('');
    const [mouthLevel, setMouthLevel] = useState(0);
    const [speechMetrics, setSpeechMetrics] = useState<SpeechMetric[]>([]);
    const [isPetMode, setIsPetMode] = useState(true);
    const [petScale, setPetScale] = useState(readStoredPetScale);
    const [isPetControlsOpen, setIsPetControlsOpen] = useState(false);
    const [activeView, setActiveView] = useState<ViewKey>('chat');
    // 多对话历史：会话列表 + 当前会话 id。
    const [conversations, setConversations] = useState<db.Conversation[]>([]);
    const [activeConversationId, setActiveConversationId] = useState('');
    // 配置 JSON 编辑器。
    const [configText, setConfigText] = useState('');
    const [configEditorState, setConfigEditorState] = useState<'idle' | 'loading' | 'saved' | 'error'>('idle');
    const [configEditorMessage, setConfigEditorMessage] = useState('');
    const [isTextInputOpen, setIsTextInputOpen] = useState(false);
    const [continuousVoiceMode, setContinuousVoiceMode] = useState(() => (localStorage.getItem(CONTINUOUS_VOICE_KEY) ?? localStorage.getItem(LEGACY_CONTINUOUS_VOICE_KEY)) === 'true');
    const [conversationMode, setConversationMode] = useState(() => localStorage.getItem(CONVERSATION_MODE_KEY) === 'free' ? 'free' : DEFAULT_CONVERSATION_MODE);
    // 默认强制中文语音：本机人设/回复都是中文，且旧版本把默认值误设为 'ja' 且可能残留在 localStorage，
    // 导致 text_lang=ja 用日语 G2P 读中文而乱读/截断。这里直接以 'zh' 初始化，忽略旧存储。
    const [speechLanguage, setSpeechLanguage] = useState<string>('zh');
    const freeConversationMode = conversationMode === 'free';
    const effectiveContinuousVoiceMode = continuousVoiceMode || freeConversationMode;
    const feedRef = useRef<HTMLDivElement>(null);
    const composerInputRef = useRef<HTMLInputElement>(null);
    const recognitionRef = useRef<any>(null);
    const bargeRecognitionRef = useRef<any>(null);
    const audioRef = useRef<HTMLAudioElement | null>(null);
    const playbackIdRef = useRef(0);
    const lipSyncCleanupRef = useRef<(() => void) | null>(null);
    const isSendingRef = useRef(false);
    const voiceStatusRef = useRef('idle');
    const voiceLoopRef = useRef(false);
    const voiceEmptyTurnsRef = useRef(0);
    const voiceStartInFlightRef = useRef(false);
    const relistenTimerRef = useRef<number | null>(null);
    const voiceGateCancelRef = useRef<(() => void) | null>(null);
    const voiceGateStreamRef = useRef<MediaStream | null>(null);
    const lastUserActivityRef = useRef(Date.now());
    const lastProactiveAtRef = useRef(0);
    const lastProactiveContextHintAtRef = useRef(0);
    const lastFollowUpAtRef = useRef(0);
    const followUpTimerRef = useRef<number | null>(null);
    const proactiveSpeechTimestampsRef = useRef<number[]>([]);
    const proactiveInFlightRef = useRef(false);
    // 流式回复状态：逐句 TTS 队列 + 是否仍在流式回复 + LLM 是否已 done + 是否正在播某句。
    const streamSentenceQueueRef = useRef<string[]>([]);
    const streamReplyActiveRef = useRef(false);
    const streamDoneRef = useRef(false);
    const sentencePlayingRef = useRef(false);
    // 逐句预合成：当前句播放时后台提前合成下一句，消除句间空档。
    const prefetchedSpeechRef = useRef<app.SpeechReply | null>(null);
    const prefetchedTextRef = useRef('');
    const prefetchInFlightRef = useRef(false);
    const prefetchPromiseRef = useRef<Promise<unknown> | null>(null);
    // GPT-SoVITS 流式：预载一个指向流式 URL 的 <audio> 元素，播放时近零停顿。
    const prefetchedAudioRef = useRef<HTMLAudioElement | null>(null);
    // 是否已确认后端支持流式合成（GetSpeechStreamUrl 可用），后续直接走流式。
    const streamSupportedRef = useRef(false);
    const streamCheckInFlightRef = useRef<Promise<boolean> | null>(null);
    // 持有最新 handleChatEvent（避免 EventsOn 一次性注册导致的闭包过期）。
    const chatEventHandlerRef = useRef<(event: ChatStreamEvent) => void>(() => undefined);
    // 持有最新 speakText（供 plugin:progress / 任务完成等一次性订阅调用，避免闭包过期）。
    const speakTextRef = useRef<(text: string) => void>(() => undefined);
    // 任务状态跟踪：用于在任务从「进行中」变为「完成/失败」时播报一句。
    const taskStatusRef = useRef<Record<string, string>>({});
    const tasksInitializedRef = useRef(false);

    const assistantLine = useMemo(() => {
        if (isSending || voiceStatus === 'thinking') {
            return '让我想想...';
        }

        const last = [...messages].reverse().find((message) => message.role === 'assistant');
        return last?.content ?? `你好，我是 ${DESKTOP_PET_NAME}。现在可以通过文字聊天和你互动。`;
    }, [isSending, messages, voiceStatus]);
    const avatarPerformance = useMemo(() => {
        const base = inferAvatarPerformance(assistantLine, emotion, voiceStatus === 'speaking');
        if (!performanceHint) {
            return base;
        }
        return {
            ...base,
            key: `${base.key}:llm`,
            mood: performanceHint.mood ?? base.mood,
            energy: typeof performanceHint.energy === 'number' ? clamp(performanceHint.energy, 0, 1) : base.energy,
            valence: typeof performanceHint.valence === 'number' ? clamp(performanceHint.valence, -1, 1) : base.valence,
            dominance: typeof performanceHint.dominance === 'number' ? clamp(performanceHint.dominance, -1, 1) : base.dominance,
            gesture: performanceHint.gesture ?? base.gesture,
            hand: performanceHint.hand ?? base.hand,
        };
    }, [assistantLine, emotion, voiceStatus, performanceHint]);
    const displayedMessages = useMemo(
        () => visibleChatMessages(messages, MAX_VISIBLE_CHAT_ROUNDS),
        [messages, MAX_VISIBLE_CHAT_ROUNDS],
    );

    useEffect(() => {
        GetState()
            .then((state: app.AppState) => {
                setMessages(state.messages ?? []);
                setEmotion(state.emotion || 'neutral');
                setAgentStatus(state.agentStatus || 'offline');
                setAgentProvider(state.agentProvider || 'unknown');
                setProviderError(state.providerError || '');
            })
            .catch((reason: unknown) => setError(String(reason)));
    }, []);

    useEffect(() => {
        refreshPlugins();
    }, []);

    // 启动加载会话列表（多对话历史）。
    useEffect(() => {
        void loadConversations();
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);

    // 每次渲染后把最新的 handleChatEvent 写入 ref，供一次性注册的 chat:event 订阅调用（避免闭包过期）。
    useEffect(() => {
        chatEventHandlerRef.current = handleChatEvent;
        speakTextRef.current = speakText;
    });

    useEffect(() => {
        if (!canUseWailsRuntime()) {
            return;
        }
        return EventsOn('chat:event', (event: ChatStreamEvent) => {
            chatEventHandlerRef.current(event);
        });
    }, []);

    useEffect(() => {
        refreshTasks();
        if (!canUseWailsRuntime()) {
            return;
        }
        const unsubscribe = EventsOn('agent:task:changed', () => refreshTasks());
        return () => {
            unsubscribe();
        };
    }, []);

    useEffect(() => {
        if (!canUseWailsRuntime()) {
            return;
        }
        // 插件进度事件（如 codex 编码 agent 的中途步骤）→ 桌宠简要说一句，并显示到对话里。
        const unsubscribe = EventsOn('plugin:progress', (payload: any) => {
            if (!payload || payload.event !== 'agent_progress') {
                return;
            }
            const message = payload?.data?.message;
            if (typeof message === 'string' && message.trim()) {
                const text = message.trim().slice(0, 120);
                speakTextRef.current(text);
                appendConversationLine(text);
            }
        });
        return () => {
            unsubscribe();
        };
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
                    applyResponsePerformance(response);
                    speakResponse(response);
                })
                .catch((reason: unknown) => {
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

    function clearFollowUpTimer() {
        if (followUpTimerRef.current) {
            window.clearTimeout(followUpTimerRef.current);
            followUpTimerRef.current = null;
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
        WindowSetBackgroundColour(isPetMode ? 0 : 245, isPetMode ? 0 : 248, isPetMode ? 0 : 251, isPetMode ? 0 : 255);
        WindowSetSize(
            isPetMode ? Math.round(PET_BASE_WIDTH * petScale) : 1200,
            isPetMode ? Math.round(PET_BASE_HEIGHT * petScale) : 800,
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

        const ensureFreeVoiceLoop = () => {
            voiceLoopRef.current = true;
            if (
                voiceStatusRef.current === 'idle' &&
                !isSendingRef.current &&
                !voiceStartInFlightRef.current &&
                !recognitionRef.current &&
                !bargeRecognitionRef.current &&
                !voiceGateStreamRef.current &&
                relistenTimerRef.current === null
            ) {
                void startVoiceInput(true);
            }
        };

        ensureFreeVoiceLoop();
        const interval = window.setInterval(ensureFreeVoiceLoop, 1200);
        return () => window.clearInterval(interval);
    }, [freeConversationMode]);

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

            const isVoiceShortcut = !event.ctrlKey &&
                !event.metaKey &&
                !event.altKey &&
                event.key.toLowerCase() === 'v' &&
                (isPetMode || freeConversationMode) &&
                (freeConversationMode || !isEditableTarget(event.target));
            if (isVoiceShortcut) {
                event.preventDefault();
                if (freeConversationMode) {
                    startManualVoiceInput();
                    return;
                }
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
                    startManualVoiceInput();
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
    }, [effectiveContinuousVoiceMode, freeConversationMode, isPetMode, isSending, voiceStatus]);

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
            clearFollowUpTimer();
            audioRef.current?.pause();
            window.speechSynthesis?.cancel?.();
            document.documentElement.classList.remove('pet-window');
            document.body.classList.remove('pet-window');
        };
    }, []);

    // playQueueHead 播放流式回复队首句：优先用已预载的流式 audio（近零停顿），其次已预合成的 base64；
    // 否则若预取仍在途则等待它完成；最后现场合成（内部先试流式，失败回退 buffered）。
    function playQueueHead(playbackId?: number) {
        const head = streamSentenceQueueRef.current[0];
        if (!head) {
            return;
        }

        // 1) 已预载的流式 audio 且对应队首。
        if (prefetchedAudioRef.current && prefetchedTextRef.current === head) {
            const prefAudio = prefetchedAudioRef.current;
            prefetchedAudioRef.current = null;
            const h = streamSentenceQueueRef.current.shift();
            sentencePlayingRef.current = true;
            stopCurrentAudio();
            const pbId = playbackIdRef.current;
            setVoiceStatus('speaking');
            void playStreamAudio(h!, pbId, prefAudio);
            startPrefetch();
            return;
        }

        // 2) 已预合成的 base64 且对应队首。
        if (prefetchedSpeechRef.current && prefetchedTextRef.current === head) {
            const pref = prefetchedSpeechRef.current;
            const prefText = prefetchedTextRef.current;
            prefetchedSpeechRef.current = null;
            prefetchedTextRef.current = '';
            const h = streamSentenceQueueRef.current.shift();
            sentencePlayingRef.current = true;
            stopCurrentAudio();
            const pbId = playbackIdRef.current;
            setVoiceStatus('speaking');
            void playSpeechReply(pref, prefText, pbId).catch(() => {
                if (pbId === playbackIdRef.current) {
                    finishSpeaking(pbId);
                }
            });
            startPrefetch();
            return;
        }

        // 过期/未就绪的预取：清掉，避免阻塞后续预取。
        if (prefetchedAudioRef.current) {
            discardPrefetchAudio();
            prefetchedTextRef.current = '';
        }
        if (prefetchedSpeechRef.current) {
            prefetchedSpeechRef.current = null;
            prefetchedTextRef.current = '';
        }

        // 3) 预取仍在途 → 等待它完成再播。
        if (prefetchInFlightRef.current && prefetchPromiseRef.current) {
            sentencePlayingRef.current = true;
            void (async () => {
                try {
                    await Promise.race([
                        prefetchPromiseRef.current,
                        new Promise((resolve) => setTimeout(resolve, 3000)),
                    ]);
                } catch {
                    // 合成失败：走下方现场合成兜底。
                }
                if (playbackId !== undefined && playbackId !== playbackIdRef.current) {
                    return;
                }
                const h = streamSentenceQueueRef.current[0];
                if (!h) {
                    finishSpeaking(playbackId);
                    return;
                }
                if (prefetchedAudioRef.current && prefetchedTextRef.current === h) {
                    const pa = prefetchedAudioRef.current;
                    prefetchedAudioRef.current = null;
                    const hh = streamSentenceQueueRef.current.shift();
                    sentencePlayingRef.current = true;
                    stopCurrentAudio();
                    const pb = playbackIdRef.current;
                    setVoiceStatus('speaking');
                    void playStreamAudio(hh!, pb, pa);
                } else if (prefetchedSpeechRef.current && prefetchedTextRef.current === h) {
                    const sp = prefetchedSpeechRef.current;
                    const ptext = prefetchedTextRef.current;
                    prefetchedSpeechRef.current = null;
                    prefetchedTextRef.current = '';
                    const hh = streamSentenceQueueRef.current.shift();
                    sentencePlayingRef.current = true;
                    stopCurrentAudio();
                    const pb = playbackIdRef.current;
                    setVoiceStatus('speaking');
                    void playSpeechReply(sp, ptext, pb).catch(() => {
                        if (pb === playbackIdRef.current) {
                            finishSpeaking(pb);
                        }
                    });
                } else {
                    if (prefetchedAudioRef.current) { discardPrefetchAudio(); prefetchedTextRef.current = ''; }
                    if (prefetchedSpeechRef.current) { prefetchedSpeechRef.current = null; prefetchedTextRef.current = ''; }
                    const hh = streamSentenceQueueRef.current.shift();
                    sentencePlayingRef.current = true;
                    void speakText(hh!);
                }
                startPrefetch();
            })();
            return;
        }

        // 4) 无预取在途 → 现场合成（speakText 内部先试流式，失败回退 buffered）。
        const h = streamSentenceQueueRef.current.shift();
        sentencePlayingRef.current = true;
        void speakText(h!);
        startPrefetch();
    }

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

        // 流式逐句：优先播已预载/预合成的队首句；否则现场合成；LLM done 后清理流式状态。
        if (streamReplyActiveRef.current) {
            sentencePlayingRef.current = false;
            playQueueHead(playbackId);
            if (sentencePlayingRef.current) {
                // playQueueHead 已发起队首播放，交给其 onended 继续 drain。
                return;
            }
            if (streamDoneRef.current) {
                streamReplyActiveRef.current = false;
                streamDoneRef.current = false;
                streamSentenceQueueRef.current = [];
                prefetchedSpeechRef.current = null;
                discardPrefetchAudio();
                prefetchedTextRef.current = '';
                prefetchInFlightRef.current = false;
                prefetchPromiseRef.current = null;
                // 继续走下面的 relisten 逻辑。
            } else {
                return;
            }
        }

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

    function startManualVoiceInput() {
        clearFollowUpTimer();
        if (isSendingRef.current || voiceStatusRef.current === 'thinking') {
            return;
        }

        const restartSoon = () => {
            window.setTimeout(() => void startVoiceInput(false), 80);
        };

        if (voiceStatusRef.current === 'speaking') {
            stopCurrentAudio();
            setVoiceStatus('idle');
            restartSoon();
            return;
        }

        if (
            voiceStatusRef.current === 'listening' ||
            recognitionRef.current ||
            voiceGateStreamRef.current ||
            voiceStartInFlightRef.current
        ) {
            cancelVoiceGate();
            recognitionRef.current?.abort?.();
            recognitionRef.current = null;
            voiceStartInFlightRef.current = false;
            setVoiceStatus('idle');
            restartSoon();
            return;
        }

        void startVoiceInput(false);
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
        recognition.lang = speechRecognitionLang(asrRecognitionLanguage(speechLanguage));
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

    // attachLipSync 用「基于播放时间的简易口型动画」驱动嘴巴，而非把 <audio> 路由进 Web Audio。
    // 原因：createMediaElementSource 会把 <audio> 输出永久路由进 AudioContext，一旦 context 是
    // suspended（首句刚创建、无近期用户手势），整句就静音（听不到）。仿照 Shinsekai：音频走浏览器
    // 默认输出、保证一定有声音；口型用时间动画近似，不再依赖 Web Audio 分析。
    function attachLipSync(audio: HTMLAudioElement, playbackId: number) {
        let frame = 0;
        let active = true;
        const tick = () => {
            if (!active || playbackId !== playbackIdRef.current) {
                return;
            }
            const playing = !audio.paused && !audio.ended && (audio.duration === Infinity || audio.currentTime < audio.duration);
            if (playing) {
                const t = performance.now() / 1000;
                const level = 0.32 + 0.32 * Math.abs(Math.sin(t * 7.5)) * Math.abs(Math.sin(t * 3.2)) + Math.random() * 0.12;
                setMouthLevel(Math.min(1, Math.max(0, level)));
            } else {
                setMouthLevel(0);
            }
            frame = window.requestAnimationFrame(tick);
        };

        lipSyncCleanupRef.current?.();
        lipSyncCleanupRef.current = () => {
            active = false;
            if (frame) {
                window.cancelAnimationFrame(frame);
            }
            setMouthLevel(0);
        };
        tick();
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

    // playSpeechReply 播放一段已合成的音频（base64），不重新合成。
    async function playSpeechReply(speech: app.SpeechReply, content: string, playbackId: number) {
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
        await audio.play().then(() => {
            console.log('[tts] play started, duration=', audio.duration);
        }).catch((err) => {
            console.warn('[tts] play rejected:', err?.name, err?.message);
        });
        addSpeechMetric({phase: 'buffered-play-started', elapsedMs: 0});
        startBargeInListening(playbackId);
        return true;
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
            // 合成偶发失败（如 GPT-SoVITS 瞬时忙）会丢一句；这里重试一次再放弃，避免静默丢句。
            let speech: app.SpeechReply | null = null;
            let lastError: unknown;
            for (let attempt = 0; attempt < 2; attempt++) {
                try {
                    speech = await SynthesizeSpeech(content, speechLanguage);
                    break;
                } catch (reason) {
                    lastError = reason;
                    if (playbackId !== playbackIdRef.current) {
                        return true;
                    }
                    if (attempt === 0) {
                        await new Promise((resolve) => setTimeout(resolve, 300));
                    }
                }
            }
            if (!speech) {
                throw lastError;
            }
            console.log('[tts] synth ok:', JSON.stringify(content), 'audioBase64Len=', speech.audioBase64.length, 'provider=', speech.provider);
            addSpeechMetric({
                phase: 'buffered-audio-ready',
                elapsedMs: Math.round(performance.now() - startedAt),
                detail: speech.provider || speech.contentType || '',
            });
            if (playbackId !== playbackIdRef.current) {
                return true;
            }
            return await playSpeechReply(speech, content, playbackId);
        } catch (reason) {
            if (playbackId !== playbackIdRef.current) {
                return true;
            }
            if (isInterruptedPlaybackError(reason)) {
                finishSpeaking(playbackId);
                return true;
            }
            console.warn('[tts] synth failed:', JSON.stringify(content), reason);
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
            // GPT-SoVITS 流式：优先用流式 URL 渐进播放（减少首字与句间延迟）。
            if (ENABLE_GPT_SOVITS_STREAMING && (await ensureStreamSupported())) {
                const streamed = await playStreamAudio(content, playbackId);
                if (streamed) {
                    return;
                }
                // 流式未开始（如不支持/失败）→ 继续走 buffered。
            }
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

    function scheduleFollowUp(trigger = 'reply-follow-up') {
        if (!FOLLOW_UP_ENABLED || (!isPetMode && !freeConversationMode)) {
            return;
        }
        if (followUpTimerRef.current || proactiveInFlightRef.current) {
            return;
        }
        const now = Date.now();
        if (now - lastFollowUpAtRef.current < FOLLOW_UP_COOLDOWN_MS) {
            return;
        }
        if (Math.random() * 100 >= FOLLOW_UP_CHANCE_PERCENT) {
            return;
        }

        followUpTimerRef.current = window.setTimeout(() => {
            followUpTimerRef.current = null;
            if (
                proactiveInFlightRef.current ||
                isSendingRef.current ||
                recognitionRef.current ||
                bargeRecognitionRef.current ||
                voiceGateStreamRef.current ||
                voiceStatusRef.current !== 'idle' ||
                isWithinQuietHours(PROACTIVE_QUIET_HOURS)
            ) {
                return;
            }

            proactiveInFlightRef.current = true;
            lastFollowUpAtRef.current = Date.now();
            lastProactiveAtRef.current = Date.now();
            GenerateProactiveMessage(trigger)
                .then((response: ChatResponse) => {
                    setMessages(response.messages ?? []);
                    setEmotion(response.emotion || response.reply?.emotion || 'neutral');
                    setAgentStatus(response.agentStatus || 'offline');
                    setAgentProvider(response.agentProvider || 'unknown');
                    setProviderError(response.providerError || '');
                    applyResponsePerformance(response);
                    speakResponse(response);
                })
                .catch((reason: unknown) => {
                    console.warn('Follow-up message failed:', reason);
                })
                .finally(() => {
                    proactiveInFlightRef.current = false;
                });
        }, Math.max(600, FOLLOW_UP_DELAY_MS));
    }

    async function sendContent(rawContent: string) {
        const content = rawContent.trim();
        if (!content || isSendingRef.current) {
            return;
        }

        resetVoiceEmptyTurns();
        clearFollowUpTimer();
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

        // 流式回复：走 StreamChat + chat:event 事件，逐句合成 TTS（与 LLM 生成重叠，降低首句延迟）。
        stopCurrentAudio();
        streamSentenceQueueRef.current = [];
        streamReplyActiveRef.current = true;
        streamDoneRef.current = false;
        sentencePlayingRef.current = false;
        prefetchedSpeechRef.current = null;
        discardPrefetchAudio();
        prefetchedTextRef.current = '';
        prefetchInFlightRef.current = false;
        prefetchPromiseRef.current = null;

        try {
            await StreamChat(new chat.ChatRequest({
                conversation_id: activeConversationId || DESKTOP_COMPANION_CONVERSATION_ID,
                content,
                use_tools: false,
            }));
        } catch (reason) {
            setError(String(reason));
            abortStreamReply();
        }
        // 成功路径不在此收尾——由 chat:event 的 'done' + TTS 队列 drain 触发 completeStreamReply。
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
        setPerformanceHint(null);
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

    function applyResponsePerformance(response: ChatResponse) {
        const mood = String(response.mood || response.reply?.mood || '').trim();
        if (!mood) {
            setPerformanceHint(null);
            return;
        }
        setPerformanceHint({
            mood: mood as AvatarPerformance['mood'],
            energy: Number(response.energy ?? response.reply?.energy ?? 0),
            valence: Number(response.valence ?? response.reply?.valence ?? 0),
            dominance: Number(response.dominance ?? response.reply?.dominance ?? 0),
            gesture: String(response.gesture || response.reply?.gesture || ''),
            hand: (response.hand || response.reply?.hand || 'none') as AvatarPerformance['hand'],
        });
    }

    // ---- 多对话历史 ----
    function mapDbMessages(rows: db.Message[]): app.CompanionMessage[] {
        return (rows ?? []).map((row) => ({
            id: row.id,
            role: row.role,
            content: row.content || '',
            emotion: row.emotion || 'neutral',
            mood: row.mood || 'calm',
            energy: Number(row.energy ?? 0),
            valence: Number(row.valence ?? 0),
            dominance: Number(row.dominance ?? 0),
            gesture: row.gesture || '',
            hand: row.hand || 'none',
            createdAt: row.created_at,
        }));
    }

    async function reloadConversation(id: string) {
        if (!id) {
            return;
        }
        try {
            const rows = await GetMessages(id);
            setMessages(mapDbMessages(rows));
        } catch {
            // 忽略
        }
    }

    async function loadConversations() {
        try {
            const list = await ListConversations();
            setConversations(list ?? []);
            const firstId = activeConversationId || (list && list[0]?.id) || '';
            if (firstId) {
                setActiveConversationId(firstId);
                await reloadConversation(firstId);
            }
        } catch {
            // 忽略
        }
    }

    async function newConversation() {
        try {
            const conv = await CreateConversation('新对话');
            setConversations((prev) => [conv, ...prev]);
            setActiveConversationId(conv.id);
            setMessages([]);
            setPerformanceHint(null);
            setActiveView('chat');
        } catch (reason) {
            setError(String(reason));
        }
    }

    function selectConversation(id: string) {
        if (!id || id === activeConversationId) {
            return;
        }
        setActiveConversationId(id);
        setPerformanceHint(null);
        void reloadConversation(id);
        setActiveView('chat');
    }

    async function refreshConfigEditor() {
        setConfigEditorState('loading');
        try {
            const cfg = await GetConfigJSON();
            setConfigText(JSON.stringify(cfg, null, 2));
            setConfigEditorState('idle');
            setConfigEditorMessage('');
        } catch (reason) {
            setConfigEditorState('error');
            setConfigEditorMessage(String(reason));
        }
    }

    async function saveConfigEditor() {
        setConfigEditorState('loading');
        try {
            const parsed = JSON.parse(configText);
            await SaveConfigJSON(parsed);
            setConfigEditorState('saved');
            setConfigEditorMessage('保存成功，重启后完全生效。');
        } catch (reason) {
            setConfigEditorState('error');
            setConfigEditorMessage(String(reason));
        }
    }

    function completeStreamReply() {
        // LLM 已 done：结束「发送中」状态，刷新消息，安排追问（TTS 队列继续后台播放）。
        streamDoneRef.current = true;
        isSendingRef.current = false;
        setIsSending(false);
        void reloadConversation(activeConversationId || DESKTOP_COMPANION_CONVERSATION_ID);
        void loadConversations();
        void GetState()
            .then((state: app.AppState) => {
                setAgentStatus(state.agentStatus || 'online');
                setAgentProvider(state.agentProvider || 'unknown');
                setProviderError(state.providerError || '');
            })
            .catch(() => undefined);
        scheduleFollowUp('reply-follow-up');
        if (streamSentenceQueueRef.current.length === 0 && !sentencePlayingRef.current) {
            // 没有正在播/排队的句子（例如空回复）→ 直接走 finishSpeaking 清理流式状态。
            finishSpeaking();
        }
    }

    function abortStreamReply() {
        streamReplyActiveRef.current = false;
        streamDoneRef.current = false;
        streamSentenceQueueRef.current = [];
        sentencePlayingRef.current = false;
        prefetchedSpeechRef.current = null;
        discardPrefetchAudio();
        prefetchedTextRef.current = '';
        prefetchInFlightRef.current = false;
        prefetchPromiseRef.current = null;
        isSendingRef.current = false;
        setIsSending(false);
        stopCurrentAudio();
        setVoiceStatus('idle');
    }

    // ensureStreamSupported 探测后端是否支持流式合成（GetSpeechStreamUrl 可用）。结果缓存。
    async function ensureStreamSupported(): Promise<boolean> {
        if (streamSupportedRef.current) {
            return true;
        }
        if (streamCheckInFlightRef.current) {
            return streamCheckInFlightRef.current;
        }
        const check = (async () => {
            try {
                const url = await GetSpeechStreamUrl('测试', speechLanguage);
                streamSupportedRef.current = !!url;
            } catch {
                streamSupportedRef.current = false;
            } finally {
                streamCheckInFlightRef.current = null;
            }
            return streamSupportedRef.current;
        })();
        streamCheckInFlightRef.current = check;
        return check;
    }

    // preloadStreamAudio 构建某句的流式 URL 并预载 <audio>（拉流缓冲，不播放），供播放时近零停顿。
    async function preloadStreamAudio(text: string): Promise<HTMLAudioElement | null> {
        try {
            const url = await GetSpeechStreamUrl(text, speechLanguage);
            if (!url) {
                return null;
            }
            const audio = new Audio(url);
            audio.preload = 'auto';
            try { audio.load(); } catch { /* 忽略 */ }
            return audio;
        } catch {
            return null;
        }
    }

    // discardPrefetchAudio 放弃未被播放的预载流（停止拉流，避免占用 GPT-SoVITS 的资源）。
    function discardPrefetchAudio() {
        const el = prefetchedAudioRef.current;
        if (el) {
            el.onended = null;
            el.onerror = null;
            try {
                el.pause();
                el.removeAttribute('src');
                el.load();
            } catch {
                // 忽略
            }
        }
        prefetchedAudioRef.current = null;
    }

    // playStreamAudio 用流式 URL 渐进播放一句；preloaded 为预载好的元素（近零停顿）。
    // 调用方负责先 stopCurrentAudio() 并取 playbackId。返回 true 表示已开始流式播放；
    // 失败返回 false，调用方回退到 buffered 合成。
    async function playStreamAudio(content: string, playbackId: number, preloaded?: HTMLAudioElement | null): Promise<boolean> {
        let audio = preloaded ?? null;
        if (!audio) {
            const url = await GetSpeechStreamUrl(content, speechLanguage);
            if (!url) {
                return false;
            }
            audio = new Audio(url);
            audio.preload = 'auto';
            try { audio.load(); } catch { /* 忽略 */ }
        }
        if (playbackId !== playbackIdRef.current) {
            return true;
        }
        audioRef.current = audio;
        attachLipSync(audio, playbackId);
        setVoiceStatus('speaking');
        audio.onended = () => finishSpeaking(playbackId);
        audio.onerror = () => {
            if (playbackId !== playbackIdRef.current) {
                return;
            }
            // 流式播放失败 → 回退到 buffered 合成。
            console.warn('GPT-SoVITS streaming 播放失败，回退到 buffered TTS。');
            void speakWithBufferedCloudVoice(content);
        };
        try {
            await audio.play();
            return true;
        } catch (reason) {
            if (playbackId !== playbackIdRef.current) {
                return true;
            }
            if (isInterruptedPlaybackError(reason)) {
                finishSpeaking(playbackId);
                return true;
            }
            console.warn('GPT-SoVITS streaming play() 被拒绝，回退到 buffered TTS:', reason);
            void speakWithBufferedCloudVoice(content);
            return true;
        }
    }

    // startPrefetch 后台预载/预合成下一句（peek 队首，不弹出），避免句间「合成空档」。
    function startPrefetch() {
        if (prefetchInFlightRef.current || prefetchedSpeechRef.current || prefetchedAudioRef.current) {
            return;
        }
        const next = streamSentenceQueueRef.current[0];
        if (!next) {
            return;
        }
        prefetchedTextRef.current = next;
        prefetchInFlightRef.current = true;
        const promise = (async () => {
            // 优先流式：预载一个指向流式 URL 的 <audio>（播放时近零停顿）。
            if (ENABLE_GPT_SOVITS_STREAMING && (await ensureStreamSupported())) {
                const audio = await preloadStreamAudio(next);
                if (audio && prefetchedTextRef.current === next) {
                    prefetchedAudioRef.current = audio;
                }
                return;
            }
            // 回退：buffered 预合成 base64。
            try {
                const speech = await SynthesizeSpeech(next, speechLanguage);
                if (prefetchedTextRef.current === next) {
                    prefetchedSpeechRef.current = speech;
                }
            } catch {
                // 忽略，播放时兜底。
            }
        })();
        prefetchPromiseRef.current = promise;
        void promise.finally(() => {
            prefetchInFlightRef.current = false;
            prefetchPromiseRef.current = null;
        });
    }

    function handleStreamToken(content: string) {
        const text = content.trim();
        if (!text) {
            return;
        }
        console.log('[tts] stream token:', JSON.stringify(text), 'queue=', streamSentenceQueueRef.current.length, 'playing=', sentencePlayingRef.current);
        if (sentencePlayingRef.current) {
            streamSentenceQueueRef.current.push(text);
        } else {
            sentencePlayingRef.current = true;
            void speakText(text);
        }
        startPrefetch();
    }

    function handleStreamDone() {
        if (!streamReplyActiveRef.current) {
            return;
        }
        completeStreamReply();
    }

    function handleChatEvent(event: ChatStreamEvent) {
        switch (event.type) {
            case 'token':
                handleStreamToken(event.content || '');
                break;
            case 'emotion':
                if (event.emotion) {
                    setEmotion(event.emotion);
                }
                if (event.mood) {
                    setPerformanceHint({
                        mood: event.mood as AvatarPerformance['mood'],
                        energy: Number(event.energy ?? 0),
                        valence: Number(event.valence ?? 0),
                        dominance: Number(event.dominance ?? 0),
                        gesture: event.gesture || '',
                        hand: (event.hand || 'none') as AvatarPerformance['hand'],
                    });
                } else {
                    // 快速通道（跳过 Planner）无 LLM mood：清空表演提示，回退前端文本启发式。
                    setPerformanceHint(null);
                }
                break;
            case 'error':
                setError(event.content || '回复失败');
                setProviderError(event.content || '');
                abortStreamReply();
                break;
            case 'done':
                handleStreamDone();
                break;
            default:
                break;
        }
    }

    function refreshPlugins() {
        ListPlugins()
            .then((reply: app.PluginListReply) => setPlugins(reply.plugins ?? []))
            .catch((reason: unknown) => setError(String(reason)));
    }

    async function reloadPlugins() {
        try {
            const reply = await ReloadPlugins();
            setPlugins(reply.plugins ?? []);
        } catch (reason) {
            setError(String(reason));
        }
    }

    async function togglePlugin(plugin: app.PluginInfo) {
        try {
            if (plugin.enabled) {
                await DisablePlugin(plugin.name);
            } else {
                await EnablePlugin(plugin.name);
            }
            refreshPlugins();
        } catch (reason) {
            setError(String(reason));
        }
    }

    async function invokePluginAction(pluginName: string, actionName: string) {
        setPluginResult('');
        let input: Record<string, any> = {};
        const raw = pluginActionInput.trim();
        if (raw) {
            try {
                const parsed = JSON.parse(raw);
                input = (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) ? parsed : {};
            } catch {
                setError('插件参数需为 JSON 对象，例如 {"path":"notes.md"}');
                return;
            }
        }
        try {
            const result = await InvokePluginAction(pluginName, actionName, input);
            setPluginResult(JSON.stringify(result, null, 2));
        } catch (reason) {
            setError(String(reason));
        }
    }

    // 把一条文本作为「助手消息」追加到当前对话（供任务过程/结果在聊天里可见）。
    function appendConversationLine(text: string) {
        if (!text.trim()) return;
        setMessages((prev) => [
            ...prev,
            {
                id: `task-${Date.now()}-${Math.random().toString(36).slice(2, 6)}`,
                role: 'assistant',
                content: text,
                emotion: 'neutral',
                mood: 'calm',
                energy: 0.5,
                valence: 0,
                dominance: 0,
                gesture: 'none',
                hand: 'none',
                createdAt: new Date().toISOString(),
            } as app.CompanionMessage,
        ]);
    }

    function openPluginDetail(plugin: app.PluginInfo) {
        setPluginDetail(plugin);
        setPluginResult('');
        setPluginConfigText('');
        setPluginConfigOpen(false);
    }

    function actionDescription(action: any): string {
        return String(action?.description || action?.name || '');
    }

    function manualActions(plugin: app.PluginInfo): any[] {
        return (plugin.actions ?? []).filter((action) => !REVIEW_ACTION_NAMES.has(String(action.name)));
    }

    function formatChangeKind(change: PluginChange): string {
        const kind = String(change.kind || '').toLowerCase();
        if (change.nestedRepo) return '嵌套仓库';
        if (change.directory) return '目录';
        if (kind === 'added') return '新增';
        if (kind === 'deleted') return '删除';
        if (kind === 'renamed') return '重命名';
        if (kind === 'modified') return '修改';
        return String(change.status || '变更');
    }

    function parseJSONMaybe(raw?: string): any {
        if (!raw) return null;
        try {
            return JSON.parse(raw);
        } catch {
            return null;
        }
    }

    function taskCodeReview(task: db.AgentTask): Record<string, any> | null {
        const result = parseJSONMaybe(task.result_json || '');
        const direct = result?.metadata?.code_review || result?.code_review || result?.metadata?.review;
        if (direct && typeof direct === 'object') return direct as Record<string, any>;

        const summary = String(result?.summary || '');
        const start = summary.indexOf('{');
        const end = summary.lastIndexOf('}');
        if (start >= 0 && end > start) {
            const nested = parseJSONMaybe(summary.slice(start, end + 1));
            if (nested && typeof nested === 'object' && (nested.diff || nested.files || nested.branch)) return nested as Record<string, any>;
        }
        return null;
    }

    function taskReviewFiles(task: db.AgentTask): PluginChange[] {
        const review = taskCodeReview(task);
        const files = review?.files;
        return Array.isArray(files) ? files as PluginChange[] : [];
    }

    function taskReviewDiff(task: db.AgentTask): string {
        const review = taskCodeReview(task);
        return String(review?.diff || '');
    }

    async function openTaskReviewInVSCode(task: db.AgentTask) {
        const review = taskCodeReview(task);
        if (!review) return;
        setPluginResult('');
        try {
            const payload: Record<string, any> = {...review, limit: 12};
            const result = await InvokePluginAction(String(review.plugin || 'code-assistant'), 'open_changes', payload);
            const parsed = (result && typeof result === 'object') ? result as Record<string, any> : {};
            setPluginResult(parsed.note ? String(parsed.note) : '已在 VS Code 打开任务变更。');
        } catch (reason) {
            setError(String(reason));
        }
    }

    async function reviewTaskDecision(task: db.AgentTask, action: 'accept_changes' | 'reject_changes') {
        const review = taskCodeReview(task);
        const pluginName = String(review?.plugin || 'code-assistant');
        try {
            const result = await InvokePluginAction(pluginName, action, review || {});
            const text = action === 'accept_changes' ? '已接受任务生成的代码改动。' : '已拒绝任务生成的代码改动。';
            setTaskReviewDiffOpen((prev) => ({...prev, [task.id]: false}));
            appendConversationLine(text);
            void speakText(text);
            setPluginResult(JSON.stringify(result, null, 2));
            refreshTasks();
        } catch (reason) {
            setError(String(reason));
        }
    }

    async function showPluginConfig(pluginName: string) {
        setPluginResult('');
        try {
            const config = await GetPluginConfig(pluginName);
            setPluginConfigText(JSON.stringify(config ?? {}, null, 2));
            setPluginConfigOpen(true);
        } catch (reason) {
            setError(String(reason));
        }
    }

    async function savePluginConfig(pluginName: string) {
        let input: Record<string, any> = {};
        const raw = pluginConfigText.trim();
        if (raw) {
            try {
                const parsed = JSON.parse(raw);
                input = (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) ? parsed : {};
            } catch {
                setError('插件配置需为 JSON 对象');
                return;
            }
        }
        try {
            await SetPluginConfig(pluginName, input);
            setPluginResult('配置已保存');
            setPluginConfigOpen(true);
            refreshPlugins();
        } catch (reason) {
            setError(String(reason));
        }
    }

    function refreshTasks() {
        ListAgentTasks('', 50)
            .then((items) => {
                const list = items ?? [];
                if (tasksInitializedRef.current) {
                    for (const task of list) {
                        const prev = taskStatusRef.current[task.id];
                        if ((task.status === 'completed' || task.status === 'failed') && prev && prev !== 'completed' && prev !== 'failed') {
                            const text = task.status === 'completed'
                                ? `任务完成了：${task.title || task.goal || ''}`
                                : `任务失败了：${task.error || task.title || task.goal || ''}`;
                            if (text.trim()) {
                                const line = text.trim().slice(0, 120);
                                speakTextRef.current(line);
                                appendConversationLine(line);
                            }
                        }
                    }
                } else {
                    tasksInitializedRef.current = true;
                }
                for (const task of list) {
                    taskStatusRef.current[task.id] = task.status;
                }
                setTasks(list);
            })
            .catch((reason: unknown) => setError(String(reason)));
    }

    async function refreshLogs() {
        setLogLoading(true);
        try {
            const level = logFilter || logLevel;
            const items = await GetLogs(level, 800);
            setLogs(items ?? []);
        } catch (reason) {
            setError(String(reason));
        } finally {
            setLogLoading(false);
        }
    }

    async function changeLogLevel(level: string) {
        setLogLevel(level);
        try {
            await SetLogLevel(level);
        } catch (reason) {
            setError(String(reason));
        }
        await refreshLogs();
    }

    useEffect(() => {
        void refreshLogs();
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [logFilter]);

    async function cancelTask(taskId: string) {
        try {
            await CancelAgentTask(taskId);
            refreshTasks();
        } catch (reason) {
            setError(String(reason));
        }
    }

    async function approveTask(taskId: string, approve: boolean) {
        try {
            await SendAgentTaskControl(taskId, approve ? 'approve' : 'reject', {});
            refreshTasks();
        } catch (reason) {
            setError(String(reason));
        }
    }

    async function answerTask(taskId: string) {
        const answer = String(taskAnswer[taskId] || '').trim();
        if (!answer) {
            return;
        }
        try {
            await AnswerAgentTaskQuestion(taskId, answer);
            setTaskAnswer({});
            refreshTasks();
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
            applyResponsePerformance(response);
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
                // funasr-server（soundfile）解不了 webm/opus，转成 16k 单声道 WAV 再发。
                const wav = await webmToWav16kMono(blob);
                const reply = await TranscribeAudio(wav.base64, wav.contentType, asrProviderLanguage(speechLanguage)) as ASRReply;
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
        addSpeechMetric({phase: 'asr-record-start', elapsedMs: 0, detail: `${ASR_PROVIDER}; language=${asrProviderLanguage(speechLanguage) || 'auto'}`});
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
        if (voiceStartInFlightRef.current) {
            return;
        }
        voiceStartInFlightRef.current = true;
        try {
            if (ASR_PROVIDER !== 'browser' && ASR_PROVIDER !== 'webspeech') {
                await startModelASRVoiceInput(fromLoop);
                return;
            }
            await startBrowserVoiceInput(fromLoop);
        } finally {
            voiceStartInFlightRef.current = false;
        }
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
        recognition.lang = speechRecognitionLang(asrRecognitionLanguage(speechLanguage));
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
        <ChatComposer
            isTextInputOpen={isTextInputOpen}
            draft={draft}
            isSending={isSending}
            voiceStatus={voiceStatus}
            freeConversationMode={freeConversationMode}
            effectiveContinuousVoiceMode={effectiveContinuousVoiceMode}
            inputRef={composerInputRef}
            onDraftChange={setDraft}
            onToggleText={() => setIsTextInputOpen((value) => !value)}
            onVoice={startManualVoiceInput}
            onSubmit={sendMessage}
        />
    );

    return (
        <main className={`app-shell ${isPetMode ? 'pet-mode' : 'web'}`}>
            {isPetMode ? (
                <PetModeView
                    emotion={emotion}
                    voiceStatus={voiceStatus}
                    mouthLevel={mouthLevel}
                    petScale={petScale}
                    performance={avatarPerformance}
                    assistantLine={assistantLine}
                    controlsOpen={isPetControlsOpen}
                    composer={composer}
                    onWheel={resizePetWithWheel}
                    onExitPetMode={() => setIsPetMode(false)}
                />
            ) : (
            <div className="web-shell">
                <WebSidebar
                    conversations={conversations}
                    activeConversationId={activeConversationId}
                    activeView={activeView}
                    agentStatus={agentStatus}
                    agentProvider={agentProvider}
                    onNewConversation={() => void newConversation()}
                    onSelectConversation={selectConversation}
                    onSelectView={setActiveView}
                    onPetMode={() => setIsPetMode(true)}
                />

                <section className="web-content" aria-label="列表内容">
                    {activeView === 'chat' && (
                        <ChatView
                            messages={displayedMessages}
                            feedRef={feedRef}
                            voiceStatus={voiceStatus}
                            agentStatus={agentStatus}
                            agentProvider={agentProvider}
                            providerError={providerError}
                            error={error}
                            voiceError={voiceError}
                            composer={composer}
                        />
                    )}

                    {activeView === 'skins' && (
                        <SkinsView
                            emotion={emotion}
                            voiceStatus={voiceStatus}
                            mouthLevel={mouthLevel}
                            petScale={petScale}
                            performance={avatarPerformance}
                        />
                    )}

                    {activeView === 'model' && (
                        <ModelView agentProvider={agentProvider} agentStatus={agentStatus} providerError={providerError} />
                    )}

                    {activeView === 'plugins' && (
                        <section className="web-view">
                            <h2 className="web-view-title">插件管理</h2>
                            {pluginDetail ? (
                                <div className="plugin-detail">
                                    <button type="button" className="ghost-button back-button" onClick={() => setPluginDetail(null)}>← 返回插件列表</button>
                                    <div className="plugin-detail-card">
                                        <div className="plugin-detail-hero">
                                            <div className="plugin-icon">{(pluginDetail.displayName || pluginDetail.name || 'P').slice(0, 1)}</div>
                                            <div className="plugin-card-main">
                                                <b>{pluginDetail.displayName || pluginDetail.name}</b>
                                                <small>v{pluginDetail.version} · {pluginDetail.author || '内置'} · {pluginDetail.enabled ? '已启用' : '未启用'}</small>
                                                <p>{pluginDetail.description}</p>
                                            </div>
                                            <button type="button" className={pluginDetail.enabled ? 'danger-soft-button' : 'primary-soft-button'} onClick={() => void togglePlugin(pluginDetail)}>{pluginDetail.enabled ? '停用' : '启用'}</button>
                                        </div>

                                        <div className="plugin-meta-grid">
                                            <div><span>入口</span><b>{pluginDetail.entry || '内置'}</b></div>
                                            <div><span>动作</span><b>{(pluginDetail.actions ?? []).length}</b></div>
                                            <div><span>模型工具</span><b>{(pluginDetail.tools ?? []).length}</b></div>
                                            <div><span>状态</span><b>{pluginDetail.enabled ? '运行中' : '已停用'}</b></div>
                                        </div>

                                        {(pluginDetail.permissions ?? []).length > 0 && (
                                            <div className="plugin-permission-list">
                                                {(pluginDetail.permissions ?? []).map((permission) => <span key={permission}>{permission}</span>)}
                                            </div>
                                        )}

                                        {(pluginDetail.tools ?? []).length > 0 && (
                                            <section className="plugin-section">
                                                <div className="plugin-section-head">
                                                    <h3>模型工具</h3>
                                                    <span>由对话模型按需调用</span>
                                                </div>
                                                <div className="plugin-tool-list">
                                                    {(pluginDetail.tools ?? []).map((tool) => (
                                                        <div className="plugin-tool-item" key={String(tool.name)}>
                                                            <b>{String(tool.name)}</b>
                                                            <span>{String(tool.description ?? '已注册给 Planner / Worker')}</span>
                                                        </div>
                                                    ))}
                                                </div>
                                            </section>
                                        )}

                                        {manualActions(pluginDetail).length > 0 && (
                                            <section className="plugin-section">
                                                <div className="plugin-section-head">
                                                    <h3>手动动作</h3>
                                                    <span>适合从插件页直接执行</span>
                                                </div>
                                                <div className="manual-action-grid">
                                                    {manualActions(pluginDetail).map((action) => (
                                                        <button type="button" key={String(action.name)} onClick={() => void invokePluginAction(pluginDetail.name, String(action.name))}>
                                                            <b>{String(action.name)}</b>
                                                            <span>{actionDescription(action)}</span>
                                                        </button>
                                                    ))}
                                                </div>
                                            </section>
                                        )}

                                        <section className="plugin-section">
                                            <div className="plugin-section-head">
                                                <h3>配置</h3>
                                                <span>保存后重新拉起 sidecar 生效</span>
                                            </div>
                                            <div className="review-toolbar">
                                                <button type="button" className="ghost-button" onClick={() => void showPluginConfig(pluginDetail.name)}>{pluginConfigOpen ? '重新加载配置' : '编辑配置'}</button>
                                                {pluginConfigOpen && <button type="button" className="primary-soft-button" onClick={() => void savePluginConfig(pluginDetail.name)}>保存配置</button>}
                                            </div>
                                            {pluginConfigOpen && (
                                                <textarea
                                                    className="plugin-config-textarea resize-none"
                                                    value={pluginConfigText}
                                                    onChange={(event) => setPluginConfigText(event.target.value)}
                                                    spellCheck={false}
                                                />
                                            )}
                                        </section>

                                        {pluginResult && <div className="plugin-result-box">{pluginResult}</div>}
                                    </div>
                                </div>
                            ) : (
                                <>
                                    <div className="plugin-panel-header">
                                        <strong>插件 {plugins.length > 0 ? `(${plugins.length})` : ''}</strong>
                                        <div className="plugin-panel-actions">
                                            <button type="button" className="ghost-button" onClick={reloadPlugins}>重新加载</button>
                                            <button type="button" className="ghost-button" onClick={refreshPlugins}>刷新</button>
                                        </div>
                                    </div>
                                    {plugins.length === 0 && (
                                        <div className="empty-state"><span>暂无插件。插件系统内核已就绪，可扩展电脑工具 / 游戏 / 任务等能力。</span></div>
                                    )}
                                    <div className="plugin-grid">
                                        {plugins.map((plugin) => (
                                            <div className={`plugin-card clickable${plugin.enabled ? '' : ' disabled'}`} key={plugin.name} onClick={() => openPluginDetail(plugin)}>
                                                <div className="plugin-card-main">
                                                    <div className="plugin-card-title">
                                                        <span className="plugin-icon small">{(plugin.displayName || plugin.name || 'P').slice(0, 1)}</span>
                                                        <div>
                                                            <b>{plugin.displayName || plugin.name}</b>
                                                            <small>v{plugin.version} · {plugin.author || '内置'}</small>
                                                        </div>
                                                    </div>
                                                    <p>{plugin.description}</p>
                                                    <div className="plugin-card-stats">
                                                        <span>{(plugin.actions ?? []).length} 动作</span>
                                                        <span>{(plugin.tools ?? []).length} 工具</span>
                                                        <span>{plugin.enabled ? '已启用' : '已停用'}</span>
                                                    </div>
                                                </div>
                                                <div className="plugin-card-actions">
                                                    <button type="button" onClick={(e) => { e.stopPropagation(); void togglePlugin(plugin); }}>{plugin.enabled ? '停用' : '启用'}</button>
                                                    <button type="button" onClick={(e) => { e.stopPropagation(); openPluginDetail(plugin); }}>详情</button>
                                                </div>
                                            </div>
                                        ))}
                                    </div>
                                    {pluginResult && <pre className="plugin-result">{pluginResult}</pre>}
                                </>
                            )}
                        </section>
                    )}

                    {activeView === 'tasks' && (
                        <section className="web-view">
                            <h2 className="web-view-title">后台任务</h2>
                            <div className="plugin-panel-header">
                                <strong>后台任务</strong>
                                <button type="button" className="ghost-button" onClick={refreshTasks}>刷新</button>
                            </div>
                            {tasks.length === 0 && (
                                <div className="empty-state"><span>暂无后台任务。在聊天里让我「写代码 / 做 PPT / 创建文件」会在这里显示。</span></div>
                            )}
                            {tasks.map((task) => {
                                const codeReview = taskCodeReview(task);
                                const files = taskReviewFiles(task);
                                const diff = taskReviewDiff(task);
                                const diffOpen = Boolean(taskReviewDiffOpen[task.id]);
                                return (
                                    <div className="task-card" key={task.id}>
                                        <div className="task-card-top">
                                            <div className="plugin-card-main">
                                                <b>{task.title || task.goal}</b>
                                                <small>{taskStatusLabel[task.status] ?? task.status}</small>
                                                {task.error && <p className="task-error">{task.error}</p>}
                                            </div>
                                            <div className="plugin-card-actions">
                                                {task.status === 'waiting_for_input' && (
                                                    <>
                                                        <input value={taskAnswer[task.id] || ''} onChange={(event) => setTaskAnswer((prev) => ({...prev, [task.id]: event.target.value}))} placeholder="补充信息..." />
                                                        <button type="button" onClick={() => void answerTask(task.id)}>回答</button>
                                                    </>
                                                )}
                                                {task.status === 'waiting_for_approval' && (
                                                    <>
                                                        <button type="button" onClick={() => void approveTask(task.id, true)}>批准</button>
                                                        <button type="button" onClick={() => void approveTask(task.id, false)}>拒绝</button>
                                                    </>
                                                )}
                                                {['queued', 'running', 'waiting_for_input', 'waiting_for_approval'].includes(task.status) && (
                                                    <button type="button" onClick={() => void cancelTask(task.id)}>取消</button>
                                                )}
                                            </div>
                                        </div>

                                        {codeReview && (
                                            <section className="task-review-panel">
                                                <div className="plugin-section-head">
                                                    <h3>代码变更</h3>
                                                    <span>{String(codeReview.branch || '待评审')} · {files.length} 个条目</span>
                                                </div>
                                                {Array.isArray(codeReview.absorbedNestedRepos) && codeReview.absorbedNestedRepos.length > 0 && (
                                                    <p className="plugin-hint">检测到新建子项目自带 git 仓库，已临时按普通目录纳入本轮评审，所以 VS Code 能显示内部文件增删。</p>
                                                )}
                                                {files.length > 0 && (
                                                    <div className="change-list compact">
                                                        {files.map((change, index) => (
                                                            <div className="change-row" key={`${task.id}-${change.path}-${index}`}>
                                                                <span className={`change-kind kind-${String(change.kind || 'changed').toLowerCase()}`}>{formatChangeKind(change)}</span>
                                                                <span className="change-path">{change.path}</span>
                                                                {change.oldPath && <span className="change-old">← {change.oldPath}</span>}
                                                                {change.nestedRepo && <span className="change-note">{change.nestedCount || 0} 个内部变更</span>}
                                                            </div>
                                                        ))}
                                                    </div>
                                                )}
                                                <div className="review-toolbar task-review-actions">
                                                    <button type="button" className="primary-soft-button" onClick={() => void openTaskReviewInVSCode(task)}>在 VS Code 查看</button>
                                                    {diff && <button type="button" className="ghost-button" onClick={() => setTaskReviewDiffOpen((prev) => ({...prev, [task.id]: !diffOpen}))}>{diffOpen ? '收起补丁' : '查看补丁'}</button>}
                                                    <button type="button" className="rev-accept" onClick={() => void reviewTaskDecision(task, 'accept_changes')}>接受</button>
                                                    <button type="button" className="rev-reject" onClick={() => void reviewTaskDecision(task, 'reject_changes')}>拒绝</button>
                                                </div>
                                                {diff && diffOpen && (
                                                    <div className="review-diff">
                                                        {diff.split('\n').map((line, i) => (
                                                            <div key={i} className={`diff-line ${line.startsWith('+') && !line.startsWith('+++') ? 'add' : line.startsWith('-') && !line.startsWith('---') ? 'del' : (line.startsWith('@@') || line.startsWith('diff ') || line.startsWith('index ')) ? 'meta' : ''}`}>{line || ' '}</div>
                                                        ))}
                                                    </div>
                                                )}
                                            </section>
                                        )}
                                    </div>
                                );
                            })}
                            {pluginResult && <div className="plugin-result-box">{pluginResult}</div>}
                        </section>
                    )}

                    {activeView === 'logs' && (
                        <section className="web-view">
                            <div className="log-header">
                                <h2 className="web-view-title">桌宠日志</h2>
                                <div className="log-toolbar">
                                    <select value={logFilter} onChange={(event) => setLogFilter(event.target.value)} aria-label="日志级别">
                                        <option value="">全部（当前等级 {logLevel}）</option>
                                        <option value="DEBUG">DEBUG</option>
                                        <option value="INFO">INFO</option>
                                        <option value="WARN">WARN</option>
                                        <option value="ERROR">ERROR</option>
                                    </select>
                                    <button type="button" className="ghost-button" onClick={() => void refreshLogs()}>刷新</button>
                                    <button type="button" className="ghost-button" onClick={() => setLogs([])}>清屏</button>
                                </div>
                            </div>
                            <div className="info-card">
                                <b>采集等级</b>
                                <span>{logLevel}</span>
                                <div className="log-levels">
                                    {['DEBUG', 'INFO', 'WARN', 'ERROR'].map((lv) => (
                                        <button key={lv} type="button" className={`ghost-button${logLevel === lv ? ' active' : ''}`} onClick={() => void changeLogLevel(lv)}>{lv}</button>
                                    ))}
                                </div>
                                <span className="log-hint">等级保存在配置文件 config.json 的 "log_level"（默认 DEBUG）。</span>
                            </div>
                            {logLoading && <div className="empty-state">加载中…</div>}
                            {logs.length === 0 && !logLoading && <div className="empty-state"><span>暂无日志。等级设为 DEBUG 可看到更详细输出（含插件调用）。</span></div>}
                            <div className="log-view">
                                {logs.map((entry, index) => (
                                    <div className={`log-entry log-${(entry.level || 'info').toLowerCase()}`} key={`${index}-${entry.time}`}>
                                        <span className="log-time">{entry.time ? new Date(entry.time).toLocaleTimeString() : ''}</span>
                                        <span className="log-level">{entry.level}</span>
                                        <span className="log-message">{entry.message}{entry.source ? `\n${entry.source}` : ''}</span>
                                        {entry.attrs && Object.keys(entry.attrs).length > 0 && (
                                            <pre className="log-attrs">{JSON.stringify(entry.attrs)}</pre>
                                        )}
                                    </div>
                                ))}
                            </div>
                        </section>
                    )}

                    {activeView === 'settings' && (
                        <section className="web-view">
                            <h2 className="web-view-title">设置</h2>
                            <div className="info-card"><b>语音语言</b>
                                <button type="button" className="ghost-button" onClick={() => setSpeechLanguage((value) => value === 'zh' ? 'ja' : 'zh')} aria-pressed={speechLanguage === 'ja'}>
                                    {speechLanguage === 'zh' ? '中文' : '日语'}
                                </button>
                            </div>
                            <div className="info-card"><b>自由聊天</b>
                                <button type="button" className="ghost-button" onClick={() => setConversationMode((value) => value === 'free' ? 'manual' : 'free')} aria-pressed={freeConversationMode}>
                                    {freeConversationMode ? '开' : '关'}
                                </button>
                            </div>
                            <div className="info-card"><b>持续对话</b>
                                <button type="button" className="ghost-button" onClick={() => setContinuousVoiceMode((value) => !value)} aria-pressed={effectiveContinuousVoiceMode} disabled={freeConversationMode}>
                                    {effectiveContinuousVoiceMode ? '开' : '关'}
                                </button>
                            </div>
                            <h3 className="web-view-sub">配置文件（可编辑任意配置项）</h3>
                            <div className="config-editor">
                                <textarea
                                    className="config-editor-textarea resize-none"
                                    value={configText}
                                    onChange={(event) => setConfigText(event.target.value)}
                                    spellCheck={false}
                                    placeholder="点击「加载配置」读取当前 config.json，修改后点「保存配置」写回。"
                                />
                                <div className="config-editor-bar">
                                    <button type="button" className="ghost-button" onClick={() => void refreshConfigEditor()}>加载配置</button>
                                    <button type="button" className="ghost-button" onClick={() => void saveConfigEditor()} disabled={configEditorState === 'loading'}>保存配置</button>
                                    <span className={`config-editor-status ${configEditorState}`}>{configEditorState === 'loading' ? '处理中…' : configEditorState === 'saved' ? '已保存' : configEditorState === 'error' ? '出错' : ''}</span>
                                </div>
                                {configEditorMessage && <div className="error">{configEditorMessage}</div>}
                            </div>
                            <h3 className="web-view-sub">快捷键</h3>
                            <div className="info-card"><b>语音输入</b><span>V</span></div>
                            <h3 className="web-view-sub">关于</h3>
                            <div className="info-card"><b>版本</b><span>Yuyu-Mind2 · v0.1</span></div>
                        </section>
                    )}
                </section>
            </div>
            )}
        </main>
    );
}

export default App;
