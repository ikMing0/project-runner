//go:build windows

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func writeCacheInput(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func fixtureCache(t *testing.T, p Project) *mavenBuildCache {
	t.Helper()
	plan, err := buildLaunchPlan(p, false)
	if err != nil {
		t.Fatal(err)
	}
	return plan.Cache
}

func TestUnchangedMavenJarReusedAfterReopeningWithNewRuntimeOptions(t *testing.T) {
	p, root := mavenFixture(t)
	writeCacheInput(t, p.ConfigFile, "external=true")
	a := NewApp()
	a.projects[p.ID] = p
	t.Cleanup(func() { a.shutdown(nil) })
	if err := a.StartProject(p.ID); err != nil {
		t.Fatal(err)
	}
	waitStatus(t, a, p.ID, "running")
	if err := a.StopProject(p.ID); err != nil {
		t.Fatal(err)
	}
	waitStatus(t, a, p.ID, "stopped")
	cache := fixtureCache(t, p)
	if !exists(cache.path) {
		t.Fatal("successful build not recorded")
	}
	if err := os.Remove(p.Environment["RUNNER_TEST_BUILD_ARGS"]); err != nil {
		t.Fatal(err)
	}
	// Frontend, documentation and test edits do not affect the skipped-test JAR.
	writeCacheInput(t, filepath.Join(root, "ruoyi-ui", "src", "main.js"), "frontend changed")
	writeCacheInput(t, filepath.Join(root, "README.md"), "docs changed")
	writeCacheInput(t, filepath.Join(root, "app", "src", "test", "java", "AppTest.java"), "test changed")
	writeCacheInput(t, p.ConfigFile, "external=config changed")
	p.Port = freePort(t)
	p.Environment["RUNNER_TEST_PORT"] = strconv.Itoa(p.Port)
	p.Environment["RUNTIME_ONLY"] = "new runtime value"
	p.JVMArgs = "-Xmx256m"
	p.AppArgs = "--new-runtime-option=true"
	// A new App proves reuse comes from disk, rather than an in-memory flag.
	reopened := NewApp()
	reopened.projects[p.ID] = p
	t.Cleanup(func() { reopened.shutdown(nil) })
	if err := reopened.StartProject(p.ID); err != nil {
		t.Fatal(err)
	}
	waitStatus(t, reopened, p.ID, "running")
	if exists(p.Environment["RUNNER_TEST_BUILD_ARGS"]) {
		t.Fatal("unchanged launch invoked Maven")
	}
	data, err := os.ReadFile(p.Environment["RUNNER_TEST_RUN_ARGS"])
	if err != nil {
		t.Fatal(err)
	}
	var args []string
	if err := json.Unmarshal(data, &args); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(args, " "), "-Xmx256m -Dserver.port="+strconv.Itoa(p.Port)) || args[len(args)-1] != "--new-runtime-option=true" {
		t.Fatalf("runtime options lost: %v", args)
	}
	found := false
	for _, line := range reopened.GetLogs(p.ID) {
		if strings.Contains(line.Text, "跳过 Maven 构建") {
			found = true
		}
	}
	if !found {
		t.Fatal("no explanation of reused JAR in log")
	}
}

func TestSmartMavenCacheDetectsContentAndArtifactChanges(t *testing.T) {
	changes := map[string]func(*testing.T, Project, string, *mavenBuildCache){
		"module source": func(t *testing.T, p Project, root string, c *mavenBuildCache) {
			writeCacheInput(t, filepath.Join(root, "app", "src", "main", "java", "Sample.java"), "class Sample { int value=2; }")
		},
		"dependency source": func(t *testing.T, p Project, root string, c *mavenBuildCache) {
			writeCacheInput(t, filepath.Join(root, "common", "src", "main", "java", "Common.java"), "class Common { int value=2; }")
		},
		"packaged resource": func(t *testing.T, p Project, root string, c *mavenBuildCache) {
			writeCacheInput(t, filepath.Join(root, "app", "src", "main", "resources", "application.yml"), "config: changed")
		},
		"pom": func(t *testing.T, p Project, root string, c *mavenBuildCache) {
			file := filepath.Join(root, "common", "pom.xml")
			data, _ := os.ReadFile(file)
			writeCacheInput(t, file, strings.Replace(string(data), "<version>1</version>", "<version>2</version>", 1))
		},
		"Maven settings": func(t *testing.T, p Project, root string, c *mavenBuildCache) {
			writeCacheInput(t, filepath.Join(root, ".mvn", "maven.config"), "-Pdevelopment")
		},
		"delete source": func(t *testing.T, p Project, root string, c *mavenBuildCache) {
			if err := os.Remove(filepath.Join(root, "app", "src", "main", "java", "Sample.java")); err != nil {
				t.Fatal(err)
			}
		},
		"rename source": func(t *testing.T, p Project, root string, c *mavenBuildCache) {
			dir := filepath.Join(root, "app", "src", "main", "java")
			if err := os.Rename(filepath.Join(dir, "Sample.java"), filepath.Join(dir, "Renamed.java")); err != nil {
				t.Fatal(err)
			}
		},
		"same timestamp changed source": func(t *testing.T, p Project, root string, c *mavenBuildCache) {
			file := filepath.Join(root, "app", "src", "main", "java", "Sample.java")
			info, _ := os.Stat(file)
			writeCacheInput(t, file, "class Sample { int value=9; }")
			if err := os.Chtimes(file, info.ModTime(), info.ModTime()); err != nil {
				t.Fatal(err)
			}
		},
		"missing jar": func(t *testing.T, p Project, root string, c *mavenBuildCache) {
			if err := os.Remove(p.Environment["RUNNER_TEST_JAR"]); err != nil {
				t.Fatal(err)
			}
		},
		"invalid jar": func(t *testing.T, p Project, root string, c *mavenBuildCache) {
			writeCacheInput(t, p.Environment["RUNNER_TEST_JAR"], "corrupt")
		},
		"replaced executable jar": func(t *testing.T, p Project, root string, c *mavenBuildCache) {
			file := p.Environment["RUNNER_TEST_JAR"]
			f, err := os.OpenFile(file, os.O_APPEND|os.O_WRONLY, 0600)
			if err != nil {
				t.Fatal(err)
			}
			f.Write([]byte("changed output"))
			f.Close()
		},
		"corrupt record": func(t *testing.T, p Project, root string, c *mavenBuildCache) { writeCacheInput(t, c.path, "bad json") },
		"missing record": func(t *testing.T, p Project, root string, c *mavenBuildCache) {
			if err := os.Remove(c.path); err != nil {
				t.Fatal(err)
			}
		},
		"JDK version": func(t *testing.T, p Project, root string, c *mavenBuildCache) {
			writeCacheInput(t, filepath.Join(p.JavaHome, "release"), "JAVA_VERSION=22")
		},
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			p, root := mavenFixture(t)
			writeCacheInput(t, filepath.Join(root, "app", "src", "main", "java", "Sample.java"), "class Sample { int value=1; }")
			writeCacheInput(t, filepath.Join(root, "common", "src", "main", "java", "Common.java"), "class Common { int value=1; }")
			c := fixtureCache(t, p)
			if needed, _ := c.needsBuild(); !needed {
				t.Fatal("first launch trusted an unknown artifact")
			}
			if err := writeExecutableJar(p.Environment["RUNNER_TEST_JAR"]); err != nil {
				t.Fatal(err)
			}
			if err := c.record(); err != nil {
				t.Fatal(err)
			}
			if needed, reason := fixtureCache(t, p).needsBuild(); needed {
				t.Fatalf("fresh record not reusable: %s", reason)
			}
			change(t, p, root, c)
			if needed, reason := fixtureCache(t, p).needsBuild(); !needed {
				t.Fatalf("change was missed: %s", reason)
			}
		})
	}
}

func TestCacheDoesNotCertifyChangesDuringBuild(t *testing.T) {
	p, root := mavenFixture(t)
	c := fixtureCache(t, p)
	c.needsBuild()
	if err := writeExecutableJar(p.Environment["RUNNER_TEST_JAR"]); err != nil {
		t.Fatal(err)
	}
	writeCacheInput(t, filepath.Join(root, "common", "src", "main", "java", "Edited.java"), "class Edited {}")
	if err := c.record(); err == nil {
		t.Fatal("build-time edit certified as current")
	}
	if exists(c.path) {
		t.Fatal("unstable build record persisted")
	}
}

func TestFailedRebuildInvalidatesOldCacheEvenWhenSourceReverted(t *testing.T) {
	p, root := mavenFixture(t)
	a := NewApp()
	a.projects[p.ID] = p
	t.Cleanup(func() { a.shutdown(nil) })
	if err := a.StartProject(p.ID); err != nil {
		t.Fatal(err)
	}
	waitStatus(t, a, p.ID, "running")
	if err := a.StopProject(p.ID); err != nil {
		t.Fatal(err)
	}
	waitStatus(t, a, p.ID, "stopped")
	if err := os.Remove(p.Environment["RUNNER_TEST_RUN_ARGS"]); err != nil {
		t.Fatal(err)
	}
	changed := filepath.Join(root, "common", "src", "main", "java", "Changed.java")
	writeCacheInput(t, changed, "class Changed {}")
	p.Environment["RUNNER_TEST_BUILD"] = "fail"
	a.projects[p.ID] = p
	if err := a.StartProject(p.ID); err != nil {
		t.Fatal(err)
	}
	waitStatus(t, a, p.ID, "failed")
	if exists(fixtureCache(t, p).path) {
		t.Fatal("failed build left reusable success record")
	}
	if err := os.Remove(changed); err != nil {
		t.Fatal(err)
	}
	if err := a.StartProject(p.ID); err != nil {
		t.Fatal(err)
	}
	waitStatus(t, a, p.ID, "failed")
	if exists(p.Environment["RUNNER_TEST_RUN_ARGS"]) {
		t.Fatal("old JAR ran after failed rebuild")
	}
}

func TestMavenBuildEnvironmentReferencesInvalidateCache(t *testing.T) {
	p, root := mavenFixture(t)
	writeCacheInput(t, filepath.Join(root, "app", "src", "main", "resources", "config.properties"), "build.flag=${env.COMPILER_FLAG}")
	p.Environment["COMPILER_FLAG"] = "old"
	c := fixtureCache(t, p)
	c.needsBuild()
	if err := writeExecutableJar(p.Environment["RUNNER_TEST_JAR"]); err != nil {
		t.Fatal(err)
	}
	if err := c.record(); err != nil {
		t.Fatal(err)
	}
	p.Environment["COMPILER_FLAG"] = "new"
	if needed, _ := fixtureCache(t, p).needsBuild(); !needed {
		t.Fatal("changed build environment not detected")
	}
	// A copied marker is never portable between worktrees.
	other, _ := mavenFixture(t)
	otherCache := fixtureCache(t, other)
	data, _ := os.ReadFile(c.path)
	writeCacheInput(t, otherCache.path, string(data))
	if needed, _ := otherCache.needsBuild(); !needed {
		t.Fatal("another worktree's marker trusted")
	}
}

func TestDeletedResourceTriggersCleanBuild(t *testing.T) {
	p, root := mavenFixture(t)
	resource := filepath.Join(root, "app", "src", "main", "resources", "removed.yml")
	writeCacheInput(t, resource, "initial resource")
	a := NewApp()
	a.projects[p.ID] = p
	t.Cleanup(func() { a.shutdown(nil) })
	if err := a.StartProject(p.ID); err != nil {
		t.Fatal(err)
	}
	waitStatus(t, a, p.ID, "running")
	if err := a.StopProject(p.ID); err != nil {
		t.Fatal(err)
	}
	waitStatus(t, a, p.ID, "stopped")
	if err := os.Remove(resource); err != nil {
		t.Fatal(err)
	}
	if err := a.StartProject(p.ID); err != nil {
		t.Fatal(err)
	}
	waitStatus(t, a, p.ID, "running")
	data, err := os.ReadFile(p.Environment["RUNNER_TEST_BUILD_ARGS"])
	if err != nil {
		t.Fatal(err)
	}
	var args []string
	if err := json.Unmarshal(data, &args); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(args, " "), "-am clean package") {
		t.Fatalf("deleted resource kept stale output: %v", args)
	}
}

func TestLockedArtifactPreventsMavenFromOverwritingRunningJar(t *testing.T) {
	p, _ := mavenFixture(t)
	if err := writeExecutableJar(p.Environment["RUNNER_TEST_JAR"]); err != nil {
		t.Fatal(err)
	}
	name, err := windows.UTF16PtrFromString(p.Environment["RUNNER_TEST_JAR"])
	if err != nil {
		t.Fatal(err)
	}
	handle, err := windows.CreateFile(name, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(handle)
	a := NewApp()
	a.projects[p.ID] = p
	t.Cleanup(func() { a.shutdown(nil) })
	if err := a.StartProject(p.ID); err != nil {
		t.Fatal(err)
	}
	status := waitStatus(t, a, p.ID, "failed")
	if !strings.Contains(status.Error, "产物被占用") {
		t.Fatalf("missing lock diagnosis: %+v", status)
	}
	if exists(p.Environment["RUNNER_TEST_BUILD_ARGS"]) || exists(p.Environment["RUNNER_TEST_RUN_ARGS"]) {
		t.Fatal("Maven or app launched despite locked JAR")
	}
}
