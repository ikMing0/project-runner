package main

import (
	"encoding/xml"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

type IDEAConfiguration struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Source    string         `json:"source"`
	Kind      string         `json:"kind"`
	Values    map[string]any `json:"values"`
	Warnings  []string       `json:"warnings"`
	MainClass string         `json:"mainClass,omitempty"`
}

type IDEAImport struct {
	Directory      string              `json:"directory"`
	Configurations []IDEAConfiguration `json:"configurations"`
	Warnings       []string            `json:"warnings"`
}

type ideaXML struct {
	XMLName  xml.Name
	Attrs    []xml.Attr `xml:",any,attr"`
	Children []ideaXML  `xml:",any"`
}

func (n ideaXML) attr(name string) string {
	for _, a := range n.Attrs {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

func (n ideaXML) child(name string) ideaXML {
	for _, c := range n.Children {
		if c.XMLName.Local == name {
			return c
		}
	}
	return ideaXML{}
}

func (n ideaXML) option(name string) string {
	for _, c := range n.Children {
		if c.XMLName.Local == "option" && c.attr("name") == name {
			return c.attr("value")
		}
	}
	return ""
}

func (n ideaXML) nodes(name string) []ideaXML {
	var result []ideaXML
	if n.XMLName.Local == name {
		result = append(result, n)
	}
	for _, c := range n.Children {
		result = append(result, c.nodes(name)...)
	}
	return result
}

func readIDEAXML(path string) (ideaXML, error) {
	info, err := os.Stat(path)
	if err != nil {
		return ideaXML{}, err
	}
	if info.Size() > 8*1024*1024 {
		return ideaXML{}, errors.New("配置文件超过 8 MiB")
	}
	data, err := os.ReadFile(path)
	var root ideaXML
	if err == nil {
		err = xml.Unmarshal(data, &root)
	}
	return root, err
}

func ideaRoot(directory string) (string, error) {
	abs, err := filepath.Abs(directory)
	if strings.TrimSpace(directory) == "" || err != nil {
		return "", errors.New("请先选择项目目录")
	}
	info, err := os.Stat(abs)
	if err != nil || !info.IsDir() {
		return "", errors.New("项目目录不存在")
	}
	for dir := filepath.Clean(abs); ; dir = filepath.Dir(dir) {
		if exists(filepath.Join(dir, ".idea")) || exists(filepath.Join(dir, ".run")) {
			return dir, nil
		}
		if exists(filepath.Join(dir, ".git")) || filepath.Dir(dir) == dir {
			return filepath.Clean(abs), nil
		}
	}
}

// Reads configuration data only: IDEA's Make/before-launch tasks are never run.
func (a *App) ReadIDEAConfigurations(directory string) (IDEAImport, error) {
	a.mu.Lock()
	previous := a.orderedProjectsLocked()
	a.mu.Unlock()
	return readIDEAConfigurations(directory, loadIDEATools(previous))
}

func readIDEAConfigurations(directory string, tools ideaTools) (IDEAImport, error) {
	root, err := ideaRoot(directory)
	result := IDEAImport{Directory: root, Configurations: []IDEAConfiguration{}, Warnings: []string{}}
	if err != nil {
		return result, err
	}
	misc, _ := readIDEAXML(filepath.Join(root, ".idea", "misc.xml"))
	sdk := ""
	for _, component := range misc.nodes("component") {
		if component.attr("name") == "ProjectRootManager" {
			sdk = component.attr("project-jdk-name")
		}
	}
	// Shared files win over stale local workspace copies of the same run config.
	var files []string
	for _, pattern := range []string{".run/*.run.xml", ".idea/runConfigurations/*.xml"} {
		matches, _ := filepath.Glob(filepath.Join(root, filepath.FromSlash(pattern)))
		sort.Strings(matches)
		files = append(files, matches...)
	}
	files = append(files, filepath.Join(root, ".idea", "workspace.xml"))
	seen := map[string]bool{}
	for _, path := range files {
		document, readErr := readIDEAXML(path)
		if os.IsNotExist(readErr) {
			continue
		}
		relative, _ := filepath.Rel(root, path)
		source := filepath.ToSlash(relative)
		if readErr != nil {
			result.Warnings = append(result.Warnings, source+" 无法读取："+readErr.Error())
			continue
		}
		for _, config := range document.nodes("configuration") {
			if config.attr("default") == "true" || config.attr("name") == "" {
				continue
			}
			key := config.attr("type") + "\x00" + config.attr("name")
			if seen[key] {
				continue
			}
			seen[key] = true
			candidates := parseIDEAConfiguration(config, root, source, sdk, tools)
			result.Configurations = append(result.Configurations, candidates...)
		}
	}
	return result, nil
}

func ideaArguments(value string) ([]string, error) {
	var args []string
	var token strings.Builder
	var quote rune
	started := false
	runes := []rune(value)
	for i := 0; i < len(runes); i++ {
		c := runes[i]
		if c == '\\' && i+1 < len(runes) && (runes[i+1] == '"' || runes[i+1] == '\'') {
			token.WriteRune(runes[i+1])
			started = true
			i++
		} else if quote != 0 {
			if c == quote {
				quote = 0
			} else {
				token.WriteRune(c)
			}
		} else if c == '"' || c == '\'' {
			quote, started = c, true
		} else if unicode.IsSpace(c) {
			if started {
				args = append(args, token.String())
				token.Reset()
				started = false
			}
		} else {
			token.WriteRune(c)
			started = true
		}
	}
	if quote != 0 {
		return nil, errors.New("参数中存在未闭合引号，请手动检查")
	}
	if started {
		args = append(args, token.String())
	}
	return args, nil
}

func ideaPort(value string) int {
	port, _ := strconv.Atoi(value)
	if port < 1 || port > 65535 {
		return 0
	}
	return port
}

func ideaEnvironment(config ideaXML) map[string]string {
	env := map[string]string{}
	for _, entry := range config.child("envs").Children {
		if entry.XMLName.Local == "env" && entry.attr("name") != "" {
			env[entry.attr("name")] = entry.attr("value")
		}
	}
	return env
}

func parseIDEAConfiguration(config ideaXML, root, source, sdk string, tools ideaTools) []IDEAConfiguration {
	typeName := config.attr("type")
	if typeName != "SpringBootApplicationConfigurationType" && typeName != "js.build_tools.npm" {
		return nil
	}
	c := IDEAConfiguration{ID: source + "|" + typeName + "|" + config.attr("name"), Name: config.attr("name"),
		Source: source, Values: map[string]any{}, Warnings: []string{}}
	env := ideaEnvironment(config)
	for key, value := range env {
		env[key] = tools.expand(value, root)
	}
	c.Values["environment"] = env
	moduleRoot := root
	arguments := func(value string) []string {
		args, err := ideaArguments(value)
		if err != nil {
			c.Warnings = append(c.Warnings, err.Error())
		}
		for i := range args {
			args[i] = tools.expand(strings.NewReplacer("$MODULE_DIR$", moduleRoot, "$MODULE_WORKING_DIR$", moduleRoot).Replace(args[i]), root)
		}
		return args
	}
	if typeName == "SpringBootApplicationConfigurationType" {
		c.Kind = "spring-maven"
		if exists(filepath.Join(root, "build.gradle")) || exists(filepath.Join(root, "build.gradle.kts")) {
			c.Kind = "spring-gradle"
		}
		c.Values["kind"], c.Values["directory"] = c.Kind, root
		module := config.child("module").attr("name")
		modulePath := ideaModulePath(root, module)
		if modulePath != "" {
			c.Values["module"] = modulePath
			moduleRoot = filepath.Join(root, modulePath)
		} else if module != "" && module != filepath.Base(root) {
			c.Warnings = append(c.Warnings, "IDEA 模块 "+module+" 未定位到构建目录，请确认模块")
		}
		c.MainClass = config.option("SPRING_BOOT_MAIN_CLASS")
		for key, value := range env {
			if strings.EqualFold(key, "SERVER_PORT") {
				if port := ideaPort(value); port != 0 {
					c.Values["port"] = port
				} else {
					c.Warnings = append(c.Warnings, "IDEA 后端环境变量端口无效，使用历史端口建议")
				}
				delete(env, key)
			}
		}
		var jvm, app []string
		for _, arg := range arguments(config.option("VM_PARAMETERS")) {
			switch {
			case strings.HasPrefix(arg, "-Dserver.port="):
				if port := ideaPort(strings.TrimPrefix(arg, "-Dserver.port=")); port != 0 {
					c.Values["port"] = port
				} else {
					c.Warnings = append(c.Warnings, "IDEA 后端端口无效，使用历史端口建议")
				}
			case strings.HasPrefix(arg, "-Dapplication.config.path="):
				path := tools.path(strings.TrimPrefix(arg, "-Dapplication.config.path="), root)
				c.Values["configProperty"] = "application.config.path"
				if exists(path) {
					c.Values["configFile"] = path
				} else {
					c.Warnings = append(c.Warnings, "外部配置路径不存在："+path+"，请选择配置文件")
				}
			default:
				jvm = append(jvm, arg)
			}
		}
		appArgs := arguments(config.option("PROGRAM_PARAMETERS"))
		for i := 0; i < len(appArgs); i++ {
			arg := appArgs[i]
			value, isPort := strings.CutPrefix(arg, "--server.port=")
			if arg == "--server.port" && i+1 < len(appArgs) {
				i++
				value, isPort = appArgs[i], true
			}
			if isPort {
				if port := ideaPort(value); port != 0 {
					c.Values["port"] = port
				}
			} else {
				app = append(app, arg)
			}
		}
		c.Values["jvmArgs"], c.Values["appArgs"] = strings.Join(jvm, "\n"), strings.Join(app, "\n")
		if config.option("ALTERNATIVE_JRE_PATH_ENABLED") == "true" {
			sdk = config.option("ALTERNATIVE_JRE_PATH")
		}
		java := tools.java(sdk, root)
		if java != "" {
			c.Values["javaHome"] = java
		} else if sdk != "" {
			c.Warnings = append(c.Warnings, "未找到 IDEA JDK "+sdk+" 的有效路径，请复用或选择 JDK")
		}
		if tool := tools.buildTool(root, c.Kind); tool != "" {
			c.Values["toolPath"] = tool
		} else if !exists(filepath.Join(root, "mvnw.cmd")) && c.Kind == "spring-maven" {
			c.Warnings = append(c.Warnings, "未记录 Maven 路径，将使用已有配置或系统 PATH")
		}
		return []IDEAConfiguration{c}
	}
	if command := config.child("command").attr("value"); command != "" && command != "run" {
		return nil
	}
	c.Kind = "node"
	pkgPath := tools.path(config.child("package-json").attr("value"), root)
	directory := filepath.Dir(pkgPath)
	c.Values["kind"] = c.Kind
	if pkgPath == "" || !exists(pkgPath) {
		c.Warnings = append(c.Warnings, "IDEA 前端 package.json 不存在，请选择前端目录")
	} else {
		c.Values["directory"] = directory
	}
	detection, _ := (&App{}).DetectProject(directory)
	manager := detection.PackageManager
	if manager == "" {
		manager = "npm"
	}
	c.Values["packageManager"], c.Values["portMode"] = manager, detection.PortMode
	if node := tools.node(config.child("node-interpreter").attr("value"), root); node != "" {
		c.Values["nodeHome"] = filepath.Dir(node)
		if executable := filepath.Join(filepath.Dir(node), manager+".cmd"); exists(executable) {
			c.Values["toolPath"] = executable
		}
	}
	if explicit := tools.path(config.child("package-manager").attr("value"), root); ideaValidFile(explicit) {
		c.Values["toolPath"] = explicit
	}
	if opts := config.child("node-options").attr("value"); opts != "" {
		env["NODE_OPTIONS"] = tools.expand(opts, root)
	}
	for key, value := range env {
		if strings.EqualFold(key, "port") && ideaPort(value) != 0 {
			c.Values["port"] = ideaPort(value)
			delete(env, key)
			if detection.PortMode == "none" {
				c.Values["portMode"] = "env"
			}
		}
	}
	var args []string
	rawArgs := arguments(config.child("arguments").attr("value"))
	for i := 0; i < len(rawArgs); i++ {
		value, isPort := strings.CutPrefix(rawArgs[i], "--port=")
		if rawArgs[i] == "--port" && i+1 < len(rawArgs) {
			i++
			value, isPort = rawArgs[i], true
		}
		if isPort {
			if port := ideaPort(value); port != 0 {
				c.Values["port"] = port
			}
		} else {
			args = append(args, rawArgs[i])
		}
	}
	c.Values["appArgs"] = strings.Join(args, "\n")
	var candidates []IDEAConfiguration
	for _, script := range config.child("scripts").Children {
		if name := script.attr("value"); script.XMLName.Local == "script" && name != "" {
			item := c
			item.ID += "|" + name
			item.Values = map[string]any{}
			for key, value := range c.Values {
				item.Values[key] = value
			}
			item.Values["script"] = name
			if len(config.child("scripts").Children) > 1 {
				item.Name += " · " + name
			}
			found := false
			for _, known := range detection.Scripts {
				found = found || known == name
			}
			if !found {
				item.Warnings = append(append([]string{}, c.Warnings...), "package.json 中没有脚本 "+name+"，请确认")
			}
			candidates = append(candidates, item)
		}
	}
	return candidates
}

func ideaModulePath(root, name string) string {
	if name == "" || name == filepath.Base(root) {
		return ""
	}
	candidate := filepath.Join(root, name)
	relative, err := filepath.Rel(root, candidate)
	if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) &&
		(exists(filepath.Join(candidate, "pom.xml")) || exists(filepath.Join(candidate, "build.gradle")) || exists(filepath.Join(candidate, "build.gradle.kts"))) {
		return relative
	}
	modules, _ := mavenModules(root)
	for _, module := range modules {
		if filepath.Base(module) == name {
			path, _ := filepath.Rel(root, module)
			return path
		}
	}
	return ""
}

func ideaValidFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
