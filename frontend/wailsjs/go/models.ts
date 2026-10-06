export namespace main {
	
	export class Detection {
	    name: string;
	    kind: string;
	    packageManager: string;
	    portMode: string;
	    scripts: string[];
	    module: string;
	    modules: string[];
	    script: string;
	
	    static createFrom(source: any = {}) {
	        return new Detection(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.kind = source["kind"];
	        this.packageManager = source["packageManager"];
	        this.portMode = source["portMode"];
	        this.scripts = source["scripts"];
	        this.module = source["module"];
	        this.modules = source["modules"];
	        this.script = source["script"];
	    }
	}
	export class FrontendConfig {
	    directory: string;
	    port: number;
	    packageManager: string;
	    script: string;
	    portMode: string;
	    nodeHome: string;
	    toolPath: string;
	    appArgs: string;
	    environment: Record<string, string>;
	    autoProxy: boolean;
	    proxyVariable: string;

	    static createFrom(source: any = {}) {
	        return new FrontendConfig(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.directory = source["directory"];
	        this.port = source["port"];
	        this.packageManager = source["packageManager"];
	        this.script = source["script"];
	        this.portMode = source["portMode"];
	        this.nodeHome = source["nodeHome"];
	        this.toolPath = source["toolPath"];
	        this.appArgs = source["appArgs"];
	        this.environment = source["environment"];
	        this.autoProxy = source["autoProxy"];
	        this.proxyVariable = source["proxyVariable"];
	    }
	}
	export class LogLine {
	    time: string;
	    source: string;
	    level: string;
	    text: string;
	
	    static createFrom(source: any = {}) {
	        return new LogLine(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.time = source["time"];
	        this.source = source["source"];
	        this.level = source["level"];
	        this.text = source["text"];
	    }
	}
	export class Project {
	    id: string;
	    name: string;
	    directory: string;
	    kind: string;
	    port: number;
	    configFile: string;
	    configProperty: string;
	    javaHome: string;
	    toolPath: string;
	    module: string;
	    packageManager: string;
	    script: string;
	    portMode: string;
	    jvmArgs: string;
	    appArgs: string;
	    environment: Record<string, string>;
	    nodeHome: string;
	    frontend?: FrontendConfig;
	
	    static createFrom(source: any = {}) {
	        return new Project(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.directory = source["directory"];
	        this.kind = source["kind"];
	        this.port = source["port"];
	        this.configFile = source["configFile"];
	        this.configProperty = source["configProperty"];
	        this.javaHome = source["javaHome"];
	        this.toolPath = source["toolPath"];
	        this.module = source["module"];
	        this.packageManager = source["packageManager"];
	        this.script = source["script"];
	        this.portMode = source["portMode"];
	        this.jvmArgs = source["jvmArgs"];
	        this.appArgs = source["appArgs"];
	        this.environment = source["environment"];
	        this.nodeHome = source["nodeHome"];
	        this.frontend = this.convertValues(source["frontend"], FrontendConfig);
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
	export class Status {
	    id: string;
	    state: string;
	    pid: number;
	    exitCode?: number;
	    error: string;
	    startedAt?: number;
	    startupDurationMs?: number;
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.state = source["state"];
	        this.pid = source["pid"];
	        this.exitCode = source["exitCode"];
	        this.error = source["error"];
	        this.startedAt = source["startedAt"];
	        this.startupDurationMs = source["startupDurationMs"];
	    }
	}

}

