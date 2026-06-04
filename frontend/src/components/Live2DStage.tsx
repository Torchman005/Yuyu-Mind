import {useEffect, useRef, useState} from 'react';

type Live2DStageProps = {
    emotion: string;
    isSpeaking?: boolean;
    mouthLevel?: number;
    petScale?: number;
};

type RendererStatus = 'ready' | 'fallback' | 'loading';

type DebugInfo = {
    phase: string;
    detail: string;
};

type NaturalPresenceState = {
    gazeX: number;
    gazeY: number;
    targetX: number;
    targetY: number;
    nextGazeShiftAt: number;
    nextBlinkAt: number;
    blinkUntil: number;
};

type AvatarController = {
    setEmotion?: (emotion: string) => void;
    resize?: () => void;
    destroy?: () => void;
};

type Cubism5Renderer = {
    create: (options: {
        canvas: HTMLCanvasElement;
        modelUrl: string;
        emotion: string;
    }) => Promise<AvatarController> | AvatarController;
};

const avatarRenderer = (import.meta.env.VITE_AVATAR_RENDERER as string | undefined) || 'css';
const modelUrl = import.meta.env.VITE_LIVE2D_MODEL_URL as string | undefined;
const cubism5CoreUrl = import.meta.env.VITE_CUBISM5_CORE_URL as string | undefined;
const cubism5RendererUrl = '/live2d/cubism5/mochi-cubism5-renderer.v2.js';
const legacyCubismCoreUrl = import.meta.env.VITE_LIVE2D_CUBISM_CORE_URL as string | undefined;
const cubism2CoreUrl = (import.meta.env.VITE_LIVE2D_CUBISM2_CORE_URL as string | undefined) || '/live2d/live2d.min.js';
const live2dDebug = (import.meta.env.VITE_LIVE2D_DEBUG as string | undefined) === 'true';

const expressionByEmotion: Record<string, string | null> = {
    happy: 'happy',
    focused: 'focused',
    thinking: 'thinking',
    neutral: null,
    sad: 'sad',
    surprised: 'surprised',
};

const motionByEmotion: Record<string, string | null> = {
    happy: null,
    focused: null,
    thinking: null,
    neutral: null,
    sad: null,
    surprised: null,
};

declare global {
    interface Window {
        PIXI?: any;
        Live2DCubismCore?: unknown;
        MochiCubism5Renderer?: Cubism5Renderer;
    }
}

function cacheBustedUrl(src: string) {
    const url = new URL(src, window.location.origin);
    url.searchParams.set('mochiVersion', 'cubism5-v2');
    return `${url.pathname}${url.search}`;
}

function loadScriptOnce(src: string, forceReload = false): Promise<void> {
    const finalSrc = forceReload ? cacheBustedUrl(src) : src;
    const existing = document.querySelector<HTMLScriptElement>(`script[src="${finalSrc}"]`);
    if (existing) {
        return Promise.resolve();
    }

    if (forceReload) {
        document.querySelectorAll<HTMLScriptElement>('script[src*="mochi-cubism5-renderer"]').forEach((script) => {
            script.remove();
        });
    }

    return new Promise((resolve, reject) => {
        const script = document.createElement('script');
        script.src = finalSrc;
        script.async = true;
        script.onload = () => resolve();
        script.onerror = () => reject(new Error(`Failed to load ${finalSrc}`));
        document.head.appendChild(script);
    });
}

function fitPixiModel(model: any, container: HTMLDivElement) {
    const width = container.clientWidth;
    const height = container.clientHeight;
    const baseModelWidth =
        Number(model?.internalModel?.width) ||
        Number(model?.internalModel?.originalWidth) ||
        Number(model?.width) ||
        1;
    const baseModelHeight =
        Number(model?.internalModel?.height) ||
        Number(model?.internalModel?.originalHeight) ||
        Number(model?.height) ||
        1;

    if (!width || !height) {
        return;
    }

    const scale = Math.min((width * 0.92) / baseModelWidth, (height * 0.92) / baseModelHeight);
    model.visible = true;
    model.alpha = 1;
    model.scale.set(scale);
    model.x = width / 2;
    model.y = height / 2;
    model.anchor?.set?.(0.5, 0.5);
}

function syncCanvasDisplaySize(canvas: HTMLCanvasElement, container: HTMLDivElement) {
    const width = container.clientWidth || 1;
    const height = container.clientHeight || 1;

    canvas.style.background = 'transparent';
    canvas.style.width = `${width}px`;
    canvas.style.height = `${height}px`;
    canvas.style.minWidth = `${width}px`;
    canvas.style.minHeight = `${height}px`;
    canvas.style.display = 'block';
}

function clearPixiBackground(app: any) {
    app.renderer.backgroundAlpha = 0;
    app.renderer.backgroundColor = 0x000000;

    const gl = app.renderer?.gl as WebGLRenderingContext | undefined;
    if (!gl) {
        return;
    }

    gl.clearColor(0, 0, 0, 0);
    gl.clear(gl.COLOR_BUFFER_BIT);
}

function timeoutAfter<T>(promise: Promise<T> | T, timeoutMs: number, label: string): Promise<T> {
    return Promise.race([
        Promise.resolve(promise),
        new Promise<T>((_, reject) => {
            window.setTimeout(() => reject(new Error(`${label} timed out after ${timeoutMs}ms`)), timeoutMs);
        }),
    ]);
}

function resetPixiExpression(model: any) {
    model?.internalModel?.motionManager?.expressionManager?.resetExpression?.();
}

function stopPixiMotions(model: any) {
    model?.internalModel?.motionManager?.stopAllMotions?.();
}

function applyPixiEmotion(model: any, emotion: string) {
    const expression = expressionByEmotion[emotion] ?? null;
    const motion = motionByEmotion[emotion] ?? null;

    stopPixiMotions(model);

    if (expression) {
        model.expression?.(expression)?.catch?.(() => undefined);
    } else {
        resetPixiExpression(model);
    }

    if (motion) {
        model.motion?.(motion)?.catch?.(() => undefined);
    }
}

function setModelParameter(model: any, id: string, value: number, weight = 1) {
    const coreModel = model?.internalModel?.coreModel;
    const parameterIndex = coreModel?.getParameterIndex?.(id);

    coreModel?.setParameterValueById?.(id, value, weight);
    coreModel?.setParamFloat?.(id, value, weight);

    if (typeof parameterIndex === 'number' && parameterIndex >= 0) {
        coreModel?.setParameterValueByIndex?.(parameterIndex, value, weight);
    }
}

function createNaturalPresenceState(): NaturalPresenceState {
    return {
        gazeX: 0,
        gazeY: 0,
        targetX: 0,
        targetY: 0.02,
        nextGazeShiftAt: 0,
        nextBlinkAt: 1200 + Math.random() * 1800,
        blinkUntil: 0,
    };
}

function emotionPresenceOffset(emotion: string) {
    switch (emotion) {
        case 'happy':
            return {x: 0.01, y: 0.04, mouth: 0.25};
        case 'focused':
        case 'thinking':
            return {x: -0.015, y: 0.015, mouth: 0.02};
        case 'sad':
            return {x: 0, y: -0.035, mouth: -0.28};
        case 'surprised':
            return {x: 0.02, y: 0.03, mouth: 0.1};
        default:
            return {x: 0, y: 0.015, mouth: 0.08};
    }
}

function applyNaturalPresence(
    model: any,
    state: NaturalPresenceState,
    isSpeaking: boolean,
    mouthLevel: number,
    emotion: string,
    elapsedMs: number,
) {
    if (elapsedMs >= state.nextGazeShiftAt) {
        state.targetX = (Math.random() - 0.5) * 0.18;
        state.targetY = 0.02 + (Math.random() - 0.5) * 0.1;
        state.nextGazeShiftAt = elapsedMs + 1800 + Math.random() * 2800;
    }

    const offset = emotionPresenceOffset(emotion);
    state.gazeX += (state.targetX + offset.x - state.gazeX) * 0.025;
    state.gazeY += (state.targetY + offset.y - state.gazeY) * 0.025;

    const microX = Math.sin(elapsedMs / 1850) * 0.012 + Math.sin(elapsedMs / 4300) * 0.008;
    const microY = Math.sin(elapsedMs / 2600) * 0.01;
    const activeSpeech = isSpeaking && mouthLevel > 0.015;
    const talkBob = activeSpeech ? Math.sin(elapsedMs / 190) * Math.min(1, mouthLevel * 2.4) : 0;
    const lookX = state.gazeX + microX;
    const lookY = state.gazeY + microY;

    setModelParameter(model, 'ParamEyeBallX', lookX, 0.72);
    setModelParameter(model, 'ParamEyeBallY', lookY, 0.72);
    setModelParameter(model, 'ParamhitomiX', lookX, 0.72);
    setModelParameter(model, 'ParamhitomiY', lookY, 0.72);
    setModelParameter(model, 'ParamAngleX', lookX * 12 + talkBob * 0.55, 0.58);
    setModelParameter(model, 'ParamAngleY', lookY * 9 + (activeSpeech ? Math.sin(elapsedMs / 310) * 0.35 : 0), 0.58);
    setModelParameter(model, 'ParamAngleZ', Math.sin(elapsedMs / 3400) * 0.45 + (activeSpeech ? Math.sin(elapsedMs / 480) * 0.25 : 0), 0.45);
    setModelParameter(model, 'ParamBodyAngleX', lookX * 2.2, 0.25);
    setModelParameter(model, 'ParamMouthForm', offset.mouth + (activeSpeech ? 0.08 : 0), 0.22);

    if (elapsedMs >= state.nextBlinkAt) {
        state.blinkUntil = elapsedMs + 125;
        state.nextBlinkAt = elapsedMs + 2400 + Math.random() * 4200;
    }
    const blinkProgress = state.blinkUntil > elapsedMs ? 1 - (state.blinkUntil - elapsedMs) / 125 : 1;
    const blinkAmount = state.blinkUntil > elapsedMs ? Math.sin(Math.PI * blinkProgress) : 0;
    const eyeOpen = Math.max(0.08, 1 - blinkAmount * 0.92);
    setModelParameter(model, 'ParamEyeLOpen', eyeOpen, 0.72);
    setModelParameter(model, 'ParamEyeROpen', eyeOpen, 0.72);
}

function applyLipSync(model: any, isSpeaking: boolean, mouthLevel: number, elapsedMs: number) {
    if (!isSpeaking || mouthLevel <= 0.012) {
        setModelParameter(model, 'ParamMouthOpenY', 0, 0.8);
        setModelParameter(model, 'ParamJawOpen', 0, 0.8);
        return;
    }

    const pulse = Math.abs(Math.sin(elapsedMs / 92));
    const openness = Math.min(1, Math.max(0, mouthLevel * 1.9 + pulse * mouthLevel * 0.55));
    setModelParameter(model, 'ParamMouthOpenY', openness, 1);
    setModelParameter(model, 'ParamJawOpen', openness * 0.45, 1);
}

function FallbackAvatar({emotion}: Live2DStageProps) {
    return (
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
    );
}

function initialStatusText() {
    if (avatarRenderer === 'css') {
        return 'CSS avatar renderer';
    }
    if (!modelUrl) {
        return 'No Live2D model URL';
    }
    return `Loading ${avatarRenderer}`;
}

export function Live2DStage({emotion, isSpeaking = false, mouthLevel = 0, petScale = 1}: Live2DStageProps) {
    const containerRef = useRef<HTMLDivElement>(null);
    const canvasRef = useRef<HTMLCanvasElement>(null);
    const appRef = useRef<any>(null);
    const modelRef = useRef<any>(null);
    const controllerRef = useRef<AvatarController | null>(null);
    const bootIdRef = useRef(0);
    const lastEmotionRef = useRef('');
    const emotionRef = useRef(emotion);
    const isSpeakingRef = useRef(isSpeaking);
    const mouthLevelRef = useRef(mouthLevel);
    const [status, setStatus] = useState<RendererStatus>(avatarRenderer === 'css' ? 'fallback' : 'loading');
    const [statusText, setStatusText] = useState(initialStatusText);
    const [debugInfo, setDebugInfo] = useState<DebugInfo>({
        phase: avatarRenderer,
        detail: modelUrl || 'not configured',
    });

    function updateDebug(phase: string, extra: Record<string, unknown> = {}) {
        const container = containerRef.current;
        const canvas = canvasRef.current;
        const app = appRef.current;
        const model = modelRef.current;
        const canvasRect = canvas?.getBoundingClientRect();
        let bounds = 'none';
        if (model?.getBounds) {
            try {
                const b = model.getBounds(false);
                bounds = `${Math.round(b.x)},${Math.round(b.y)} ${Math.round(b.width)}x${Math.round(b.height)}`;
            } catch {
                bounds = 'error';
            }
        }
        const info = {
            model: modelUrl || 'not configured',
            container: container ? `${container.clientWidth}x${container.clientHeight}` : 'none',
            canvas: canvas ? `${canvas.clientWidth}x${canvas.clientHeight}/${canvas.width}x${canvas.height}` : 'none',
            canvasRect: canvasRect ? `${Math.round(canvasRect.width)}x${Math.round(canvasRect.height)}` : 'none',
            canvasStyle: canvas ? `${canvas.style.width || 'css'}x${canvas.style.height || 'css'}` : 'none',
            renderer: app?.renderer ? `${app.renderer.width}x${app.renderer.height}` : 'none',
            children: app?.stage?.children?.length ?? 0,
            parent: model?.parent ? 'yes' : 'no',
            modelSize: model
                ? `${Number(model?.internalModel?.width) || Number(model?.internalModel?.originalWidth) || Number(model?.width) || 0}x${
                    Number(model?.internalModel?.height) || Number(model?.internalModel?.originalHeight) || Number(model?.height) || 0
                }`
                : 'none',
            modelPos: model ? `${Math.round(model.x || 0)},${Math.round(model.y || 0)} scale=${Number(model.scale?.x || 0).toFixed(4)}` : 'none',
            bounds,
            renderable: model ? String(model.renderable ?? 'unknown') : 'none',
            ...extra,
        };
        const detail = Object.entries(info)
            .map(([key, value]) => `${key}: ${String(value)}`)
            .join(' | ');
        setDebugInfo({phase, detail});
        (window as any).__MOCHI_LIVE2D_STATUS = {phase, ...info};
    }

    useEffect(() => {
        emotionRef.current = emotion;
    }, [emotion]);

    useEffect(() => {
        isSpeakingRef.current = isSpeaking;
    }, [isSpeaking]);

    useEffect(() => {
        mouthLevelRef.current = mouthLevel;
    }, [mouthLevel]);

    useEffect(() => {
        const bootId = bootIdRef.current + 1;
        bootIdRef.current = bootId;
        let disposed = false;
        let localApp: any = null;
        let localModel: any = null;
        let localController: AvatarController | null = null;

        function isCurrentBoot() {
            return !disposed && bootIdRef.current === bootId;
        }

        async function bootRenderer() {
            if (avatarRenderer === 'css') {
                setStatusText('CSS avatar renderer');
                setStatus('fallback');
                return;
            }

            if (!modelUrl || !containerRef.current || !canvasRef.current) {
                setStatusText('No Live2D model URL');
                setStatus('fallback');
                return;
            }
            syncCanvasDisplaySize(canvasRef.current, containerRef.current);

            try {
                updateDebug('boot');
                if (avatarRenderer === 'cubism5') {
                    await bootCubism5Renderer();
                    return;
                }

                if (avatarRenderer === 'pixi' || avatarRenderer === 'legacy-pixi') {
                    await bootPixiRenderer();
                    return;
                }

                throw new Error(`Unsupported VITE_AVATAR_RENDERER: ${avatarRenderer}`);
            } catch (error) {
                console.warn('Avatar renderer fallback:', error);
                setStatusText(error instanceof Error ? error.message : String(error));
                updateDebug('error', {error: error instanceof Error ? error.message : String(error)});
                setStatus('fallback');
            }
        }

        async function bootCubism5Renderer() {
            if (!canvasRef.current) {
                return;
            }
            if (!cubism5CoreUrl) {
                throw new Error('VITE_CUBISM5_CORE_URL is required for Cubism 5');
            }
            if (!cubism5RendererUrl) {
                throw new Error('VITE_CUBISM5_RENDERER_URL is required for Cubism 5');
            }

            setStatusText('Loading Cubism 5 Core');
            await loadScriptOnce(cubism5CoreUrl);
            if (!window.Live2DCubismCore) {
                throw new Error('Cubism 5 Core loaded, but window.Live2DCubismCore is missing');
            }

            setStatusText('Loading Cubism 5 renderer bridge');
            window.MochiCubism5Renderer = undefined;
            await loadScriptOnce(cubism5RendererUrl, true);
            const renderer = window.MochiCubism5Renderer as Cubism5Renderer | undefined;
            if (!renderer?.create) {
                throw new Error('MochiCubism5Renderer.create was not found');
            }

            setStatusText('Loading Cubism 5 model');
            const controller = await timeoutAfter(
                renderer.create({
                    canvas: canvasRef.current,
                    modelUrl: modelUrl as string,
                    emotion,
                }),
                3000,
                'Cubism 5 renderer bridge',
            );
            if (disposed) {
                controller?.destroy?.();
                return;
            }
            localController = controller;
            if (!isCurrentBoot()) {
                localController?.destroy?.();
                return;
            }
            controllerRef.current = localController;
            setStatusText('Cubism 5 ready');
            setStatus('ready');
        }

        async function bootPixiRenderer() {
            if (!containerRef.current || !canvasRef.current) {
                return;
            }
            syncCanvasDisplaySize(canvasRef.current, containerRef.current);
            const resolvedModelUrl = modelUrl as string;

            setStatusText('Loading Pixi');
            updateDebug('loading-pixi');
            const PIXI = await import('pixi.js');
            window.PIXI = PIXI;
            PIXI.settings.PREFER_ENV = PIXI.ENV.WEBGL_LEGACY;
            PIXI.settings.SPRITE_MAX_TEXTURES = 1;

            if (legacyCubismCoreUrl && !window.Live2DCubismCore) {
                setStatusText('Loading Cubism Core');
                updateDebug('loading-cubism-core');
                await loadScriptOnce(legacyCubismCoreUrl);
            }
            if (cubism2CoreUrl && !(window as any).Live2D) {
                setStatusText('Loading Cubism 2 runtime');
                updateDebug('loading-cubism2-core');
                await loadScriptOnce(cubism2CoreUrl);
            }

            setStatusText('Loading Live2D plugin');
            updateDebug('loading-plugin', {
                hasCubismCore: Boolean(window.Live2DCubismCore),
                hasLive2D: Boolean((window as any).Live2D),
                pixiEnv: PIXI.settings.PREFER_ENV,
                spriteMaxTextures: PIXI.settings.SPRITE_MAX_TEXTURES,
            });
            const {Live2DModel} = await import('pixi-live2d-display');
            Live2DModel.registerTicker?.(PIXI.Ticker);
            const context =
                canvasRef.current.getContext('webgl2', {
                    alpha: true,
                    premultipliedAlpha: false,
                    preserveDrawingBuffer: true,
                    antialias: true,
                }) ||
                canvasRef.current.getContext('webgl', {
                    alpha: true,
                    premultipliedAlpha: false,
                    preserveDrawingBuffer: true,
                    antialias: true,
                });
            const app = new PIXI.Application({
                view: canvasRef.current,
                context: (context || undefined) as any,
                resizeTo: containerRef.current,
                transparent: true,
                useContextAlpha: 'notMultiplied',
                premultipliedAlpha: false,
                preserveDrawingBuffer: true,
                clearBeforeRender: true,
                backgroundColor: 0x000000,
                backgroundAlpha: 0,
                antialias: true,
                autoStart: true,
                resolution: window.devicePixelRatio || 1,
            } as any);
            localApp = app;
            if (!isCurrentBoot()) {
                localApp.destroy(false);
                return;
            }
            appRef.current = localApp;
            syncCanvasDisplaySize(canvasRef.current, containerRef.current);
            app.renderer.resize(containerRef.current.clientWidth || 1, containerRef.current.clientHeight || 1);
            clearPixiBackground(app);
            const gl = (app.renderer as any).gl as WebGLRenderingContext | undefined;
            updateDebug('pixi-ready', {
                maxTexture: gl?.getParameter?.(gl.MAX_TEXTURE_SIZE) ?? 'unknown',
                maxTextureUnits: gl?.getParameter?.(gl.MAX_TEXTURE_IMAGE_UNITS) ?? 'unknown',
                pixiEnv: PIXI.settings.PREFER_ENV,
            });

            setStatusText('Loading Live2D model');
            updateDebug('loading-model');
            const model = await Live2DModel.from(resolvedModelUrl, {autoInteract: false});
            localModel = model;
            if (!isCurrentBoot()) {
                app.destroy(false);
                model.destroy();
                return;
            }

            app.stage.addChild(model);
            if (!model.parent) {
                app.stage.addChildAt(model, 0);
            }
            if (!app.stage.children.includes(model)) {
                throw new Error('Live2D model loaded, but Pixi stage did not retain it');
            }
            modelRef.current = model;
            let elapsedMs = 0;
            const presenceState = createNaturalPresenceState();
            app.ticker.add((delta: number) => {
                elapsedMs += app.ticker.deltaMS || delta * 16.67;
                applyNaturalPresence(model, presenceState, isSpeakingRef.current, mouthLevelRef.current, emotionRef.current, elapsedMs);
                applyLipSync(model, isSpeakingRef.current, mouthLevelRef.current, elapsedMs);
            });
            syncCanvasDisplaySize(canvasRef.current, containerRef.current);
            fitPixiModel(model, containerRef.current);
            app.renderer.resize(containerRef.current.clientWidth || 1, containerRef.current.clientHeight || 1);
            clearPixiBackground(app);
            app.render();
            lastEmotionRef.current = emotion;
            applyPixiEmotion(model, emotion);
            updateDebug('ready', {
                maxTexture: gl?.getParameter?.(gl.MAX_TEXTURE_SIZE) ?? 'unknown',
                maxTextureUnits: gl?.getParameter?.(gl.MAX_TEXTURE_IMAGE_UNITS) ?? 'unknown',
            });
            requestAnimationFrame(() => {
                if (isCurrentBoot() && modelRef.current && containerRef.current) {
                    if (canvasRef.current) {
                        syncCanvasDisplaySize(canvasRef.current, containerRef.current);
                    }
                    if (appRef.current?.stage && !appRef.current.stage.children.includes(modelRef.current)) {
                        appRef.current.stage.addChild(modelRef.current);
                    }
                    fitPixiModel(modelRef.current, containerRef.current);
                    clearPixiBackground(appRef.current);
                    appRef.current?.render?.();
                    updateDebug('ready-after-frame', {
                        maxTexture: gl?.getParameter?.(gl.MAX_TEXTURE_SIZE) ?? 'unknown',
                        maxTextureUnits: gl?.getParameter?.(gl.MAX_TEXTURE_IMAGE_UNITS) ?? 'unknown',
                    });
                }
            });
            setStatusText('Live2D ready');
            setStatus('ready');
        }

        bootRenderer();

        return () => {
            disposed = true;
            localController?.destroy?.();
            localModel?.destroy?.();
            localApp?.destroy?.(false);
            if (controllerRef.current === localController) {
                controllerRef.current = null;
            }
            if (modelRef.current === localModel) {
                modelRef.current = null;
            }
            if (appRef.current === localApp) {
                appRef.current = null;
            }
        };
    }, []);

    useEffect(() => {
        controllerRef.current?.setEmotion?.(emotion);

        const model = modelRef.current;
        if (!model || status !== 'ready') {
            return;
        }

        if (lastEmotionRef.current === emotion) {
            return;
        }

        lastEmotionRef.current = emotion;
        applyPixiEmotion(model, emotion);
    }, [emotion, status]);

    useEffect(() => {
        function onResize() {
            controllerRef.current?.resize?.();
            if (modelRef.current && containerRef.current) {
                if (canvasRef.current) {
                    syncCanvasDisplaySize(canvasRef.current, containerRef.current);
                }
                fitPixiModel(modelRef.current, containerRef.current);
                appRef.current?.renderer?.resize?.(containerRef.current.clientWidth || 1, containerRef.current.clientHeight || 1);
                clearPixiBackground(appRef.current);
                updateDebug('resize');
            }
        }

        window.addEventListener('resize', onResize);
        return () => window.removeEventListener('resize', onResize);
    }, []);

    useEffect(() => {
        controllerRef.current?.resize?.();
        if (modelRef.current && containerRef.current) {
            if (canvasRef.current) {
                syncCanvasDisplaySize(canvasRef.current, containerRef.current);
            }
            fitPixiModel(modelRef.current, containerRef.current);
            appRef.current?.renderer?.resize?.(containerRef.current.clientWidth || 1, containerRef.current.clientHeight || 1);
            clearPixiBackground(appRef.current);
            appRef.current?.render?.();
            updateDebug('scale');
        }
    }, [petScale, status]);

    return (
        <div className="live2d-stage" ref={containerRef} style={{'--pet-scale': petScale} as any}>
            <canvas className={status === 'ready' ? 'live2d-canvas ready' : 'live2d-canvas'} ref={canvasRef}/>
            {status !== 'ready' && <FallbackAvatar emotion={emotion}/>}
            {avatarRenderer !== 'css' && (live2dDebug || status !== 'ready') && (
                <div className="live2d-status">
                    <strong>{statusText}</strong>
                    <span>renderer: {avatarRenderer}</span>
                    <span>{debugInfo.phase}</span>
                    <span>{debugInfo.detail}</span>
                </div>
            )}
        </div>
    );
}
