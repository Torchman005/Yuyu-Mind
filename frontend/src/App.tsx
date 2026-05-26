import {FormEvent, useEffect, useMemo, useRef, useState} from 'react';
import './App.css';
import {GetState, SendMessage} from '../wailsjs/go/main/App';

type Message = {
    id: number;
    role: string;
    content: string;
    emotion: string;
    createdAt: string;
};

const emotionLabel: Record<string, string> = {
    neutral: '待机',
    happy: '开心',
    focused: '专注',
    thinking: '思考',
};

function App() {
    const [messages, setMessages] = useState<Message[]>([]);
    const [draft, setDraft] = useState('');
    const [emotion, setEmotion] = useState('neutral');
    const [agentStatus, setAgentStatus] = useState('offline');
    const [isSending, setIsSending] = useState(false);
    const [error, setError] = useState('');
    const feedRef = useRef<HTMLDivElement>(null);

    const assistantLine = useMemo(() => {
        const last = [...messages].reverse().find((message) => message.role === 'assistant');
        return last?.content ?? '你好，我是 Mochi。今天先从文字聊天和长期记忆开始。';
    }, [messages]);

    useEffect(() => {
        GetState()
            .then((state) => {
                setMessages(state.messages ?? []);
                setEmotion(state.emotion || 'neutral');
                setAgentStatus(state.agentStatus || 'offline');
            })
            .catch((reason) => setError(String(reason)));
    }, []);

    useEffect(() => {
        feedRef.current?.scrollTo({
            top: feedRef.current.scrollHeight,
            behavior: 'smooth',
        });
    }, [messages]);

    async function sendMessage(event: FormEvent) {
        event.preventDefault();
        const content = draft.trim();
        if (!content || isSending) {
            return;
        }

        setDraft('');
        setIsSending(true);
        setError('');

        try {
            const response = await SendMessage(content);
            setMessages(response.messages ?? []);
            setEmotion(response.emotion || response.reply?.emotion || 'neutral');
            setAgentStatus(response.agentStatus || 'offline');
        } catch (reason) {
            setError(String(reason));
        } finally {
            setIsSending(false);
        }
    }

    return (
        <main className="app-shell">
            <section className="stage" aria-label="Mochi live2d stage">
                <div className="status-bar">
                    <span>Mochi AI</span>
                    <span>{emotionLabel[emotion] ?? emotion} · Agent {agentStatus}</span>
                </div>

                <div className={`avatar ${emotion}`}>
                    <div className="hair hair-left"/>
                    <div className="hair hair-right"/>
                    <div className="face">
                        <div className="eye left"/>
                        <div className="eye right"/>
                        <div className="blush left"/>
                        <div className="blush right"/>
                        <div className="mouth"/>
                    </div>
                    <div className="body"/>
                </div>

                <div className="speech-bubble">
                    <p>{assistantLine}</p>
                </div>

                <div className="stage-tools">
                    <button type="button">Live2D</button>
                    <button type="button">记忆</button>
                    <button type="button">插件</button>
                    <button type="button">屏幕</button>
                </div>
            </section>

            <section className="chat-panel" aria-label="Mochi chat">
                <header>
                    <div>
                        <p className="eyebrow">Desktop Companion MVP</p>
                        <h1>文字聊天与本地记忆</h1>
                    </div>
                    <span className={`pill agent-${agentStatus}`}>Agent {agentStatus}</span>
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
                            <span className="message-role">{message.role === 'user' ? '你' : 'Mochi'}</span>
                            <p>{message.content}</p>
                        </article>
                    ))}
                </div>

                {error && <div className="error">{error}</div>}

                <form className="composer" onSubmit={sendMessage}>
                    <input
                        value={draft}
                        onChange={(event) => setDraft(event.target.value)}
                        placeholder="和 Mochi 说点什么..."
                        autoComplete="off"
                    />
                    <button type="submit" disabled={isSending || !draft.trim()}>
                        {isSending ? '发送中' : '发送'}
                    </button>
                </form>
            </section>
        </main>
    );
}

export default App;
