export namespace main {
	
	export class Detection {
	    name: string;
	    kind: string;
	    packageManager: string;
	    portMode: string;
	    scripts: string[];
	    module: string;
	    modules: string[];
	
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

