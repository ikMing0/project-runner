package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type ideaTools struct {
	userHome string
	sdks     map[string]string
	nodePath string
	maven    string
	previous []Project
}

func loadIDEATools(previous []Project) ideaTools {
	home, _ := os.UserHomeDir()
	result := ideaTools{userHome: home, sdks: map[string]string{}, previous: previous}
	config, _ := os.UserConfigDir()
	var directories []string
	for _, pattern := range []string{"IntelliJIdea*", "IdeaIC*"} {
		matches, _ := filepath.Glob(filepath.Join(config, "JetBrains", pattern))
		directories = append(directories, matches...)
	}
	sort.SliceStable(directories, func(i, j int) bool {
		left, le := os.Stat(directories[i])
		right, re := os.Stat(directories[j])
		if le != nil || re != nil {
			return directories[i] > directories[j]
		}
		return left.ModTime().After(right.ModTime())
	})
	for _, directory := range directories {
		readIDEAToolDirectory(&result, filepath.Join(directory, "options"))
	}
	return result
}

func readIDEAToolDirectory(result *ideaTools, directory string) {
	table, _ := readIDEAXML(filepath.Join(directory, "jdk.table.xml"))
	for _, jdk := range table.nodes("jdk") {
		name := jdk.child("name").attr("value")
		path := result.path(jdk.child("homePath").attr("value"), "")
		if name != "" && result.sdks[name] == "" && ideaValidFile(filepath.Join(path, "bin", "java.exe")) {
			result.sdks[name] = path
		}
	}
	nodes, _ := readIDEAXML(filepath.Join(directory, "nodejs.xml"))
	for _, interpreter := range nodes.nodes("local-interpreter") {
		path := result.path(interpreter.attr("path"), "")
		if result.nodePath == "" && ideaValidFile(path) {
			result.nodePath = path
		}
	}
	for _, name := range []string{"maven.xml", "project.default.xml", "other.xml"} {
		settings, _ := readIDEAXML(filepath.Join(directory, name))
		for _, option := range settings.nodes("option") {
			if option.attr("name") == "mavenHome" && result.maven == "" {
				home := result.path(option.attr("value"), "")
				if executable := filepath.Join(home, "bin", "mvn.cmd"); ideaValidFile(executable) {
					result.maven = executable
				}
			}
		}
	}
}

func (t ideaTools) expand(value, root string) string {
	return strings.NewReplacer("$PROJECT_DIR$", root, "$USER_HOME$", t.userHome,
		"$MODULE_DIR$", root, "$MODULE_WORKING_DIR$", root).Replace(value)
}

func (t ideaTools) path(value, root string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	value = t.expand(strings.TrimSpace(value), root)
	value = strings.TrimPrefix(value, "file://")
	if strings.Contains(value, "$") || strings.ContainsRune(value, '\x00') {
		return ""
	}
	value = filepath.FromSlash(value)
	if !filepath.IsAbs(value) {
		if root == "" {
			return ""
		}
		value = filepath.Join(root, value)
	}
	return filepath.Clean(value)
}

func (t ideaTools) java(name, root string) string {
	if home := t.sdks[name]; ideaValidFile(filepath.Join(home, "bin", "java.exe")) {
		return home
	}
	if home := t.path(name, root); ideaValidFile(filepath.Join(home, "bin", "java.exe")) {
		return home
	}
	if name != "" {
		return ""
	}
	for _, p := range t.previous {
		if ideaValidFile(filepath.Join(p.JavaHome, "bin", "java.exe")) {
			return p.JavaHome
		}
	}
	if home := os.Getenv("JAVA_HOME"); ideaValidFile(filepath.Join(home, "bin", "java.exe")) {
		return home
	}
	return ""
}

func (t ideaTools) node(value, root string) string {
	if value != "" && value != "project" {
		if path := t.path(value, root); ideaValidFile(path) {
			return path
		}
		return ""
	}
	if ideaValidFile(t.nodePath) {
		return t.nodePath
	}
	if path, err := exec.LookPath("node.exe"); err == nil {
		return path
	}
	return ""
}

func (t ideaTools) buildTool(root, kind string) string {
	wrapper := "mvnw.cmd"
	if kind == "spring-gradle" {
		wrapper = "gradlew.bat"
	}
	if path := filepath.Join(root, wrapper); ideaValidFile(path) {
		return path
	}
	if kind == "spring-maven" {
		for _, file := range []string{".idea/workspace.xml", ".idea/misc.xml"} {
			settings, _ := readIDEAXML(filepath.Join(root, filepath.FromSlash(file)))
			for _, option := range settings.nodes("option") {
				if option.attr("name") == "mavenHome" {
					home := t.path(option.attr("value"), root)
					if path := filepath.Join(home, "bin", "mvn.cmd"); ideaValidFile(path) {
						return path
					}
				}
			}
		}
		if ideaValidFile(t.maven) {
			return t.maven
		}
	}
	for _, p := range t.previous {
		if p.Kind == kind && ideaValidFile(p.ToolPath) {
			return p.ToolPath
		}
	}
	executable := "mvn.cmd"
	if kind == "spring-gradle" {
		executable = "gradle.bat"
	}
	if path, err := exec.LookPath(executable); err == nil {
		return path
	}
	return ""
}
