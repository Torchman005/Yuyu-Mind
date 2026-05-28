import {useEffect, useRef, useState} from 'react';

type Live2DStageProps = {
    emotion: string;
};

type RendererStatus = 'ready' | 'fallback' | 'loading';

type DebugInfo = {
    phase: string;
    detail: string;
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

const legacyMotionByEmotion: Record<string, string> = {
    happy: 'Idle',
    focused: 'Idle',
    thinking: 'Idle',
    neutral: 'Idle',
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

    canvas.style.width = `${width}px`;
    canvas.style.height = `${height}px`;
    canvas.style.minWidth = `${width}px`;
    canvas.style.minHeight = `${height}px`;
    canvas.style.display = 'block';
}

function timeoutAfter<T>(promise: Promise<T> | T, timeoutMs: number, label: string): Promise<T> {
    return Promise.race([
        Promise.resolve(promise),
        new Promise<T>((_, reject) => {
            window.setTimeout(() => reject(new Error(`${label} timed out after ${timeoutMs}ms`)), timeoutMs);
        }),
    ]);
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

export function Live2DStage({emotion}: Live2DStageProps) {
    const containerRef = useRef<HTMLDivElement>(null);
    const canvasRef = useRef<HTMLCanvasElement>(null);
    const appRef = useRef<any>(null);
    const modelRef = useRef<any>(null);
    const controllerRef = useRef<AvatarController | null>(null);
    const bootIdRef = useRef(0);
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
            const app = new PIXI.Application({
                view: canvasRef.current,
                resizeTo: containerRef.current,
                transparent: true,
                backgroundAlpha: 0,
                antialias: true,
                autoStart: true,
                resolution: window.devicePixelRatio || 1,
            });
            localApp = app;
            if (!isCurrentBoot()) {
                localApp.destroy(false);
                return;
            }
            appRef.current = localApp;
            syncCanvasDisplaySize(canvasRef.current, containerRef.current);
            app.renderer.resize(containerRef.current.clientWidth || 1, containerRef.current.clientHeight || 1);
            const gl = (app.renderer as any).gl as WebGLRenderingContext | undefined;
            updateDebug('pixi-ready', {
                maxTexture: gl?.getParameter?.(gl.MAX_TEXTURE_SIZE) ?? 'unknown',
                maxTextureUnits: gl?.getParameter?.(gl.MAX_TEXTURE_IMAGE_UNITS) ?? 'unknown',
                pixiEnv: PIXI.settings.PREFER_ENV,
            });

            setStatusText('Loading Live2D model');
            updateDebug('loading-model');
            const model = await Live2DModel.from(resolvedModelUrl, {autoInteract: true});
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
            syncCanvasDisplaySize(canvasRef.current, containerRef.current);
            fitPixiModel(model, containerRef.current);
            app.renderer.resize(containerRef.current.clientWidth || 1, containerRef.current.clientHeight || 1);
            app.render();
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

        const motion = legacyMotionByEmotion[emotion] ?? legacyMotionByEmotion.neutral;
        model.motion?.(motion)?.catch?.(() => undefined);
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
                updateDebug('resize');
            }
        }

        window.addEventListener('resize', onResize);
        return () => window.removeEventListener('resize', onResize);
    }, []);

    return (
        <div className="live2d-stage" ref={containerRef}>
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
