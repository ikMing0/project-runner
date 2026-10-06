package main

import (
	"regexp"
	"strings"
	"sync"
)

// Observe only this application's launch output, not Maven output or earlier
// attempts. Mapper context plus a missing class avoids retrying ordinary
// database, Redis, configuration and XML syntax failures.
type mavenArtifactFailure struct {
	mu      sync.Mutex
	mapper  bool
	missing string
	corrupt bool
}

var missingArtifactClass = regexp.MustCompile(`(?:java\.lang\.)?(?:ClassNotFoundException|NoClassDefFoundError):\s*(?:Cannot find class:\s*)?([A-Za-z_$][\w$]*(?:[./][\w$]+)+)`)

func (d *mavenArtifactFailure) observe(line string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, marker := range []string{"Error parsing Mapper XML", "Could not resolve type alias", "Failed to parse mapping resource"} {
		if strings.Contains(line, marker) {
			d.mapper = true
		}
	}
	if match := missingArtifactClass.FindStringSubmatch(line); len(match) > 1 {
		d.missing = strings.ReplaceAll(match[1], "/", ".")
	}
	if strings.Contains(line, "java.util.zip.ZipException") &&
		(strings.Contains(line, "invalid LOC header") || strings.Contains(line, "zip END header not found") || strings.Contains(line, "error in opening zipfile")) {
		d.corrupt = true
	}
}

func (d *mavenArtifactFailure) reason() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.mapper && d.missing != "" {
		return "Mapper 引用了缺失的类 " + d.missing + "，疑似旧编译产物或资源残留"
	}
	if d.corrupt {
		return "启动依赖 JAR 损坏，无法读取压缩内容"
	}
	return ""
}

func mavenCleanCommand(spec commandSpec) commandSpec {
	spec.Args = append([]string{}, spec.Args...)
	for i := len(spec.Args) - 1; i >= 0; i-- {
		if spec.Args[i] == "package" {
			if i == 0 || spec.Args[i-1] != "clean" {
				spec.Args = append(append(spec.Args[:i:i], "clean"), spec.Args[i:]...)
			}
			break
		}
	}
	return spec
}

func mavenCommandIsClean(spec *commandSpec) bool {
	if spec == nil {
		return false
	}
	for i := len(spec.Args) - 1; i > 0; i-- {
		if spec.Args[i] == "package" {
			return spec.Args[i-1] == "clean"
		}
	}
	return false
}
