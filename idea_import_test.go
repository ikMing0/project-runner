package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func ideaFixtureFile(t *testing.T, root, name, contents string) string {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func ideaFixture(t *testing.T) (string, ideaTools) {
	t.Helper()
	root := t.TempDir()
	ideaFixtureFile(t, root, ".git/HEAD", "ref: refs/heads/main")
	ideaFixtureFile(t, root, "pom.xml", `<project><modelVersion>4.0.0</modelVersion><groupId>test</groupId><artifactId>parent</artifactId><version>1</version><packaging>pom</packaging><modules><module>ruoyi-admin</module></modules></project>`)
	ideaFixtureFile(t, root, "ruoyi-admin/pom.xml", `<project><modelVersion>4.0.0</modelVersion><groupId>test</groupId><artifactId>ruoyi-admin</artifactId><version>1</version></project>`)
	ideaFixtureFile(t, root, "ruoyi-ui/package.json", `{"scripts":{"dev:vite":"vite","build":"vite build"},"devDependencies":{"vite":"6"}}`)
	ideaFixtureFile(t, root, "config dir/application.properties", "server.port=8090")
	ideaFixtureFile(t, root, ".idea/misc.xml", `<project><component name="ProjectRootManager" project-jdk-name="ms-21"/></project>`)
	ideaFixtureFile(t, root, "home/.jdks/ms-21/bin/java.exe", "fixture")
	ideaFixtureFile(t, root, "node/node.exe", "fixture")
	ideaFixtureFile(t, root, "node/npm.cmd", "fixture")
	mvn := ideaFixtureFile(t, root, "maven/bin/mvn.cmd", "fixture")
	ideaFixtureFile(t, root, "options/jdk.table.xml", `<application><component name="ProjectJdkTable"><jdk><name value="ms-21"/><homePath value="$USER_HOME$/.jdks/ms-21"/></jdk></component></application>`)
	ideaFixtureFile(t, root, "options/nodejs.xml", `<application><component name="NodeJsLocalInterpreterManager"><local-interpreter path="$USER_HOME$/../node/node.exe"/></component></application>`)
	tools := ideaTools{userHome: filepath.Join(root, "home"), sdks: map[string]string{}, previous: []Project{{Kind: "spring-maven", ToolPath: mvn}}}
	readIDEAToolDirectory(&tools, filepath.Join(root, "options"))
	return root, tools
}

const ideaPairXML = `<project><component name="RunManager">
 <configuration default="true" type="SpringBootApplicationConfigurationType"><option name="VM_PARAMETERS" value="-Dserver.port=9999"/></configuration>
 <configuration name="RuoYiApplication" type="SpringBootApplicationConfigurationType">
  <module name="ruoyi-admin"/><option name="SPRING_BOOT_MAIN_CLASS" value="com.ruoyi.RuoYiApplication"/>
  <option name="VM_PARAMETERS" value="-Dserver.port=8081 -Dapplication.config.path=&quot;$PROJECT_DIR$/config dir/application.properties&quot; -Xmx512m"/>
  <option name="PROGRAM_PARAMETERS" value="--spring.profiles.active=local --message &quot;hello world&quot;"/>
  <envs><env name="MODE" value="local"/></envs><method v="2"><option name="Make" enabled="true"/></method>
 </configuration>
 <configuration name="dev:vite" type="js.build_tools.npm">
  <package-json value="$PROJECT_DIR$/ruoyi-ui/package.json"/><command value="run"/><scripts><script value="dev:vite"/></scripts>
  <node-interpreter value="project"/><arguments value="--host 127.0.0.1 --port=83"/>
  <envs><env name="port" value="82"/><env name="VUE_APP_BASE_API_TARGET" value="http://localhost:8081"/></envs>
 </configuration>
 <configuration name="Install" type="js.build_tools.npm"><command value="install"/></configuration>
 <configuration name="Tests" type="JUnit"/>
</component></project>`

func TestIDEAPairReadsTypedParametersMacrosAndGlobalTools(t *testing.T) {
	root, tools := ideaFixture(t)
	source := ideaFixtureFile(t, root, ".idea/workspace.xml", ideaPairXML)
	before, _ := os.ReadFile(source)
	result, err := readIDEAConfigurations(filepath.Join(root, "ruoyi-admin"), tools)
	if err != nil || len(result.Configurations) != 2 || result.Directory != root {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	back, front := result.Configurations[0], result.Configurations[1]
	if back.Values["module"] != "ruoyi-admin" || back.Values["port"] != 8081 || back.MainClass != "com.ruoyi.RuoYiApplication" {
		t.Fatalf("backend=%+v", back)
	}
	if back.Values["javaHome"] != filepath.Join(root, "home", ".jdks", "ms-21") || back.Values["toolPath"] != filepath.Join(root, "maven", "bin", "mvn.cmd") {
		t.Fatalf("tools=%+v", back.Values)
	}
	if back.Values["configFile"] != filepath.Join(root, "config dir", "application.properties") &&
		back.Values["configFile"] != filepath.ToSlash(filepath.Join(root, "config dir", "application.properties")) {
		// Macro expansion retains the original Windows/slash mixture until save.
		if filepath.Clean(back.Values["configFile"].(string)) != filepath.Join(root, "config dir", "application.properties") {
			t.Fatal(back.Values["configFile"])
		}
	}
	if back.Values["jvmArgs"] != "-Xmx512m" || back.Values["appArgs"] != "--spring.profiles.active=local\n--message\nhello world" {
		t.Fatalf("args=%+v", back.Values)
	}
	if front.Values["directory"] != filepath.Join(root, "ruoyi-ui") || front.Values["script"] != "dev:vite" || front.Values["port"] != 83 ||
		front.Values["nodeHome"] != filepath.Join(root, "node") || front.Values["toolPath"] != filepath.Join(root, "node", "npm.cmd") ||
		front.Values["appArgs"] != "--host\n127.0.0.1" {
		t.Fatalf("frontend=%+v", front)
	}
	env := front.Values["environment"].(map[string]string)
	if _, ok := env["port"]; ok {
		t.Fatal("Port alias retained")
	}
	after, _ := os.ReadFile(source)
	if string(before) != string(after) {
		t.Fatal("IDEA workspace was modified")
	}
}

func TestIDEASharedFilesWinAndMalformedFilesDoNotPreventOtherImports(t *testing.T) {
	root, tools := ideaFixture(t)
	ideaFixtureFile(t, root, ".idea/workspace.xml", ideaPairXML)
	ideaFixtureFile(t, root, ".run/backend.run.xml", `<component><configuration name="RuoYiApplication" type="SpringBootApplicationConfigurationType"><module name="ruoyi-admin"/><option name="VM_PARAMETERS" value="-Dserver.port=9001"/></configuration></component>`)
	ideaFixtureFile(t, root, ".idea/runConfigurations/broken.xml", "<component")
	result, err := readIDEAConfigurations(root, tools)
	if err != nil || len(result.Configurations) != 2 || result.Configurations[0].Values["port"] != 9001 ||
		result.Configurations[0].Source != ".run/backend.run.xml" || len(result.Warnings) != 1 {
		t.Fatalf("%+v %v", result, err)
	}
}

func TestIDEAInvalidPathsMacrosAndArgumentQuotesBecomeVisibleWarnings(t *testing.T) {
	root, tools := ideaFixture(t)
	ideaFixtureFile(t, root, ".idea/workspace.xml", `<project><component name="RunManager"><configuration name="Broken" type="SpringBootApplicationConfigurationType"><module name="../../outside"/><option name="ALTERNATIVE_JRE_PATH_ENABLED" value="true"/><option name="ALTERNATIVE_JRE_PATH" value="missing-jdk"/><option name="VM_PARAMETERS" value="-Dapplication.config.path=$PROJECT_DIR$/missing.properties"/></configuration><configuration name="BadArgs" type="SpringBootApplicationConfigurationType"><option name="PROGRAM_PARAMETERS" value="&quot;unclosed"/></configuration><configuration name="LostFrontend" type="js.build_tools.npm"><package-json value="$UNKNOWN$/package.json"/><scripts><script value="dev"/></scripts></configuration></component></project>`)
	result, err := readIDEAConfigurations(root, tools)
	if err != nil || len(result.Configurations) != 3 {
		t.Fatalf("%+v %v", result, err)
	}
	if len(result.Configurations[0].Warnings) < 3 || len(result.Configurations[1].Warnings) == 0 || len(result.Configurations[2].Warnings) == 0 {
		t.Fatal(result.Configurations)
	}
	if _, ok := result.Configurations[0].Values["configFile"]; ok {
		t.Fatal("Invalid external config was imported")
	}
	if _, ok := result.Configurations[2].Values["directory"]; ok {
		t.Fatal("Unresolved macro was imported")
	}
	args, err := ideaArguments(`-Dpath="D:\config dir\application.properties" --label 'hello world' -Xmx512m`)
	if err != nil || !reflect.DeepEqual(args, []string{`-Dpath=D:\config dir\application.properties`, "--label", "hello world", "-Xmx512m"}) {
		t.Fatalf("%v %v", args, err)
	}
}

func TestIDEAWrapperOverridesReusedToolsAndCLIPortOverridesVMAndEnvironment(t *testing.T) {
	root, tools := ideaFixture(t)
	wrapper := ideaFixtureFile(t, root, "mvnw.cmd", "fixture")
	ideaFixtureFile(t, root, ".idea/workspace.xml", `<project><component name="RunManager"><configuration name="Backend" type="SpringBootApplicationConfigurationType"><option name="VM_PARAMETERS" value="-Dserver.port=8081"/><option name="PROGRAM_PARAMETERS" value="--server.port 8082 --debug"/><envs><env name="SERVER_PORT" value="8080"/></envs></configuration></component></project>`)
	result, err := readIDEAConfigurations(root, tools)
	if err != nil || result.Configurations[0].Values["port"] != 8082 || result.Configurations[0].Values["toolPath"] != wrapper ||
		result.Configurations[0].Values["appArgs"] != "--debug" {
		t.Fatalf("%+v %v", result, err)
	}
	env := result.Configurations[0].Values["environment"].(map[string]string)
	if _, ok := env["SERVER_PORT"]; ok {
		t.Fatal("Duplicate server port preserved")
	}
	ideaFixtureFile(t, root, ".idea/workspace.xml", `<project><configuration name="Environment" type="SpringBootApplicationConfigurationType"><envs><env name="server_port" value="8123"/></envs></configuration></project>`)
	result, err = readIDEAConfigurations(root, tools)
	if err != nil || result.Configurations[0].Values["port"] != 8123 || len(result.Configurations[0].Values["environment"].(map[string]string)) != 0 {
		t.Fatalf("case-insensitive environment port: %+v %v", result, err)
	}
}

func TestIDEAAbsentConfigurationAndMalformedEntityStayReadOnly(t *testing.T) {
	root, tools := ideaFixture(t)
	result, err := readIDEAConfigurations(root, tools)
	if err != nil || len(result.Configurations) != 0 {
		t.Fatalf("%+v %v", result, err)
	}
	ideaFixtureFile(t, root, ".idea/workspace.xml", `<!DOCTYPE project [<!ENTITY external SYSTEM "file:///does-not-exist">]><project><configuration name="&external;" type="SpringBootApplicationConfigurationType"/></project>`)
	result, err = readIDEAConfigurations(root, tools)
	if err != nil || len(result.Configurations) != 0 || len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "无法读取") {
		t.Fatalf("%+v %v", result, err)
	}
}
