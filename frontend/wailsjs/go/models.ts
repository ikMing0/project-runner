export namespace main {
	
	export class CodexModel {
	    id: string;
	    name: string;
	    reasoning: string[];

	    static createFrom(source: any = {}) {
	        return new CodexModel(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.reasoning = source["reasoning"];
	    }
	}
	export class CodexOptions {
	    executable: string;
	    defaultModel: string;
	    configuredModel: string;
	    catalogError: string;
	    models: CodexModel[];

	    static createFrom(source: any = {}) {
	        return new CodexOptions(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.executable = source["executable"];
	        this.defaultModel = source["defaultModel"];
	        this.configuredModel = source["configuredModel"];
	        this.catalogError = source["catalogError"];
	        this.models = this.convertValues(source["models"], CodexModel);
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
	export class GitChange {
	    path: string;
	    oldPath?: string;
	    indexStatus: string;
	    worktreeStatus: string;
	    staged: boolean;
	    unstaged: boolean;
	    untracked: boolean;
	    conflict: boolean;

	    static createFrom(source: any = {}) {
	        return new GitChange(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.oldPath = source["oldPath"];
	        this.indexStatus = source["indexStatus"];
	        this.worktreeStatus = source["worktreeStatus"];
	        this.staged = source["staged"];
	        this.unstaged = source["unstaged"];
	        this.untracked = source["untracked"];
	        this.conflict = source["conflict"];
	    }
	}
	export class GitChanges {
	    root: string;
	    branch: string;
	    detached: boolean;
	    files: GitChange[];

	    static createFrom(source: any = {}) {
	        return new GitChanges(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.root = source["root"];
	        this.branch = source["branch"];
	        this.detached = source["detached"];
	        this.files = this.convertValues(source["files"], GitChange);
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
	export class GitFileDiff {
	    text: string;
	    binary: boolean;
	    truncated: boolean;
	    untracked: boolean;

	    static createFrom(source: any = {}) {
	        return new GitFileDiff(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.text = source["text"];
	        this.binary = source["binary"];
	        this.truncated = source["truncated"];
	        this.untracked = source["untracked"];
	    }
	}
	export class IDEAConfiguration {
	    id: string;
	    name: string;
	    source: string;
	    kind: string;
	    values: Record<string, any>;
	    warnings: string[];
	    mainClass?: string;

	    static createFrom(source: any = {}) {
	        return new IDEAConfiguration(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.source = source["source"];
	        this.kind = source["kind"];
	        this.values = source["values"];
	        this.warnings = source["warnings"];
	        this.mainClass = source["mainClass"];
	    }
	}
	export class IDEAImport {
	    directory: string;
	    configurations: IDEAConfiguration[];
	    warnings: string[];

	    static createFrom(source: any = {}) {
	        return new IDEAImport(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.directory = source["directory"];
	        this.configurations = this.convertValues(source["configurations"], IDEAConfiguration);
	        this.warnings = source["warnings"];
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
	export class LogLine {
	    time: string;
	    source: string;
	    level: string;
	    text: string;
	    attempt?: number;
	
	    static createFrom(source: any = {}) {
	        return new LogLine(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.time = source["time"];
	        this.source = source["source"];
	        this.level = source["level"];
	        this.text = source["text"];
	        this.attempt = source["attempt"];
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
	    attempt?: number;
	    recovery?: string;
	
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
	        this.attempt = source["attempt"];
	        this.recovery = source["recovery"];
	    }
	}
	export class TerminalInfo {
	    id: string;
	    projectId: string;
	    serviceId: string;
	    title: string;
	    directory: string;
	    shell: string;
	    state: string;
	    pid: number;
	    exitCode?: number;

	    static createFrom(source: any = {}) {
	        return new TerminalInfo(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.projectId = source["projectId"];
	        this.serviceId = source["serviceId"];
	        this.title = source["title"];
	        this.directory = source["directory"];
	        this.shell = source["shell"];
	        this.state = source["state"];
	        this.pid = source["pid"];
	        this.exitCode = source["exitCode"];
	    }
	}
	export class TerminalSnapshot {
	    info: TerminalInfo;
	    sequence: number;
	    data: string;
	    truncated: boolean;

	    static createFrom(source: any = {}) {
	        return new TerminalSnapshot(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.info = this.convertValues(source["info"], TerminalInfo);
	        this.sequence = source["sequence"];
	        this.data = source["data"];
	        this.truncated = source["truncated"];
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

