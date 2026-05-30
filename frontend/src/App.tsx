import {FormEvent, WheelEvent, useEffect, useMemo, useRef, useState} from 'react';
import './App.css';
import {ClearChat, GetState, SendMessage, SynthesizeSpeech} from '../wailsjs/go/main/App';
import {Quit, WindowCenter, WindowMinimise, WindowSetAlwaysOnTop, WindowSetBackgroundColour, WindowSetSize} from '../wailsjs/runtime/runtime';
import {Live2DStage} from './components/Live2DStage';

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

const emotionLabel: Record<string, string> = {
    neutral: '待机',
    happy: '开心',
    focused: '专注',
    thinking: '思考',
    sad: '低落',
    surprised: '惊讶',
};

const PET_MODE_KEY = 'mochi.petMode';
const PET_SCALE_KEY = 'mochi.petScale';
const PET_CONTROLS_SHORTCUT = 'Ctrl + Shift + M';
const PET_BASE_WIDTH = 380;
const PET_BASE_HEIGHT = 560;
const PET_MIN_SCALE = 0.6;
const PET_MAX_SCALE = 1.8;
const PET_SCALE_STEP = 0.08;
const DESKTOP_PET_NAME = (import.meta.env.VITE_DESKTOP_PET_NAME as string | undefined)?.trim() || 'Mochi';

function canUseWailsRuntime() {
    return Boolean((window as any).runtime);
}

function clamp(value: number, min: number, max: number) {
    return Math.min(max, Math.max(min, value));
}

function readStoredPetScale() {
    const value = Number(localStorage.getItem(PET_SCALE_KEY));
    if (!Number.isFinite(value)) {
        return 1;
    }
    return clamp(value, PET_MIN_SCALE, PET_MAX_SCALE);
}

function isEditableTarget(target: EventTarget | null) {
    const element = target as HTMLElement | null;
    return Boolean(element?.closest('input, textarea, [contenteditable="true"]'));
}

function App() {
    const [messages, setMessages] = useState<Message[]>([]);
    const [draft, setDraft] = useState('');
    const [emotion, setEmotion] = useState('neutral');
    const [agentStatus, setAgentStatus] = useState('offline');
    const [agentProvider, setAgentProvider] = useState('unknown');
    const [providerError, setProviderError] = useState('');
    const [isSending, setIsSending] = useState(false);
    const [error, setError] = useState('');
    const [voiceStatus, setVoiceStatus] = useState('idle');
    const [voiceError, setVoiceError] = useState('');
    const [isPetMode, setIsPetMode] = useState(() => localStorage.getItem(PET_MODE_KEY) === 'true');
    const [petScale, setPetScale] = useState(readStoredPetScale);
    const [isPetControlsOpen, setIsPetControlsOpen] = useState(false);
    const [isTextInputOpen, setIsTextInputOpen] = useState(false);
    const feedRef = useRef<HTMLDivElement>(null);
    const composerInputRef = useRef<HTMLInputElement>(null);
    const recognitionRef = useRef<any>(null);
    const audioRef = useRef<HTMLAudioElement | null>(null);

    const assistantLine = useMemo(() => {
        if (isSending || voiceStatus === 'thinking') {
            return '让我想想...';
        }

        const last = [...messages].reverse().find((message) => message.role === 'assistant');
        return last?.content ?? `你好，我是 ${DESKTOP_PET_NAME}。现在可以通过文字聊天和你互动。`;
    }, [isSending, messages, voiceStatus]);

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
        feedRef.current?.scrollTo({
            top: feedRef.current.scrollHeight,
            behavior: 'smooth',
        });
    }, [messages]);

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
        function onKeyDown(event: KeyboardEvent) {
            if (event.key === 'Escape') {
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
                if (!isSending && voiceStatus !== 'thinking' && voiceStatus !== 'speaking') {
                    startVoiceInput();
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
    }, [isPetMode, isSending, voiceStatus]);

    useEffect(() => {
        if (isTextInputOpen) {
            composerInputRef.current?.focus();
        }
    }, [isTextInputOpen]);

    useEffect(() => {
        return () => {
            recognitionRef.current?.abort?.();
            audioRef.current?.pause();
            window.speechSynthesis?.cancel?.();
            document.documentElement.classList.remove('pet-window');
            document.body.classList.remove('pet-window');
        };
    }, []);

    function speakWithSystemVoice(text: string) {
        if (!('speechSynthesis' in window) || !text.trim()) {
            setVoiceStatus('idle');
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

        utterance.onstart = () => setVoiceStatus('speaking');
        utterance.onend = () => setVoiceStatus('idle');
        utterance.onerror = () => setVoiceStatus('idle');
        window.speechSynthesis.speak(utterance);
    }

    async function speakText(text: string) {
        const content = text.trim();
        if (!content) {
            setVoiceStatus('idle');
            return;
        }

        try {
            setVoiceStatus('speaking');
            audioRef.current?.pause();
            window.speechSynthesis?.cancel?.();

            const speech = await SynthesizeSpeech(content);
            const audio = new Audio(`data:${speech.contentType || 'audio/mpeg'};base64,${speech.audioBase64}`);
            audioRef.current = audio;
            audio.onended = () => setVoiceStatus('idle');
            audio.onerror = () => {
                setVoiceError('Fish Audio 音频播放失败，已切换到系统朗读。');
                speakWithSystemVoice(content);
            };
            await audio.play();
        } catch (reason) {
            setVoiceError(`Fish Audio TTS 失败：${String(reason)}`);
            speakWithSystemVoice(content);
        }
    }

    async function sendContent(rawContent: string) {
        const content = rawContent.trim();
        if (!content || isSending) {
            return;
        }

        setDraft('');
        setIsTextInputOpen(false);
        setIsPetControlsOpen(false);
        setIsSending(true);
        setError('');
        setVoiceError('');
        setVoiceStatus('thinking');
        setEmotion('focused');

        try {
            const response = await SendMessage(content) as ChatResponse;
            setMessages(response.messages ?? []);
            setEmotion(response.emotion || response.reply?.emotion || 'neutral');
            setAgentStatus(response.agentStatus || 'offline');
            setAgentProvider(response.agentProvider || 'unknown');
            setProviderError(response.providerError || '');
            void speakText(response.speechText || response.reply?.content || '');
        } catch (reason) {
            setError(String(reason));
            setVoiceStatus('idle');
        } finally {
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

    function startVoiceInput() {
        const SpeechRecognition = (window as any).SpeechRecognition || (window as any).webkitSpeechRecognition;
        if (!SpeechRecognition) {
            setVoiceError('当前 WebView 不支持浏览器语音识别，可以先继续使用文字输入。');
            setIsTextInputOpen(true);
            return;
        }

        if (voiceStatus === 'listening') {
            recognitionRef.current?.stop?.();
            return;
        }

        window.speechSynthesis?.cancel?.();
        const recognition = new SpeechRecognition();
        recognitionRef.current = recognition;
        recognition.lang = 'zh-CN';
        recognition.continuous = false;
        recognition.interimResults = true;

        let finalTranscript = '';
        let latestTranscript = '';
        setVoiceError('');
        setVoiceStatus('listening');
        setEmotion('thinking');

        recognition.onresult = (event: any) => {
            let interimTranscript = '';
            for (let index = event.resultIndex; index < event.results.length; index += 1) {
                const transcript = event.results[index][0]?.transcript ?? '';
                if (event.results[index].isFinal) {
                    finalTranscript += transcript;
                } else {
                    interimTranscript += transcript;
                }
            }
            latestTranscript = (finalTranscript || interimTranscript).trim();
            setDraft(latestTranscript);
            if (latestTranscript) {
                setIsTextInputOpen(true);
            }
        };

        recognition.onerror = (event: any) => {
            setVoiceStatus('idle');
            setVoiceError(event.error ? `语音识别失败：${event.error}` : '语音识别失败。');
        };

        recognition.onend = () => {
            const content = (finalTranscript || latestTranscript).trim();
            recognitionRef.current = null;
            if (!content) {
                setVoiceStatus('idle');
                setEmotion('neutral');
                return;
            }
            void sendContent(content);
        };

        recognition.start();
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
                onClick={startVoiceInput}
                disabled={isSending || voiceStatus === 'thinking' || voiceStatus === 'speaking'}
            >
                {voiceStatus === 'listening' ? '停止' : '语音'}
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
            <section className="stage" aria-label="Mochi Live2D 舞台" onWheel={resizePetWithWheel}>
                {!isPetMode && (
                    <div className="status-bar">
                        <span>{DESKTOP_PET_NAME} AI</span>
                        <span>{emotionLabel[emotion] ?? emotion} · Agent {agentStatus} · {agentProvider}</span>
                    </div>
                )}

                <Live2DStage emotion={emotion} isSpeaking={voiceStatus === 'speaking'} petScale={petScale}/>

                {isPetMode && voiceStatus === 'speaking' && assistantLine.trim() && (
                    <div className="pet-subtitle" aria-live="polite">
                        <span>{DESKTOP_PET_NAME}</span>
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
                                onClick={clearChat}
                                disabled={isSending || voiceStatus === 'speaking' || messages.length === 0}
                            >
                                清空聊天
                            </button>
                            <span className={`pill agent-${agentStatus}`}>Agent {agentStatus} · {agentProvider}</span>
                        </div>
                    </header>

                    <div className="message-feed" ref={feedRef}>
                        {messages.length === 0 && (
                            <div className="empty-state">
                                <strong>第一步已经就位。</strong>
                                <span>试试输入“你好”或“记住我喜欢中文简洁回复”。</span>
                            </div>
                        )}

                        {messages.map((message) => (
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
