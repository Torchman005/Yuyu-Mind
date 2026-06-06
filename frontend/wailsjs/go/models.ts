export namespace main {
	
	export class Message {
	    id: number;
	    role: string;
	    content: string;
	    emotion: string;
	    createdAt: string;
	
	    static createFrom(source: any = {}) {
	        return new Message(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.role = source["role"];
	        this.content = source["content"];
	        this.emotion = source["emotion"];
	        this.createdAt = source["createdAt"];
	    }
	}
	export class AppState {
	    messages: Message[];
	    emotion: string;
	    agentStatus: string;
	    agentProvider: string;
	    providerError: string;
	
	    static createFrom(source: any = {}) {
	        return new AppState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.messages = this.convertValues(source["messages"], Message);
	        this.emotion = source["emotion"];
	        this.agentStatus = source["agentStatus"];
	        this.agentProvider = source["agentProvider"];
	        this.providerError = source["providerError"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ChatReply {
	    messages: Message[];
	    reply: Message;
	    speechText: string;
	    emotion: string;
	    agentStatus: string;
	    agentProvider: string;
	    providerError: string;
	
	    static createFrom(source: any = {}) {
	        return new ChatReply(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.messages = this.convertValues(source["messages"], Message);
	        this.reply = this.convertValues(source["reply"], Message);
	        this.speechText = source["speechText"];
	        this.emotion = source["emotion"];
	        this.agentStatus = source["agentStatus"];
	        this.agentProvider = source["agentProvider"];
	        this.providerError = source["providerError"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class FishLiveProbeResult {
	    ok: boolean;
	    error?: string;
	    events: string[];
	    elapsedMs: number;
	    audioSize: number;
	
	    static createFrom(source: any = {}) {
	        return new FishLiveProbeResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ok = source["ok"];
	        this.error = source["error"];
	        this.events = source["events"];
	        this.elapsedMs = source["elapsedMs"];
	        this.audioSize = source["audioSize"];
	    }
	}
	
	export class PetHitTestState {
	    enabled: boolean;
	    controlsOpen: boolean;
	    x: number;
	    y: number;
	    width: number;
	    height: number;
	
	    static createFrom(source: any = {}) {
	        return new PetHitTestState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.controlsOpen = source["controlsOpen"];
	        this.x = source["x"];
	        this.y = source["y"];
	        this.width = source["width"];
	        this.height = source["height"];
	    }
	}
	export class PluginInfo {
	    schemaVersion: string;
	    name: string;
	    displayName: string;
	    description: string;
	    version: string;
	    author: string;
	    enabled: boolean;
	    entry: string;
	    permissions: string[];
	    context: Record<string, any>;
	    config: Record<string, any>;
	    configSchema: Record<string, any>;
	    actions: any[];
	    loadedActions: string[];

	    static createFrom(source: any = {}) {
	        return new PluginInfo(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.schemaVersion = source["schemaVersion"];
	        this.name = source["name"];
	        this.displayName = source["displayName"];
	        this.description = source["description"];
	        this.version = source["version"];
	        this.author = source["author"];
	        this.enabled = source["enabled"];
	        this.entry = source["entry"];
	        this.permissions = source["permissions"];
	        this.context = source["context"];
	        this.config = source["config"];
	        this.configSchema = source["configSchema"];
	        this.actions = source["actions"];
	        this.loadedActions = source["loadedActions"];
	    }
	}
	export class PluginListReply {
	    ok: boolean;
	    plugins: PluginInfo[];

	    static createFrom(source: any = {}) {
	        return new PluginListReply(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ok = source["ok"];
	        this.plugins = this.convertValues(source["plugins"], PluginInfo);
	    }

		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class SpeechReply {
	    audioBase64: string;
	    contentType: string;
	    provider: string;
	
	    static createFrom(source: any = {}) {
	        return new SpeechReply(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.audioBase64 = source["audioBase64"];
	        this.contentType = source["contentType"];
	        this.provider = source["provider"];
	    }
	}
	export class SpeechStreamStart {
	    sessionId: string;
	    contentType: string;
	    provider: string;
	
	    static createFrom(source: any = {}) {
	        return new SpeechStreamStart(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sessionId = source["sessionId"];
	        this.contentType = source["contentType"];
	        this.provider = source["provider"];
	    }
	}

}
