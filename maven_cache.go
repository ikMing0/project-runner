package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const mavenCacheVersion = 1

type mavenBuildRecord struct {
	Version     int      `json:"version"`
	Root        string   `json:"root"`
	Module      string   `json:"module"`
	Inputs      string   `json:"inputs"`
	Jar         string   `json:"jar"`
	JarHash     string   `json:"jarHash"`
	SourceFiles []string `json:"sourceFiles"`
}

type mavenBuildCache struct {
	root, module, path string
	spec               commandSpec
	force              bool
	inputs             string
	sourceFiles        []string
	cleanRequired      bool
}

func newMavenBuildCache(root, module string, spec commandSpec, force bool) *mavenBuildCache {
	key := sha256.Sum256([]byte(strings.ToLower(filepath.Clean(module))))
	return &mavenBuildCache{root: root, module: module, spec: spec, force: force,
		path: filepath.Join(root, "target", ".project-runner", hex.EncodeToString(key[:16])+".json")}
}

// A JAR is reusable only when a successful build recorded these exact inputs
// and its content. Existing artifacts from IDEA or a failed build are untrusted.
func (c *mavenBuildCache) needsBuild() (bool, string) {
	inputs, err := c.fingerprint()
	if err != nil {
		return true, "无法完整核验构建输入，执行构建"
	}
	c.inputs = inputs
	if c.force {
		return true, "手动重新构建，清理后构建"
	}
	data, err := os.ReadFile(c.path)
	if err != nil {
		return true, "尚无成功构建记录，执行首次构建"
	}
	var record mavenBuildRecord
	if json.Unmarshal(data, &record) != nil || record.Version != mavenCacheVersion || record.Root != c.root || record.Module != c.module {
		return true, "构建记录无效，重新构建"
	}
	if record.Inputs != inputs {
		current := map[string]bool{}
		for _, file := range c.sourceFiles {
			current[file] = true
		}
		for _, file := range record.SourceFiles {
			if !current[file] {
				c.cleanRequired = true
				return true, "源码或资源被删除/重命名，清理后重新构建"
			}
		}
		return true, "源码、资源或构建配置有变化，自动构建"
	}
	jar, err := mavenArtifact(c.root, c.module)
	if err != nil {
		return true, "启动 JAR 缺失或无效，重新构建"
	}
	digest, err := fileDigest(jar)
	if err != nil || record.Jar != jar || record.JarHash != digest {
		return true, "启动 JAR 被替换或修改，重新构建"
	}
	return false, "构建输入未变化，复用当前工作树 JAR，跳过 Maven 构建"
}

func (c *mavenBuildCache) invalidate() error {
	err := os.Remove(c.path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// Recheck after packaging: edits made while Maven runs must not certify an
// artifact as up to date for a later launch. A cache write never gates a good run.
func (c *mavenBuildCache) record() error {
	jar, err := mavenArtifact(c.root, c.module)
	if err != nil {
		return err
	}
	inputs, err := c.fingerprint()
	if err != nil {
		return err
	}
	if c.inputs == "" || inputs != c.inputs {
		return fmt.Errorf("构建期间输入有变化，本次不保存复用记录")
	}
	digest, err := fileDigest(jar)
	if err != nil {
		return err
	}
	data, err := json.Marshal(mavenBuildRecord{Version: mavenCacheVersion, Root: c.root, Module: c.module, Inputs: inputs, Jar: jar, JarHash: digest, SourceFiles: c.sourceFiles})
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(c.path), "build-record-*.tmp")
	if err != nil {
		return err
	}
	temporary := file.Name()
	defer os.Remove(temporary)
	_, err = file.Write(data)
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(temporary, c.path)
}

var pomEnvironmentRef = regexp.MustCompile(`\$\{env\.([A-Za-z_][A-Za-z0-9_]*)\}`)
var batchEnvironmentRef = regexp.MustCompile(`%([A-Za-z_][A-Za-z0-9_]*)%`)

func (c *mavenBuildCache) fingerprint() (string, error) {
	modules, err := mavenModules(c.root)
	if err != nil {
		return "", err
	}
	files := map[string]bool{}
	sources := map[string]bool{}
	for _, module := range modules {
		files[filepath.Join(module, "pom.xml")] = true
		if err := collectBuildFiles(filepath.Join(module, "src", "main"), sources); err != nil {
			return "", err
		}
	}
	c.sourceFiles = nil
	for source := range sources {
		files[source] = true
		c.sourceFiles = append(c.sourceFiles, source)
	}
	sort.Strings(c.sourceFiles)
	if err := collectBuildFiles(filepath.Join(c.root, ".mvn"), files); err != nil {
		return "", err
	}
	for _, name := range []string{"mvnw", "mvnw.cmd", "settings.xml"} {
		files[filepath.Join(c.root, name)] = true
	}
	// Track the selected tool/JDK, Maven's own settings, and the user's settings.
	// Runtime JVM/app arguments, ports and external application config are absent.
	tool := c.spec.Executable
	if !strings.ContainsAny(tool, `/\`) {
		tool, err = commandOnPath(tool, c.spec.Env)
		if err != nil {
			return "", err
		}
	}
	files[tool] = true
	for _, name := range []string{"settings.xml", "toolchains.xml"} {
		files[filepath.Join(filepath.Dir(tool), "..", "conf", name)] = true
	}
	java := "java.exe"
	if home := environmentValue(c.spec.Env, "JAVA_HOME"); home != "" {
		java = filepath.Join(home, "bin", "java.exe")
	} else {
		java, err = commandOnPath(java, c.spec.Env)
		if err != nil {
			return "", err
		}
	}
	files[java] = true
	javaHome := filepath.Dir(filepath.Dir(java))
	files[filepath.Join(javaHome, "release")] = true
	files[filepath.Join(javaHome, "bin", "javac.exe")] = true
	userMaven := environmentValue(c.spec.Env, "MAVEN_USER_HOME")
	if userMaven == "" {
		userHome := environmentValue(c.spec.Env, "USERPROFILE")
		if userHome == "" {
			userHome, _ = os.UserHomeDir()
		}
		userMaven = filepath.Join(userHome, ".m2")
	}
	for _, name := range []string{"settings.xml", "toolchains.xml"} {
		files[filepath.Join(userMaven, name)] = true
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, filepath.Clean(name))
	}
	sort.Strings(names)
	digest := sha256.New()
	hashField(digest, c.root)
	hashField(digest, c.module)
	variables := map[string]bool{}
	for _, key := range []string{"JAVA_HOME", "MAVEN_HOME", "M2_HOME", "MAVEN_USER_HOME", "MAVEN_OPTS", "MAVEN_ARGS", "JAVA_TOOL_OPTIONS", "JDK_JAVA_OPTIONS"} {
		variables[key] = true
	}
	for _, name := range names {
		hashField(digest, name)
		data, err := os.ReadFile(name)
		if os.IsNotExist(err) {
			hashField(digest, "missing")
			continue
		}
		if err != nil {
			return "", err
		}
		hashField(digest, "file")
		sum := sha256.Sum256(data)
		digest.Write(sum[:])
		// Only variables consumed by the build invalidate reuse. Runtime-only
		// environment changes (DB/port settings, for example) don't rebuild code.
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".exe" && ext != ".jar" && ext != ".class" {
			for _, match := range pomEnvironmentRef.FindAllSubmatch(data, -1) {
				variables[strings.ToUpper(string(match[1]))] = true
			}
			if ext == ".cmd" || ext == ".bat" {
				for _, match := range batchEnvironmentRef.FindAllSubmatch(data, -1) {
					variables[strings.ToUpper(string(match[1]))] = true
				}
			}
		}
	}
	keys := make([]string, 0, len(variables))
	for key := range variables {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		hashField(digest, key)
		hashField(digest, environmentValue(c.spec.Env, key))
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func collectBuildFiles(root string, files map[string]bool) error {
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			info, err := os.Stat(path)
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("无法核验特殊构建输入: %s", path)
			}
			files[path] = true
		}
		return nil
	})
}

func hashField(digest hash.Hash, value string) {
	fmt.Fprintf(digest, "%d:", len(value))
	io.WriteString(digest, value)
}

func fileDigest(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}
