package main

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type launchPlan struct {
	Build          *commandSpec
	BuildDirectory string
	Next           func() (commandSpec, error)
	Temporary      []string
	Cache          *mavenBuildCache
}

type mavenPOM struct {
	ArtifactID string `xml:"artifactId"`
	Version    string `xml:"version"`
	Packaging  string `xml:"packaging"`
	Parent     struct {
		Version string `xml:"version"`
	} `xml:"parent"`
	Modules    []string `xml:"modules>module"`
	Properties struct {
		Items []struct {
			XMLName xml.Name
			Value   string `xml:",chardata"`
		} `xml:",any"`
	} `xml:"properties"`
	Build struct {
		Directory string `xml:"directory"`
		FinalName string `xml:"finalName"`
		Plugins   []struct {
			ArtifactID string   `xml:"artifactId"`
			Goals      []string `xml:"executions>execution>goals>goal"`
		} `xml:"plugins>plugin"`
	} `xml:"build"`
}

func readMavenPOM(directory string) (mavenPOM, error) {
	var pom mavenPOM
	data, err := os.ReadFile(filepath.Join(directory, "pom.xml"))
	if err == nil {
		err = xml.Unmarshal(data, &pom)
	}
	if err != nil {
		return pom, fmt.Errorf("读取 Maven 项目失败 (%s): %w", directory, err)
	}
	return pom, nil
}

// Only follow explicitly declared modules, never adjacent worktrees.
func mavenModules(root string) ([]string, error) {
	var modules []string
	seen := map[string]bool{}
	var visit func(string) error
	visit = func(dir string) error {
		key := strings.ToLower(filepath.Clean(dir))
		if seen[key] {
			return nil
		}
		seen[key] = true
		pom, err := readMavenPOM(dir)
		if err != nil {
			return err
		}
		modules = append(modules, dir)
		for _, name := range pom.Modules {
			child, err := safeModule(root, filepath.Join(mustRelative(root, dir), strings.TrimSpace(name)))
			if err != nil {
				return err
			}
			if err := visit(child); err != nil {
				return err
			}
		}
		return nil
	}
	err := visit(root)
	return modules, err
}

func mustRelative(root, path string) string {
	rel, _ := filepath.Rel(root, path)
	return rel
}

func bootModules(root string) []string {
	modules, err := mavenModules(root)
	if err != nil {
		return nil
	}
	var candidates []string
	for _, dir := range modules {
		pom, _ := readMavenPOM(dir)
		for _, plugin := range pom.Build.Plugins {
			if plugin.ArtifactID != "spring-boot-maven-plugin" {
				continue
			}
			for _, goal := range plugin.Goals {
				if goal == "repackage" {
					candidates = append(candidates, mustRelative(root, dir))
					break
				}
			}
		}
	}
	return candidates
}

func mavenLayout(p Project) (root, module string, err error) {
	module, err = filepath.Abs(p.Directory)
	if err != nil {
		return
	}
	if p.Module != "" {
		module, err = safeModule(module, p.Module)
		if err != nil {
			return
		}
	}
	pom, err := readMavenPOM(module)
	if err != nil {
		return "", "", err
	}
	if pom.Packaging == "pom" {
		candidates := bootModules(module)
		if len(candidates) != 1 {
			return "", "", fmt.Errorf("请选择启动模块；检测到 %d 个可执行模块: %s", len(candidates), strings.Join(candidates, ", "))
		}
		module = filepath.Join(module, candidates[0])
	}
	root = module
	for candidate := module; ; candidate = filepath.Dir(candidate) {
		if exists(filepath.Join(candidate, "pom.xml")) {
			modules, scanErr := mavenModules(candidate)
			if scanErr != nil {
				return "", "", scanErr
			}
			for _, dir := range modules {
				if strings.EqualFold(dir, module) {
					root = candidate
					break
				}
			}
		}
		if exists(filepath.Join(candidate, ".git")) || filepath.Dir(candidate) == candidate {
			break
		}
	}
	return root, module, nil
}

func buildLaunchPlan(p Project, clean bool) (launchPlan, error) {
	if p.Kind != "spring-maven" {
		spec, err := buildCommand(p)
		return launchPlan{Next: func() (commandSpec, error) { return spec, nil }, Temporary: spec.Temporary}, err
	}
	root, module, err := mavenLayout(p)
	if err != nil {
		return launchPlan{}, err
	}
	// Reuse environment/tool selection without running the single-module goal.
	spec, err := buildCommand(p)
	if err != nil {
		return launchPlan{}, err
	}
	spec.Directory = root
	spec.Args = []string{"-B", "-ntp", "-f", filepath.Join(root, "pom.xml")}
	if !strings.EqualFold(root, module) {
		spec.Args = append(spec.Args, "-pl", filepath.ToSlash(mustRelative(root, module)), "-am")
	}
	if clean {
		spec.Args = append(spec.Args, "clean")
	}
	spec.Args = append(spec.Args, "package", "-Dmaven.test.skip=true", "-Dstyle.color=never")
	return launchPlan{Build: &spec, BuildDirectory: root, Cache: newMavenBuildCache(root, module, spec, clean), Next: func() (commandSpec, error) {
		jar, err := mavenArtifact(root, module)
		if err != nil {
			return commandSpec{}, err
		}
		java := "java.exe"
		if home := environmentValue(spec.Env, "JAVA_HOME"); home != "" {
			java = filepath.Join(home, "bin", "java.exe")
		}
		args := append([]string{}, splitLines(p.JVMArgs)...)
		args = append(args, fmt.Sprintf("-Dserver.port=%d", p.Port))
		if p.ConfigFile != "" {
			args = append(args, "-D"+p.ConfigProperty+"="+p.ConfigFile)
		}
		args = append(args, "-jar", jar)
		args = append(args, splitLines(p.AppArgs)...)
		return commandSpec{Executable: java, Args: args, Directory: root, Env: spec.Env}, nil
	}}, nil
}

func mavenArtifact(root, module string) (string, error) {
	jar, err := mavenArtifactPath(root, module)
	if err != nil {
		return "", err
	}
	reader, err := zip.OpenReader(jar)
	if err != nil {
		return "", fmt.Errorf("构建成功，但无法读取启动产物 %s: %w", jar, err)
	}
	defer reader.Close()
	for _, file := range reader.File {
		if file.Name != "META-INF/MANIFEST.MF" {
			continue
		}
		stream, err := file.Open()
		if err != nil {
			return "", err
		}
		manifest, err := io.ReadAll(io.LimitReader(stream, 1024*1024))
		stream.Close()
		if err != nil {
			return "", err
		}
		// Manifest continuation lines start with a space.
		text := strings.ReplaceAll(strings.ReplaceAll(string(manifest), "\r\n", "\n"), "\n ", "")
		for _, line := range strings.Split(text, "\n") {
			if strings.HasPrefix(line, "Main-Class:") && strings.TrimSpace(strings.TrimPrefix(line, "Main-Class:")) != "" {
				return jar, nil
			}
		}
	}
	return "", fmt.Errorf("%s 不是可执行 JAR；请在启动模块配置 spring-boot-maven-plugin 的 repackage", jar)
}

func mavenArtifactPath(root, module string) (string, error) {
	var poms []mavenPOM
	for dir := module; ; dir = filepath.Dir(dir) {
		if exists(filepath.Join(dir, "pom.xml")) {
			pom, err := readMavenPOM(dir)
			if err != nil {
				return "", err
			}
			poms = append(poms, pom)
		}
		if strings.EqualFold(dir, root) || filepath.Dir(dir) == dir {
			break
		}
	}
	pom := poms[0]
	if pom.Packaging != "" && pom.Packaging != "jar" {
		return "", fmt.Errorf("自动构建启动目前需要 JAR 模块，当前 packaging=%s", pom.Packaging)
	}
	props := map[string]string{}
	directory, name := "target", "${project.artifactId}-${project.version}"
	for i := len(poms) - 1; i >= 0; i-- {
		for _, property := range poms[i].Properties.Items {
			props[property.XMLName.Local] = strings.TrimSpace(property.Value)
		}
		if poms[i].Build.Directory != "" {
			directory = poms[i].Build.Directory
		}
		if poms[i].Build.FinalName != "" {
			name = poms[i].Build.FinalName
		}
	}
	version := pom.Version
	if version == "" {
		version = pom.Parent.Version
	}
	props["project.artifactId"], props["artifactId"] = pom.ArtifactID, pom.ArtifactID
	props["project.version"], props["version"] = version, version
	props["project.basedir"], props["basedir"] = module, module
	expand := func(value string) (string, error) {
		for i := 0; i < 12 && strings.Contains(value, "${"); i++ {
			for key, replacement := range props {
				value = strings.ReplaceAll(value, "${"+key+"}", replacement)
			}
		}
		if strings.Contains(value, "${") {
			return "", fmt.Errorf("无法解析 Maven 产物路径 %q；请在模块 POM 中明确配置 build.directory / finalName", value)
		}
		return strings.TrimSpace(value), nil
	}
	directory, err := expand(directory)
	if err != nil {
		return "", err
	}
	name, err = expand(name)
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(directory) {
		directory = filepath.Join(module, directory)
	}
	return filepath.Join(directory, name+".jar"), nil
}
