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

}
