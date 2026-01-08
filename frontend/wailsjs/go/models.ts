export namespace todo {
	
	export class Settings {
	    hideDone: boolean;
	    alwaysOnTop: boolean;
	    viewMode: string;
	    conciseMode: boolean;
	    theme: string;
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.hideDone = source["hideDone"];
	        this.alwaysOnTop = source["alwaysOnTop"];
	        this.viewMode = source["viewMode"];
	        this.conciseMode = source["conciseMode"];
	        this.theme = source["theme"];
	    }
	}
	export class Task {
	    id: number;
	    groupId: number;
	    title: string;
	    content: string;
	    status: string;
	    important: boolean;
	    urgent: boolean;
	    createdAt: number;
	    updatedAt: number;
	
	    static createFrom(source: any = {}) {
	        return new Task(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.groupId = source["groupId"];
	        this.title = source["title"];
	        this.content = source["content"];
	        this.status = source["status"];
	        this.important = source["important"];
	        this.urgent = source["urgent"];
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	    }
	}
	export class Group {
	    id: number;
	    name: string;
	    createdAt: number;
	    updatedAt: number;
	
	    static createFrom(source: any = {}) {
	        return new Group(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	    }
	}
	export class Board {
	    groups: Group[];
	    tasks: Task[];
	    settings: Settings;
	    statuses: string[];
	
	    static createFrom(source: any = {}) {
	        return new Board(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.groups = this.convertValues(source["groups"], Group);
	        this.tasks = this.convertValues(source["tasks"], Task);
	        this.settings = this.convertValues(source["settings"], Settings);
	        this.statuses = source["statuses"];
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

export namespace version {
	
	export class ReleaseInfo {
	    version: string;
	    name: string;
	    description: string;
	    publishedAt: string;
	    downloadUrl: string;
	    pageUrl: string;
	    required: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ReleaseInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.publishedAt = source["publishedAt"];
	        this.downloadUrl = source["downloadUrl"];
	        this.pageUrl = source["pageUrl"];
	        this.required = source["required"];
	    }
	}
	export class UpdateCheckResult {
	    hasUpdate: boolean;
	    currentVersion: string;
	    latestRelease?: ReleaseInfo;
	
	    static createFrom(source: any = {}) {
	        return new UpdateCheckResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.hasUpdate = source["hasUpdate"];
	        this.currentVersion = source["currentVersion"];
	        this.latestRelease = this.convertValues(source["latestRelease"], ReleaseInfo);
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

