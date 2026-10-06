# 项目运行台

一个 Windows 桌面工具，用来同时管理多个工作树或分支的本地开发进程。后端使用 Go，界面使用 Wails + WebView2。当前支持 Spring Boot（Maven/Gradle）和 Vue/Node 的 `package.json` 脚本。

## 直接使用

双击 `ProjectRunner.exe`。首次使用时点击“添加项目”，选择项目或工作树目录。工具会尝试识别 `pom.xml`、`build.gradle` / `build.gradle.kts` 和 `package.json`。显示名称默认取 Git 工作树根目录名；如果不是 Git 工作树，就取所选目录名。手动填写的名称不会被自动识别覆盖。然后在界面里填写端口、启动脚本或本地配置路径，保存并启动。

- Maven 项目点击“启动”后，先从当前工作树根目录执行 `mvn -pl <启动模块> -am package -Dmaven.test.skip=true`，自动构建启动模块及其依赖，再使用配置的 JDK 运行本次生成的可执行 JAR。无需手动 `install`，不会把项目模块覆盖到共享 `.m2` 中。旧配置也自动使用此流程；Gradle 仍使用 `bootRun`。
- Maven 项目默认保留构建缓存，修改源码后点击“重启”即可重新构建；需要清理产物时点击“重新构建”，工具会停止当前实例，执行 `clean package`，成功后再启动。开发启动默认跳过测试，项目测试应单独运行。
- 界面依次显示“构建中”“启动中”“运行中”。构建失败不会启动旧 JAR，构建阶段也可以停止；关闭窗口会终止构建和运行进程及其子进程。同一工作树同时只允许一个 Maven 构建/运行实例，不同工作树可独立运行。
- 选择包含多个 Maven 模块的根目录时，自动识别显式绑定 `repackage` 的启动模块；存在多个候选时手动选择“模块子目录”。选择模块目录也会向上定位包含它的聚合 POM，限定在当前 Git 工作树内。需要启动模块生成包含 `Main-Class` 的可执行 JAR，目前支持 `jar`，不支持 `war`。产物路径根据 POM 的 `build.directory`、`finalName` 和常用项目属性解析；复杂 profile 或外部父 POM 中定义的路径暂不自动解析，失败时会提示明确配置路径。
- 优先使用项目中的 `mvnw.cmd` / `gradlew.bat`；否则使用系统 PATH 中的 `mvn.cmd` / `gradle.bat`。也可以在界面中指定 Maven / Gradle 启动文件。
- 新增或编辑项目时，JDK 目录和 Maven / Gradle 启动文件旁的“复用”按钮可选取其他已保存项目的路径。启动文件只列出相同启动类型的项目；不会顺带复制项目目录、端口或本地配置。
- Spring Boot 端口会传给 JVM：`-Dserver.port=8081`。本地配置文件或目录会传成 `-Dapplication.config.path=...`，属性名可改。
- Vue/Node 选择 `package.json` 脚本与包管理器。Vite 和 Vue CLI 可由界面传入端口；其他脚本可选择 `PORT` 环境变量或“不传端口”。
- 多个配置可同时运行，日志各自展示并可搜索。“重点”默认显示错误、警告和运行状态，折叠普通信息与堆栈，并合并重复消息；“仅错误”和“全部”可随时切换。检测到错误会弹出一次提示，并在项目顶部保留提示条。日志内容仍会在当前运行中保留，直到达到缓存上限。
- 左侧项目列表可直接点击“启动”或“停止”操作对应项目；点击项目名称仍可切换到该项目的日志与配置。侧栏启动当前项目时会先保存尚未保存的修改。
- 项目卡片会显示本次启动计时；项目被判定为运行中后固定为启动耗时。停止后仍显示本次的耗时，关闭工具后不保存计时。
- 已保存项目默认只显示日志。顶部“配置”按钮可展开或收起配置表单；新增项目自动展开，保存后返回日志视图。展开时配置区和日志区分别滚动，也可拖动两者之间的分隔条调整日志高度；窗口重新打开后保留该高度。
- Maven、npm 等命令行进程在后台运行，不会弹出额外终端窗口。Windows 批处理文件通过 `cmd.exe` 启动，支持普通的空格、中文路径，无需改成 8.3 短路径。未保存的配置不会自动保留。

配置保存在 `%AppData%\ProjectRunner\projects.json`，日志只在当前运行中保留，单个项目最多保留约 2500 行。

## 系统要求

Windows 10/11 x64，已安装 Microsoft Edge WebView2 Runtime。运行 Spring Boot 需要项目所需版本的 JDK；运行 Vue/Node 项目需要 Node.js 和相应的 npm / pnpm / yarn。Maven / Gradle 项目应有 Wrapper，或提供可用的构建工具。

## 从源码构建

需要 Go 1.25+、Node.js/npm、Wails CLI v2.15.0。源码目录运行：

```powershell
go test ./...
wails build -clean -o ProjectRunner.exe
```

产物位于 `build\bin\ProjectRunner.exe`。

## 当前范围

启动方式以本地开发为主，目前没有 Docker、SSH、热重载或直接选择已有 JAR 的运行模式。Maven 启动流程自动构建并运行产物；停止、取消、构建失败、多工作树和 Windows 路径行为均有进程级回归测试。配置 JDK、构建工具和启动模块后即可使用；运行时仍需项目自己的数据库、Redis 等环境。
