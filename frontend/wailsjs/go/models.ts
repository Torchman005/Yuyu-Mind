export namespace agent {
	
	export class TaskSpec {
	    conversation_id?: string;
	    parent_task_id?: string;
	    title: string;
	    goal: string;
	    instructions: string;
	    workspace: string;
	    constraints?: string[];
	    context?: Record<string, any>;
	    allowed_actions?: string[];
	    priority?: number;
	
	    static createFrom(source: any = {}) {
	        return new TaskSpec(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.conversation_id = source["conversation_id"];
	        this.parent_task_id = source["parent_task_id"];
	        this.title = source["title"];
	        this.goal = source["goal"];
	        this.instructions = source["instructions"];
	        this.workspace = source["workspace"];
	        this.constraints = source["constraints"];
	        this.context = source["context"];
	        this.allowed_actions = source["allowed_actions"];
	        this.priority = source["priority"];
	    }
	}

}

export namespace app {
	
	export class ASRReply {
	    text: string;
	    provider: string;
	    language: string;
	    duration: number;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new ASRReply(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.text = source["text"];
	        this.provider = source["provider"];
	        this.language = source["language"];
	        this.duration = source["duration"];
	        this.error = source["error"];
	    }
	}
	export class CompanionMessage {
	    id: string;
	    role: string;
	    content: string;
	    emotion: string;
	    mood: string;
	    energy: number;
	    gesture: string;
	    hand: string;
	    createdAt: string;
	
	    static createFrom(source: any = {}) {
	        return new CompanionMessage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.role = source["role"];
	        this.content = source["content"];
	        this.emotion = source["emotion"];
	        this.mood = source["mood"];
	        this.energy = source["energy"];
	        this.gesture = source["gesture"];
	        this.hand = source["hand"];
	        this.createdAt = source["createdAt"];
	    }
	}
	export class AppState {
	    messages: CompanionMessage[];
	    emotion: string;
	    agentStatus: string;
	    agentProvider: string;
	    providerError: string;
	
	    static createFrom(source: any = {}) {
	        return new AppState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.messages = this.convertValues(source["messages"], CompanionMessage);
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
	    messages: CompanionMessage[];
	    reply: CompanionMessage;
	    speechText: string;
	    emotion: string;
	    mood: string;
	    energy: number;
	    gesture: string;
	    hand: string;
	    agentStatus: string;
	    agentProvider: string;
	    providerError: string;
	
	    static createFrom(source: any = {}) {
	        return new ChatReply(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.messages = this.convertValues(source["messages"], CompanionMessage);
	        this.reply = this.convertValues(source["reply"], CompanionMessage);
	        this.speechText = source["speechText"];
	        this.emotion = source["emotion"];
	        this.mood = source["mood"];
	        this.energy = source["energy"];
	        this.gesture = source["gesture"];
	        this.hand = source["hand"];
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

export namespace chat {
	
	export class ChatRequest {
	    conversation_id: string;
	    session_id?: string;
	    message_id?: string;
	    sender_id?: string;
	    sender_name?: string;
	    content: string;
	    mentioned?: boolean;
	    source_kind?: string;
	    use_tools: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ChatRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.conversation_id = source["conversation_id"];
	        this.session_id = source["session_id"];
	        this.message_id = source["message_id"];
	        this.sender_id = source["sender_id"];
	        this.sender_name = source["sender_name"];
	        this.content = source["content"];
	        this.mentioned = source["mentioned"];
	        this.source_kind = source["source_kind"];
	        this.use_tools = source["use_tools"];
	    }
	}

}

export namespace db {
	
	export class AgentTask {
	    id: string;
	    conversation_id?: string;
	    parent_task_id?: string;
	    title: string;
	    goal: string;
	    status: string;
	    priority: number;
	    spec_json: string;
	    result_json?: string;
	    error?: string;
	    // Go type: time
	    created_at: any;
	    // Go type: time
	    updated_at: any;
	    // Go type: time
	    started_at?: any;
	    // Go type: time
	    completed_at?: any;
	
	    static createFrom(source: any = {}) {
	        return new AgentTask(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.conversation_id = source["conversation_id"];
	        this.parent_task_id = source["parent_task_id"];
	        this.title = source["title"];
	        this.goal = source["goal"];
	        this.status = source["status"];
	        this.priority = source["priority"];
	        this.spec_json = source["spec_json"];
	        this.result_json = source["result_json"];
	        this.error = source["error"];
	        this.created_at = this.convertValues(source["created_at"], null);
	        this.updated_at = this.convertValues(source["updated_at"], null);
	        this.started_at = this.convertValues(source["started_at"], null);
	        this.completed_at = this.convertValues(source["completed_at"], null);
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
	export class AgentTaskControl {
	    id: string;
	    task_id: string;
	    type: string;
	    payload?: string;
	    status: string;
	    // Go type: time
	    created_at: any;
	    // Go type: time
	    applied_at?: any;
	
	    static createFrom(source: any = {}) {
	        return new AgentTaskControl(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.task_id = source["task_id"];
	        this.type = source["type"];
	        this.payload = source["payload"];
	        this.status = source["status"];
	        this.created_at = this.convertValues(source["created_at"], null);
	        this.applied_at = this.convertValues(source["applied_at"], null);
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
	export class AgentTaskEvent {
	    id: string;
	    task_id: string;
	    type: string;
	    level: string;
	    message: string;
	    payload?: string;
	    // Go type: time
	    created_at: any;
	
	    static createFrom(source: any = {}) {
	        return new AgentTaskEvent(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.task_id = source["task_id"];
	        this.type = source["type"];
	        this.level = source["level"];
	        this.message = source["message"];
	        this.payload = source["payload"];
	        this.created_at = this.convertValues(source["created_at"], null);
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
	export class Conversation {
	    id: string;
	    title: string;
	    provider: string;
	    model: string;
	    // Go type: time
	    created_at: any;
	    // Go type: time
	    updated_at: any;
	
	    static createFrom(source: any = {}) {
	        return new Conversation(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.provider = source["provider"];
	        this.model = source["model"];
	        this.created_at = this.convertValues(source["created_at"], null);
	        this.updated_at = this.convertValues(source["updated_at"], null);
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
	export class ConversationSummary {
	    conversation_id: string;
	    summary: string;
	    token_estimate: number;
	    // Go type: time
	    updated_at: any;
	
	    static createFrom(source: any = {}) {
	        return new ConversationSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.conversation_id = source["conversation_id"];
	        this.summary = source["summary"];
	        this.token_estimate = source["token_estimate"];
	        this.updated_at = this.convertValues(source["updated_at"], null);
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
	export class MemoryCandidate {
	    id: string;
	    scope: string;
	    kind: string;
	    key: string;
	    value_json: string;
	    text: string;
	    evidence_count: number;
	    confidence: number;
	    status: string;
	    // Go type: time
	    created_at: any;
	    // Go type: time
	    updated_at: any;
	
	    static createFrom(source: any = {}) {
	        return new MemoryCandidate(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.scope = source["scope"];
	        this.kind = source["kind"];
	        this.key = source["key"];
	        this.value_json = source["value_json"];
	        this.text = source["text"];
	        this.evidence_count = source["evidence_count"];
	        this.confidence = source["confidence"];
	        this.status = source["status"];
	        this.created_at = this.convertValues(source["created_at"], null);
	        this.updated_at = this.convertValues(source["updated_at"], null);
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
	export class Message {
	    id: string;
	    conversation_id: string;
	    role: string;
	    content: string;
	    tool_calls?: string;
	    tool_call_id?: string;
	    source_kind?: string;
	    emotion?: string;
	    mood?: string;
	    energy?: number;
	    gesture?: string;
	    hand?: string;
	    // Go type: time
	    created_at: any;
	
	    static createFrom(source: any = {}) {
	        return new Message(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.conversation_id = source["conversation_id"];
	        this.role = source["role"];
	        this.content = source["content"];
	        this.tool_calls = source["tool_calls"];
	        this.tool_call_id = source["tool_call_id"];
	        this.source_kind = source["source_kind"];
	        this.emotion = source["emotion"];
	        this.mood = source["mood"];
	        this.energy = source["energy"];
	        this.gesture = source["gesture"];
	        this.hand = source["hand"];
	        this.created_at = this.convertValues(source["created_at"], null);
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
	export class TokenUsageRecord {
	    id: string;
	    conversation_id: string;
	    provider: string;
	    model: string;
	    mode: string;
	    prompt_tokens: number;
	    completion_tokens: number;
	    total_tokens: number;
	    model_calls: number;
	    duration_ms: number;
	    status: string;
	    error?: string;
	    // Go type: time
	    created_at: any;
	
	    static createFrom(source: any = {}) {
	        return new TokenUsageRecord(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.conversation_id = source["conversation_id"];
	        this.provider = source["provider"];
	        this.model = source["model"];
	        this.mode = source["mode"];
	        this.prompt_tokens = source["prompt_tokens"];
	        this.completion_tokens = source["completion_tokens"];
	        this.total_tokens = source["total_tokens"];
	        this.model_calls = source["model_calls"];
	        this.duration_ms = source["duration_ms"];
	        this.status = source["status"];
	        this.error = source["error"];
	        this.created_at = this.convertValues(source["created_at"], null);
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
	export class TokenUsageSummary {
	    conversation_id?: string;
	    provider?: string;
	    model?: string;
	    request_count: number;
	    failed_count: number;
	    model_calls: number;
	    prompt_tokens: number;
	    completion_tokens: number;
	    total_tokens: number;
	    total_duration_ms: number;
	    average_duration_ms: number;
	
	    static createFrom(source: any = {}) {
	        return new TokenUsageSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.conversation_id = source["conversation_id"];
	        this.provider = source["provider"];
	        this.model = source["model"];
	        this.request_count = source["request_count"];
	        this.failed_count = source["failed_count"];
	        this.model_calls = source["model_calls"];
	        this.prompt_tokens = source["prompt_tokens"];
	        this.completion_tokens = source["completion_tokens"];
	        this.total_tokens = source["total_tokens"];
	        this.total_duration_ms = source["total_duration_ms"];
	        this.average_duration_ms = source["average_duration_ms"];
	    }
	}
	export class UserMemory {
	    id: string;
	    scope: string;
	    kind: string;
	    key: string;
	    value_json: string;
	    text: string;
	    confidence: number;
	    source: string;
	    source_message_id?: string;
	    status: string;
	    use_count: number;
	    // Go type: time
	    created_at: any;
	    // Go type: time
	    updated_at: any;
	    // Go type: time
	    last_used_at?: any;
	
	    static createFrom(source: any = {}) {
	        return new UserMemory(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.scope = source["scope"];
	        this.kind = source["kind"];
	        this.key = source["key"];
	        this.value_json = source["value_json"];
	        this.text = source["text"];
	        this.confidence = source["confidence"];
	        this.source = source["source"];
	        this.source_message_id = source["source_message_id"];
	        this.status = source["status"];
	        this.use_count = source["use_count"];
	        this.created_at = this.convertValues(source["created_at"], null);
	        this.updated_at = this.convertValues(source["updated_at"], null);
	        this.last_used_at = this.convertValues(source["last_used_at"], null);
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

}

export namespace memory {
	
	export class AddCandidateRequest {
	    scope: string;
	    kind: string;
	    key: string;
	    value: any;
	    text: string;
	    evidence_count: number;
	    confidence: number;
	
	    static createFrom(source: any = {}) {
	        return new AddCandidateRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.scope = source["scope"];
	        this.kind = source["kind"];
	        this.key = source["key"];
	        this.value = source["value"];
	        this.text = source["text"];
	        this.evidence_count = source["evidence_count"];
	        this.confidence = source["confidence"];
	    }
	}
	export class TaskContext {
	    preferences: Record<string, any>;
	    facts?: Record<string, any>;
	    project?: Record<string, any>;
	    instructions?: string[];
	    episodes?: string[];
	    conversation_summary?: string;
	    memory_ids: string[];
	
	    static createFrom(source: any = {}) {
	        return new TaskContext(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.preferences = source["preferences"];
	        this.facts = source["facts"];
	        this.project = source["project"];
	        this.instructions = source["instructions"];
	        this.episodes = source["episodes"];
	        this.conversation_summary = source["conversation_summary"];
	        this.memory_ids = source["memory_ids"];
	    }
	}
	export class TaskContextRequest {
	    task_id?: string;
	    conversation_id?: string;
	    scopes?: string[];
	    query?: string;
	
	    static createFrom(source: any = {}) {
	        return new TaskContextRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.task_id = source["task_id"];
	        this.conversation_id = source["conversation_id"];
	        this.scopes = source["scopes"];
	        this.query = source["query"];
	    }
	}
	export class UpsertMemoryRequest {
	    scope: string;
	    kind: string;
	    key: string;
	    value: any;
	    text: string;
	    confidence: number;
	    source: string;
	    source_message_id?: string;
	
	    static createFrom(source: any = {}) {
	        return new UpsertMemoryRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.scope = source["scope"];
	        this.kind = source["kind"];
	        this.key = source["key"];
	        this.value = source["value"];
	        this.text = source["text"];
	        this.confidence = source["confidence"];
	        this.source = source["source"];
	        this.source_message_id = source["source_message_id"];
	    }
	}

}

export namespace types {
	
	export class ProviderConfig {
	    id: string;
	    name: string;
	    base_url: string;
	    api_key: string;
	    model: string;
	
	    static createFrom(source: any = {}) {
	        return new ProviderConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.base_url = source["base_url"];
	        this.api_key = source["api_key"];
	        this.model = source["model"];
	    }
	}

}

