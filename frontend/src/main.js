import './style.css';
import { isNewRun, actionableError, serviceIDs, activeState, groupState, currentAttemptLog, recoveryText } from './run-state.mjs';
import { TerminalConsole } from './terminal-console.mjs';
import { GitChangesView } from './git-changes.mjs';
import { FrontendDirectoryLink } from './frontend-directory.mjs';
import { moveProject, ProjectOrderController } from './project-order.mjs';
import { nextProjectPort } from './project-ports.mjs';
import { ideaChoices, ideaFieldKeys, mergeIDEAConfiguration } from './idea-import.mjs';
import { codexSelection, codexEfforts, codexEffortLabels } from './codex-selection.mjs';
import { parseAnsiLog, renderAnsiLog } from './ansi-log.mjs';
import { ProjectTools } from './project-tools.mjs';
import { problemAdvice } from './problem-advice.mjs';
import { sourceLinks } from './source-links.mjs';
import { DependencyEditor } from './dependency-editor.mjs';
import { IDEAJumper } from './idea-jump.mjs';
import { DesktopTools, metricsText } from './desktop-tools.mjs';
import {
  ListProjects, SaveProject, DeleteProject, ReorderProjects, GetStatuses, GetLogs,
  PickDirectory, PickConfigFile, PickToolFile, DetectProject, DetectFrontend, StartProject, StopProject, RestartProject, RebuildProject, StartService, StopService,
  NewTerminal, GetTerminals, GetTerminalOutput, WriteTerminal, ResizeTerminal, CloseTerminal,
  GetCodexOptions, OpenCodexAnalysis, ReadIDEAConfigurations,
  GetGitChanges, GetGitFileDiff,
  CheckProject, ListRunHistory, ReadRunHistory, RunHistoryText,
  ListTemplates, SaveTemplate, DeleteTemplate, ExportProjectTemplate, ReadTemplateFile, ApplyTemplate,
  RestartService, OpenSourceInIDEA, GetEditorSettings, SetIDEAPath, PickIDEAPath,
  GetBuildInfo, CheckForUpdates, GetDesktopSettings, SaveDesktopSettings, HideToTray, QuitApplication, GetProcessMetrics,
} from '../wailsjs/go/main/App';
import { EventsOn, BrowserOpenURL, ClipboardSetText } from '../wailsjs/runtime/runtime';

const app = document.querySelector('#app');
const browseIcon = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M3.5 7.5A2.5 2.5 0 0 1 6 5h4l2 2h6a2.5 2.5 0 0 1 2.5 2.5v8A2.5 2.5 0 0 1 18 20H6a2.5 2.5 0 0 1-2.5-2.5z"/><path d="M3.5 10h17"/></svg>`;
app.innerHTML = `
  <div class="shell">
    <aside class="sidebar">
      <div class="brand"><div class="brand-icon">▶</div><div><strong>项目运行台</strong><small>Windows · 本地运行</small></div></div>
      <button id="add-project" class="button primary full">＋ 添加项目</button>
      <button id="templates" class="button full templates-button">配置模板 / 导入导出</button>
      <div class="side-label"><span>运行配置</span><span id="project-count">0</span></div>
      <div id="project-list" class="project-list"></div>
      <div class="side-footer"><div class="desktop-sidebar-actions"><button id="desktop-settings" class="button small" type="button">应用设置 / 更新</button><button id="hide-tray" class="button small" type="button">收起</button></div><div><span class="pulse-dot"></span><span id="close-behavior">关闭窗口时停止所有项目</span></div></div>
    </aside>
    <main class="main">
      <div id="empty-state" class="empty-state">
        <div class="empty-icon">⌘</div><h1>把项目加进来，集中管理运行</h1>
        <p>Spring Boot 与 Vue 项目都可以从工作树启动。每个实例分别配置端口、路径和日志。Maven 项目会自动构建依赖后启动。</p>
        <button id="empty-add" class="button primary">添加第一个项目</button>
      </div>
      <div id="project-view" class="project-view hidden">
        <header class="project-header">
          <div><div class="eyebrow" id="header-kind">运行配置</div><h1 id="header-name">项目</h1><p id="header-path"></p></div>
          <div class="header-actions"><span id="status-pill" class="status-pill">未运行</span><button id="start" class="button primary">▶ 启动</button><button id="restart" class="button">↻ 重启</button><button id="rebuild" class="button" title="清理产物后重新构建并启动">重新构建</button><button id="stop" class="button danger">■ 停止</button><button id="check-project" class="button">启动检查</button><button id="toggle-config" class="button config-toggle" type="button" aria-controls="settings-panel" aria-expanded="false">配置</button></div>
        </header>
        <div id="source-banner" class="source-banner hidden" role="status" aria-live="polite"><div><strong id="source-title"></strong><span id="source-message"></span></div><button id="source-restart" class="button small" type="button">重启后端</button></div>
        <div id="issue-banner" class="issue-banner hidden" role="status" aria-live="polite"><span class="issue-icon">!</span><div class="issue-copy"><strong id="issue-title"></strong><span id="issue-message"></span></div><button id="issue-view" class="button small">查看日志</button></div>
        <div id="problem-advice" class="problem-advice hidden" role="status"></div>
        <div id="content" class="content config-collapsed">
          <section id="settings-panel" class="settings panel">
            <div class="section-title"><div><h2>启动配置</h2><p>界面设置会在启动时转换为相应参数</p></div><div class="section-actions"><button id="idea-import" class="text-button" type="button">从 IDEA 导入</button><button id="duplicate" class="text-button">复制配置</button><button id="delete" class="text-button destructive">删除</button><button id="save" class="button secondary">保存配置</button></div></div>
            <div id="idea-import-hint" class="idea-import-hint hidden" role="status"></div>
            <div class="form-grid">
              <label class="field"><span>显示名称</span><input id="name" placeholder="例如 ruoyi-admin / feature-a"></label>
              <label class="field"><span>启动类型</span><select id="kind"><option value="spring-maven">Spring Boot · Maven</option><option value="spring-gradle">Spring Boot · Gradle</option><option value="node">Vue / Node 脚本</option></select></label>
              <label class="field wide"><span>项目 / 工作树目录</span><div class="input-action"><input id="directory" placeholder="选择含 pom.xml、build.gradle 或 package.json 的目录"><button id="browse-directory" class="button small browse-button" type="button">${browseIcon}浏览</button></div></label>
              <label class="field"><span>端口</span><input id="port" type="number" min="1" max="65535" value="8080"></label>
              <label class="field java-field"><span>模块子目录（可选）</span><input id="module" list="module-options" placeholder="例如 ruoyi-admin"><datalist id="module-options"></datalist></label>
              <label class="field java-field"><span>JDK 目录（可选）</span><div class="input-action"><input id="java-home" placeholder="留空使用系统 JAVA_HOME"><button id="reuse-java" class="button small browse-button reuse-button" type="button" title="从其他已保存项目复用 JDK 目录">复用</button><button id="browse-java" class="button small browse-button" type="button">${browseIcon}浏览</button></div></label>
              <label class="field java-field"><span>Maven / Gradle 启动文件（可选）</span><div class="input-action"><input id="tool-path" placeholder="留空使用项目 Wrapper 或系统 PATH"><button id="reuse-tool" class="button small browse-button reuse-button" type="button" title="从同类型的已保存项目复用启动文件">复用</button><button id="browse-tool" class="button small browse-button" type="button">${browseIcon}浏览</button></div></label>
              <label class="field node-field"><span>包管理器</span><select id="manager"><option value="npm">npm</option><option value="pnpm">pnpm</option><option value="yarn">yarn</option></select></label>
              <label class="field node-field"><span>package.json 脚本</span><input id="script" list="script-options" placeholder="dev"><datalist id="script-options"></datalist></label>
              <label class="field node-field"><span>端口传入方式</span><select id="port-mode"><option value="vite">Vite --port</option><option value="vue-cli">Vue CLI --port</option><option value="env">PORT 环境变量</option><option value="none">不传端口</option></select></label>
              <label class="field node-field"><span>Node.js 目录（可选）</span><div class="input-action"><input id="node-home" placeholder="留空使用系统 PATH"><button id="browse-node" class="button small browse-button" type="button">${browseIcon}浏览</button></div></label>
              <label class="field node-field wide"><span>包管理器启动文件（可选）</span><div class="input-action"><input id="node-tool" placeholder="例如 C:\\nvm4w\\nodejs\\npm.cmd"><button id="browse-node-tool" class="button small browse-button" type="button">${browseIcon}浏览</button></div></label>
              <label class="field wide java-field"><span>本地配置文件 / 目录</span><div class="input-action"><input id="config-file" placeholder="例如 D:\\project\\file\\config\\application.properties"><button id="browse-config" class="button small browse-button" type="button">${browseIcon}选文件</button><button id="browse-config-dir" class="button small browse-button" type="button">${browseIcon}选目录</button></div></label>
              <label class="field java-field"><span>配置路径属性名</span><input id="config-property" value="application.config.path"></label>
              <label class="field java-field"><span>其他 JVM 参数</span><textarea id="jvm-args" rows="3" placeholder="每行一个参数，例如 -Xmx512m"></textarea></label>
              <label class="field"><span>其他应用参数</span><textarea id="app-args" rows="3" placeholder="每行一个参数"></textarea></label>
              <label class="field"><span>环境变量</span><textarea id="environment" rows="3" placeholder="每行 KEY=VALUE"></textarea></label>
              <label class="field wide"><span>就绪检查地址（可选，留空检查端口）</span><input id="health-url" placeholder="http://127.0.0.1:8080/actuator/health"><small class="field-hint">仅访问本机 HTTP(S) 地址；非 200 的接口可指定预期状态码。</small></label>
              <label class="field"><span>就绪等待时间（秒）</span><input id="health-timeout" type="number" min="5" max="600" value="90"></label>
              <label class="field"><span>健康接口预期状态码</span><input id="health-status" type="number" min="200" max="399" value="200"></label>
              <label class="check-field wide"><input id="auto-open" type="checkbox"><span>前后端均就绪后自动打开页面</span></label>
              <div id="dependencies" class="field wide"></div>
            </div>
            <div id="frontend-section" class="frontend-section java-field">
              <div class="section-title"><div><label class="check-field"><input id="frontend-enabled" type="checkbox"><strong>启用配套前端</strong></label><p>作为一组启停，也可在日志页单独操作前端或后端</p></div><button id="detect-frontend" class="button small" type="button">识别配套前端</button></div>
              <div id="frontend-fields" class="form-grid hidden">
                <label class="check-field wide"><input id="wait-backend" type="checkbox"><span>全部启动时，等待后端就绪后再启动前端</span></label>
                <label class="field wide"><span>前端目录（含 package.json）</span><div class="input-action"><input id="frontend-directory" aria-describedby="frontend-directory-hint"><button id="browse-frontend" class="button small browse-button" type="button">${browseIcon}浏览</button></div><small id="frontend-directory-hint" class="field-hint"></small></label>
                <label class="field"><span>前端端口</span><input id="frontend-port" type="number" min="1" max="65535"></label>
                <label class="field"><span>package.json 脚本</span><input id="frontend-script" list="frontend-script-options"><datalist id="frontend-script-options"></datalist></label>
                <label class="field"><span>包管理器</span><select id="frontend-manager"><option value="npm">npm</option><option value="pnpm">pnpm</option><option value="yarn">yarn</option></select></label>
                <label class="field"><span>端口传入方式</span><select id="frontend-port-mode"><option value="vite">Vite --port</option><option value="vue-cli">Vue CLI --port</option><option value="env">PORT 环境变量</option><option value="none">不传端口</option></select></label>
                <label class="field"><span>Node.js 目录（可选）</span><div class="input-action"><input id="frontend-node-home" placeholder="例如 C:\\nvm4w\\nodejs"><button id="browse-frontend-node" class="button small browse-button" type="button">${browseIcon}浏览</button></div></label>
                <label class="field"><span>包管理器启动文件（可选）</span><div class="input-action"><input id="frontend-tool" placeholder="例如 C:\\nvm4w\\nodejs\\npm.cmd"><button id="browse-frontend-tool" class="button small browse-button" type="button">${browseIcon}浏览</button></div></label>
                <div class="field wide proxy-field"><label class="check-field"><input id="frontend-auto-proxy" type="checkbox"><span>代理地址自动跟随后端端口</span></label><p id="frontend-proxy-hint"></p></div>
                <label class="field wide"><span>代理环境变量名</span><input id="frontend-proxy-variable" placeholder="VUE_APP_BASE_API_TARGET"></label>
                <label class="field"><span>前端应用参数</span><textarea id="frontend-args" rows="3" placeholder="每行一个参数"></textarea></label>
                <label class="field"><span>前端环境变量</span><textarea id="frontend-environment" rows="3" placeholder="每行 KEY=VALUE；端口和自动代理由运行台传入"></textarea></label>
                <label class="field wide"><span>前端就绪检查地址（可选）</span><input id="frontend-health-url" placeholder="留空检查前端端口"></label>
                <label class="field"><span>前端就绪等待时间（秒）</span><input id="frontend-health-timeout" type="number" min="5" max="600" value="90"></label>
                <label class="field"><span>前端健康接口预期状态码</span><input id="frontend-health-status" type="number" min="200" max="399" value="200"></label>
              </div>
            </div>
          </section>
          <div id="log-resizer" class="log-resizer" role="separator" aria-label="调整运行日志高度" aria-orientation="horizontal" aria-controls="log-output" tabindex="0" title="上下拖动，调整运行日志高度"><span></span></div>
          <section class="logs panel">
            <div id="service-bar" class="service-bar hidden"><div id="service-tabs" class="service-tabs" role="tablist" aria-label="服务日志"></div><div class="service-actions"><button id="service-start" class="button small">启动当前服务</button><button id="service-stop" class="button small danger">停止当前服务</button></div></div>
            <div class="log-toolbar"><div class="output-heading"><h2 id="output-title">运行日志</h2><span id="log-summary">等待启动</span></div><div class="log-actions"><input id="log-search" placeholder="搜索日志"><button id="run-history" class="button small">启动历史</button><button id="open-browser" class="button small">打开页面</button></div></div>
            <div class="log-filterbar"><div class="log-view-controls"><div class="log-filters"><button class="log-filter selected" data-log-filter="focus" type="button">重点</button><button class="log-filter" data-log-filter="error" type="button">仅错误</button><button class="log-filter" data-log-filter="all" type="button">全部</button></div><button id="terminal-toggle" class="terminal-toggle" type="button" aria-controls="terminal-pane" aria-pressed="false"><span aria-hidden="true">&gt;_</span> 终端</button><div class="codex-actions"><button id="codex-analyze" class="terminal-toggle" type="button">Codex 分析</button><button id="codex-settings" class="terminal-toggle" type="button" aria-label="Codex 分析设置" title="模型与推理强度">▾</button></div></div><span id="log-filter-counts">普通日志会在重点视图中折叠</span><span id="terminal-shortcuts" class="hidden">Ctrl+C 中断 · Ctrl+V 粘贴</span></div>
            <div id="log-output" class="log-output"><div class="log-placeholder">启动项目后，日志将在这里实时显示。</div></div>
            <div id="terminal-pane" class="terminal-pane hidden"><div class="terminal-tabbar"><div id="terminal-tabs" class="terminal-tabs" role="tablist" aria-label="终端标签页"></div><button id="terminal-new" class="button small" type="button">＋ 新标签页</button></div><div id="terminal-workspace" class="terminal-workspace"></div></div>
            <div id="git-pane" class="git-pane hidden"><div class="git-toolbar"><div><h2 id="git-summary">Git 未提交</h2><p id="git-root"></p></div><span class="git-readonly">只读</span><button id="git-refresh" class="button small" type="button">刷新</button></div><div class="git-workspace"><aside class="git-filelist"><input id="git-search" placeholder="搜索未提交文件" aria-label="搜索未提交文件"><div id="git-files"></div></aside><div class="git-detail"><div class="git-detail-toolbar"><span id="git-file-title">文件差异</span><div><button id="git-working" class="button small" type="button">工作区差异</button><button id="git-staged" class="button small" type="button">已暂存差异</button></div></div><span id="git-diff-note"></span><pre id="git-diff">切换到 Git 查看未提交文件。</pre></div></div></div>
          </section>
        </div>
      </div>
    </main>
  </div>
  <dialog id="reuse-dialog" class="reuse-dialog" aria-labelledby="reuse-title"><div class="reuse-dialog-head"><div><h2 id="reuse-title">复用配置</h2><p>选择已保存项目中的路径</p></div><button id="reuse-close" class="button small" type="button" aria-label="关闭">✕</button></div><div id="reuse-options" class="reuse-options"></div></dialog>
  <dialog id="codex-dialog" class="reuse-dialog" aria-labelledby="codex-title"><div class="reuse-dialog-head"><div><h2 id="codex-title">Codex 分析设置</h2><p>定位启动 / 编译错误，只读分析，输出简短建议</p></div><button id="codex-close" class="button small" type="button" aria-label="关闭 Codex 设置">✕</button></div><div class="codex-form"><label class="field"><span>模型（留空自动选择 CLI 可用模型）</span><input id="codex-model" list="codex-model-options" placeholder="自动选择 CLI 可用模型"><datalist id="codex-model-options"></datalist></label><label class="field"><span>推理强度</span><select id="codex-effort"></select></label><p id="codex-cli-hint" class="field-hint"></p><p class="field-hint">默认低强度，先看异常链，必要时查相关源码。每次分析最多带入最近 400 行日志；可在终端继续追问。</p><button id="codex-save" class="button primary" type="button">保存设置</button></div></dialog>
  <dialog id="idea-dialog" class="reuse-dialog" aria-labelledby="idea-title"><div class="reuse-dialog-head"><div><h2 id="idea-title">选择 IDEA 运行配置</h2><p>导入到当前表单，保存后生效</p></div><button id="idea-close" class="button small" type="button" aria-label="取消 IDEA 导入">✕</button></div><div class="codex-form"><label class="field"><span>主运行配置</span><select id="idea-primary"></select></label><label id="idea-frontend-field" class="field"><span>配套前端</span><select id="idea-frontend"></select></label><p id="idea-selection-hint" class="field-hint"></p><button id="idea-apply" class="button primary" type="button">导入所选配置</button></div></dialog>
  <div id="toast" class="toast hidden"></div>
`;

const $ = (id) => document.getElementById(id);
const processMetrics = new Map();
let metricsLoading = false;
const desktopUI = new DesktopTools({
  api: {GetBuildInfo,CheckForUpdates,GetDesktopSettings,SaveDesktopSettings,HideToTray,QuitApplication},
  notify,openURL:BrowserOpenURL,
  onSettings:settings => {
    $('hide-tray').disabled = !settings.trayAvailable;
    $('close-behavior').textContent = settings.closeToTray && settings.trayAvailable ? '关闭窗口收起到托盘，项目继续运行' : '关闭窗口时停止所有项目';
  },
});
const metricsLabel = document.createElement('span');
metricsLabel.id = 'process-metrics'; metricsLabel.className = 'process-metrics hidden';
metricsLabel.title = '当前服务及子进程的 CPU（按逻辑核心归一）与工作集内存；共享页可能重复计入。';
document.querySelector('.output-heading').append(metricsLabel);
const dependencyUI = new DependencyEditor($('dependencies'), () => {
  dirty = true;
  $('save').textContent = '保存更改';
});
const ideaJumper = new IDEAJumper({ OpenSourceInIDEA, GetEditorSettings, SetIDEAPath, PickIDEAPath }, notify);
const editorSettingsButton = document.createElement('button');
editorSettingsButton.type = 'button'; editorSettingsButton.className = 'terminal-toggle';
editorSettingsButton.textContent = 'IDEA 跳转设置';
editorSettingsButton.onclick = () => ideaJumper.settings(logServiceID()).catch(error => notify(error, true));
document.querySelector('.log-view-controls').append(editorSettingsButton);
const terminalUI = new TerminalConsole({
  api: { NewTerminal, GetTerminals, GetTerminalOutput, WriteTerminal, ResizeTerminal, CloseTerminal },
  prepare: async () => {
    if (!draft?.id || dirty) await saveCurrent();
    return terminalContext();
  },
  notify,
  onLogs: () => { if (draft) { renderHeader(); renderLogs(); } },
  onLayout: () => { if (draft && configOpen) setLogHeight(preferredLogHeight); },
  events: EventsOn,
  onRender: () => applyOutputView(),
});
const gitUI = new GitChangesView({
  api: { GetGitChanges, GetGitFileDiff },
  directory: () => logSide === 'frontend' && draft?.frontend ? $('frontend-directory').value.trim() : $('directory').value.trim(),
  onChange: () => { if (draft) renderHeader(); },
});
const projectTools = new ProjectTools({
  api: { CheckProject, StopService, ListRunHistory, ReadRunHistory, RunHistoryText,
    ListTemplates, SaveTemplate, DeleteTemplate, ExportProjectTemplate, ReadTemplateFile, ApplyTemplate, PickDirectory },
  notify, clipboard: ClipboardSetText,
  getDraft: () => { readForm(); return draft; },
  applyDraft: applyTemplateDraft,
  selectOwner: async (serviceID) => {
    const parent = projects.find(p => serviceIDs(p).includes(serviceID));
    if (!parent) { notify('对应配置已删除，请刷新启动检查', true); return; }
    await selectProject(parent.id);
    if (draft?.id === parent.id) await selectLogSide(serviceID.endsWith(':frontend') ? 'frontend' : 'backend');
  },
});

function terminalContext() {
  return { projectId: draft?.id || '', serviceId: logServiceID(), paired: !!draft?.frontend };
}
let projects = [];
let statuses = new Map();
let current = null;
let draft = null;
let frontendDraft = null;
const frontendDirectoryLink = new FrontendDirectoryLink();
const projectOrderUI = new ProjectOrderController($('project-list'), { onMove: reorderProjectList, onRelease: renderList });
let logSide = 'backend';
let logLoadVersion = 0;
let logs = [];
let gitVisible = false;
let dirty = false;
let autoName = '';
let suggestedProjectPort;
const ideaEditedFields = new Set();
let ideaRequestVersion = 0;
let ideaContext = null;
let ideaImportedDirectory = '';
let toastTimer;
let logMode = 'focus';
let groupedLogRows = new Map();
const alertedRuns = new Set();
const projectProblems = new Map();
const pendingProjects = new Set();
const logHeightStorageKey = 'project-runner-log-height';
const storedLogHeight = Number(window.localStorage.getItem(logHeightStorageKey));
let preferredLogHeight = Number.isFinite(storedLogHeight) && storedLogHeight >= 170 ? storedLogHeight : 320;
let displayedLogHeight = preferredLogHeight;
let configOpen = false;
let codexOptions = null;
let codexSettingsProject = null;

function projectCodexSelection(projectID = draft?.id) {
  try { return codexSelection(JSON.parse(window.localStorage.getItem(`project-runner-codex-${projectID}`))); }
  catch { return codexSelection(null); }
}

function renderCodexHint() {
  const selection = projectCodexSelection();
  $('codex-analyze').title = `只读分析当前${logSide === 'frontend' ? '前端' : '后端'}日志 · ${selection.model || '自动选择'} · ${selection.effort}`;
}

function renderCodexEfforts() {
  const previous = $('codex-effort').value;
  const levels = codexEfforts(codexOptions, $('codex-model').value.trim());
  $('codex-effort').replaceChildren(...levels.map(level => new Option(codexEffortLabels[level], level)));
  $('codex-effort').value = levels.includes(previous) ? previous : levels.includes('low') ? 'low' : levels[0];
}

async function openCodexSettings() {
  try {
    if (!draft?.id || dirty) await saveCurrent();
    const context = terminalContext();
    const options = await GetCodexOptions(context.serviceId);
    if (draft?.id !== context.projectId || logServiceID() !== context.serviceId) return;
    codexOptions = options;
    codexSettingsProject = context.projectId;
    const selection = projectCodexSelection();
    $('codex-model').value = selection.model;
    $('codex-model').placeholder = `自动选择${options.defaultModel ? `（${options.defaultModel}）` : ''}`;
    $('codex-model-options').replaceChildren(...(options.models || []).map(model => new Option(model.name || model.id, model.id)));
    renderCodexEfforts();
    if (codexEfforts(options, selection.model).includes(selection.effort)) $('codex-effort').value = selection.effort;
    const modelHint = options.configuredModel && options.defaultModel && options.configuredModel !== options.defaultModel
      ? `。诊断默认使用 ${options.defaultModel}（本机配置：${options.configuredModel}）` : '';
    $('codex-cli-hint').textContent = options.executable
      ? `CLI：${options.executable}${modelHint}${options.catalogError ? `。${options.catalogError}` : ''}`
      : '未找到 Codex CLI，请将 codex.exe / codex.cmd 加入 PATH 并重开运行台';
    $('codex-dialog').showModal();
  } catch (error) { notify(error, true); }
}

function analyzeWithCodex() {
  return terminalUI.newTab(context => {
    const selection = projectCodexSelection(context.projectId);
    return OpenCodexAnalysis(context.serviceId, 100, 30, selection.model, selection.effort);
  }).catch(error => notify(error, true));
}

function logHeightBounds() {
  const content = $('content');
  const style = getComputedStyle(content);
  const available = content.clientHeight - parseFloat(style.paddingTop) - parseFloat(style.paddingBottom) - $('log-resizer').offsetHeight;
  const max = Math.max(170, Math.floor(available - 104));
  const min = Math.min(max, terminalUI.isVisible() ? 320 : draft.frontend ? 220 : 170);
  return { min, max };
}

function setLogHeight(height, save = false) {
  if (!draft || !configOpen) return;
  const { min, max } = logHeightBounds();
  displayedLogHeight = Math.max(min, Math.min(max, Math.round(height)));
  $('content').style.setProperty('--log-height', `${displayedLogHeight}px`);
  $('log-resizer').setAttribute('aria-valuemin', String(min));
  $('log-resizer').setAttribute('aria-valuemax', String(max));
  $('log-resizer').setAttribute('aria-valuenow', String(displayedLogHeight));
  if (save) {
    preferredLogHeight = displayedLogHeight;
    window.localStorage.setItem(logHeightStorageKey, String(preferredLogHeight));
  }
}

function setConfigOpen(open) {
  configOpen = open;
  $('content').classList.toggle('config-collapsed', !open);
  $('toggle-config').classList.toggle('active', open);
  $('toggle-config').textContent = open ? '收起配置' : '配置';
  $('toggle-config').setAttribute('aria-expanded', String(open));
  if (open) setLogHeight(preferredLogHeight);
}

function blankProject() {
  return { id: '', name: '', directory: '', kind: 'spring-maven', port: nextProjectPort(projects, 'spring-maven'),
    configFile: '', configProperty: 'application.config.path', javaHome: '', toolPath: '', module: '',
    packageManager: 'npm', script: 'dev', portMode: 'vite', nodeHome: '', jvmArgs: '', appArgs: '', environment: {}, frontend: null };
}

function blankFrontend() {
  return { directory: '', port: nextProjectPort(projects, 'node', [draft?.port]), packageManager: 'npm', script: 'dev:vite', portMode: 'vite', nodeHome: '', toolPath: '',
    appArgs: '', environment: {}, autoProxy: true, proxyVariable: 'VUE_APP_BASE_API_TARGET' };
}

function logServiceID() { return logSide === 'frontend' && draft?.frontend ? `${draft.id}:frontend` : draft?.id; }
function groupActive(project) { return serviceIDs(project).some(isActive); }
function canStartGroup(project) { return serviceIDs(project).some((id) => !isActive(id)); }
function serviceProject() { return logSide === 'frontend' && draft?.frontend ? { ...draft.frontend, id: logServiceID(), kind: 'node' } : draft; }
async function selectLogSide(side) {
  gitVisible = false;
  gitUI.reset();
  logSide = side;
  terminalUI.showLogs();
  const version = ++logLoadVersion;
  logs = [];
  renderHeader();
  renderLogs();
  const lines = draft?.id ? await GetLogs(logServiceID()) : [];
  if (version !== logLoadVersion) return;
  logs = lines || [];
  renderLogs();
}

function openGitChanges() {
  terminalUI.showLogs();
  gitVisible = true;
  renderHeader();
  renderIssue();
  gitUI.refresh();
}

function notify(message, error = false) {
  $('toast').textContent = String(message);
  $('toast').className = `toast ${error ? 'error' : ''}`;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => $('toast').classList.add('hidden'), 4200);
}

function stateFor(id) { return statuses.get(id)?.state || 'stopped'; }
function isActive(id) { return activeState(stateFor(id)); }
function usesPort(project) { return project.kind !== 'node' || project.portMode !== 'none'; }
function stateText(state) { return ({ waiting: '等待后端就绪', checking: '检查中', building: '构建中', starting: '启动中', running: '运行中', unready: '未就绪', partial: '部分运行', failed: '启动失败', stopped: '未运行' })[state] || '未运行'; }
function kindText(kind) { return ({ 'spring-maven': 'SPRING · MAVEN', 'spring-gradle': 'SPRING · GRADLE', node: 'VUE / NODE' })[kind] || '运行配置'; }
function formatStartupDuration(ms) {
  const seconds = Math.max(0, ms) / 1000;
  if (seconds < 60) return `${seconds.toFixed(1)} 秒`;
  return `${Math.floor(seconds / 60)} 分 ${Math.floor(seconds % 60)} 秒`;
}
function startupText(status) {
  if (['checking', 'building', 'starting'].includes(status?.state) && status.startedAt) {
    return `${recoveryText(status) || stateText(status.state)} · ${formatStartupDuration(Date.now() - status.startedAt)}`;
  }
  if (status?.startupDurationMs != null) {
    return `${status.state === 'running' ? '启动耗时' : '上次启动'} · ${formatStartupDuration(status.startupDurationMs)}`;
  }
  return '';
}
function refreshStartupTimes() {
  document.querySelectorAll('.project-startup-time').forEach((element) => {
    const timing = startupText(statuses.get(element.dataset.projectId));
    element.textContent = timing;
    element.title = timing;
  });
}

function renderList() {
  $('project-count').textContent = String(projects.length);
  if (projectOrderUI.isDragging()) return;
  const list = $('project-list');
  list.replaceChildren();
  for (const project of projects) {
    const item = document.createElement('div');
    item.className = `project-item ${current === project.id ? 'selected' : ''}`;
    item.dataset.projectId = project.id;
    const handle = document.createElement('button');
    handle.type = 'button';
    handle.className = 'project-drag-handle';
    handle.title = '上下拖动排序；Alt+↑ / ↓ 调整位置';
    handle.setAttribute('aria-label', `调整 ${project.name} 的位置`);
    handle.innerHTML = '<svg viewBox="0 0 12 20" aria-hidden="true"><circle cx="3" cy="4" r="1.3"/><circle cx="9" cy="4" r="1.3"/><circle cx="3" cy="10" r="1.3"/><circle cx="9" cy="10" r="1.3"/><circle cx="3" cy="16" r="1.3"/><circle cx="9" cy="16" r="1.3"/></svg>';
    const select = document.createElement('button');
    select.type = 'button';
    select.className = 'project-select';
    select.title = `查看 ${project.name} 的日志与配置`;
    select.setAttribute('aria-label', select.title);
    const top = document.createElement('div');
    top.className = 'project-item-top';
    const dot = document.createElement('span');
    dot.className = `state-dot ${groupState(project, statuses)}`;
    const name = document.createElement('strong');
    name.textContent = project.name;
    top.append(dot, name);
    const problems = serviceIDs(project).map((id) => projectProblems.get(id));
    const problem = problems.includes('error') ? 'error' : problems.includes('warn') ? 'warn' : null;
    if (problem) {
      const mark = document.createElement('span');
      mark.className = `project-alert-mark ${problem}`;
      mark.textContent = problem === 'error' ? '错误' : '警告';
      top.append(mark);
    }
    const sub = document.createElement('small');
    sub.textContent = project.frontend ? `后端 :${project.port} · 前端 :${project.frontend.port}` : `${project.kind === 'node' ? 'Vue / Node' : 'Spring Boot'} · :${project.port}`;
    select.append(top, sub);
    const status = statuses.get(project.id);
    const timing = startupText(status);
    if (timing) {
      const duration = document.createElement('span');
      duration.className = `project-startup-time ${['checking', 'building', 'starting'].includes(status.state) ? 'starting' : ''}`;
      duration.dataset.projectId = project.id;
      duration.textContent = timing;
      duration.title = timing;
      select.append(duration);
    }
    select.onclick = () => selectProject(project.id);
    const active = groupActive(project);
    const quick = document.createElement('button');
    quick.type = 'button';
    quick.className = `project-quick-action ${active ? 'stop' : 'start'}`;
    quick.textContent = pendingProjects.has(project.id) ? '处理中' : active ? '■ 停止' : '▶ 启动';
    quick.title = `${active ? '停止' : '启动'} ${project.name}`;
    quick.setAttribute('aria-label', quick.title);
    quick.disabled = pendingProjects.has(project.id);
    quick.onclick = () => quickAction(project.id, active ? 'stop' : 'start');
    item.append(handle, select, quick);
    list.append(item);
  }
}

async function reorderProjectList(sourceID, targetID, position) {
  const previous = projects;
  const ordered = moveProject(projects, sourceID, targetID, position);
  if (ordered.every((project, i) => project.id === projects[i].id)) return;
  projects = ordered;
  renderList();
  try {
    await ReorderProjects(ordered.map(project => project.id));
  } catch (error) {
    try { projects = await ListProjects(); } catch { projects = previous; }
    renderList();
    notify(`项目排序保存失败：${error}`, true);
  }
}

async function selectProject(id) {
  if (dirty && !confirm('当前配置尚未保存，确定切换项目吗？')) return;
  gitVisible = false;
  gitUI.reset();
  current = id;
  resetIDEAImport();
  draft = structuredClone(projects.find((p) => p.id === id));
  frontendDraft = draft.frontend ? structuredClone(draft.frontend) : null;
  frontendDirectoryLink.reset(draft.directory, frontendDraft?.directory);
  dirty = false;
  autoName = '';
  suggestedProjectPort = undefined;
  configOpen = false;
  render();
  await selectLogSide('backend');
}

function readEnvironment(id) {
  const environment = {};
  for (const line of $(id).value.split('\n')) {
    if (!line.trim()) continue;
    const equals = line.indexOf('=');
    if (equals < 1) throw new Error(`环境变量格式应为 KEY=VALUE：${line}`);
    environment[line.slice(0, equals).trim()] = line.slice(equals + 1);
  }
  return environment;
}

function readForm() {
  if (!draft) return;
  syncFrontendDirectory();
  draft.name = $('name').value.trim();
  draft.directory = $('directory').value.trim();
  draft.kind = $('kind').value;
  draft.port = Number($('port').value);
  draft.module = $('module').value.trim();
  draft.javaHome = $('java-home').value.trim();
  draft.toolPath = $(draft.kind === 'node' ? 'node-tool' : 'tool-path').value.trim();
  draft.nodeHome = $('node-home').value.trim();
  draft.packageManager = $('manager').value;
  draft.script = $('script').value.trim();
  draft.portMode = $('port-mode').value;
  draft.configFile = $('config-file').value.trim();
  draft.configProperty = $('config-property').value.trim();
  draft.jvmArgs = $('jvm-args').value;
  draft.appArgs = $('app-args').value;
  draft.environment = readEnvironment('environment');
  draft.health = readHealth('');
  draft.autoOpen = $('auto-open').checked;
  draft.dependencies = dependencyUI.read();
  draft.waitForBackend = draft.kind !== 'node' && $('frontend-enabled').checked && $('wait-backend').checked;
  frontendDraft = {
    directory: $('frontend-directory').value.trim(), port: Number($('frontend-port').value),
    script: $('frontend-script').value.trim(), packageManager: $('frontend-manager').value, portMode: $('frontend-port-mode').value,
    nodeHome: $('frontend-node-home').value.trim(), toolPath: $('frontend-tool').value.trim(), appArgs: $('frontend-args').value,
    environment: draft.kind !== 'node' && $('frontend-enabled').checked ? readEnvironment('frontend-environment') : (frontendDraft?.environment || {}), autoProxy: $('frontend-auto-proxy').checked, proxyVariable: $('frontend-proxy-variable').value.trim(),
    health: readHealth('frontend-'),
  };
  draft.frontend = draft.kind !== 'node' && $('frontend-enabled').checked ? frontendDraft : null;
  if (!draft.frontend) logSide = 'backend';
}

function readHealth(prefix) {
  const url = $(prefix + 'health-url').value.trim();
  const timeoutSeconds = Number($(prefix + 'health-timeout').value || 90);
  const expectedStatus = Number($(prefix + 'health-status').value || 200);
  return !url && timeoutSeconds === 90 && expectedStatus === 200 ? null : { url, timeoutSeconds, expectedStatus };
}

function renderHealth(prefix, health) {
  $(prefix + 'health-url').value = health?.url || '';
  $(prefix + 'health-timeout').value = health?.timeoutSeconds || 90;
  $(prefix + 'health-status').value = health?.expectedStatus || 200;
}

function applyTemplateDraft(project) {
  if (dirty && !confirm('当前配置尚未保存，确定应用模板到新配置吗？')) return false;
  gitVisible = false;
  gitUI.reset();
  resetIDEAImport();
  draft = structuredClone(project);
  const nextPort = nextProjectPort(projects, draft.kind);
  retargetHealth(draft.health, draft.port, nextPort);
  draft.port = nextPort;
  if (draft.frontend) {
    const frontPort = nextProjectPort(projects, 'node', [draft.port]);
    retargetHealth(draft.frontend.health, draft.frontend.port, frontPort);
    draft.frontend.port = frontPort;
  }
  frontendDraft = draft.frontend ? structuredClone(draft.frontend) : null;
  frontendDirectoryLink.reset(draft.directory, frontendDraft?.directory);
  current = null; logSide = 'backend'; ++logLoadVersion;
  dirty = true; configOpen = true; logs = []; autoName = '';
  suggestedProjectPort = draft.port;
  $('log-search').value = '';
  render();
  notify('模板已填入新配置，请检查目录和本机配置后保存');
  return true;
}

function retargetHealth(health, oldPort, newPort) {
  if (!health?.url || !newPort) return;
  try {
    const url = new URL(health.url);
    if (Number(url.port || (url.protocol === 'https:' ? 443 : 80)) === oldPort) {
      url.port = String(newPort);
      health.url = url.href;
    }
  } catch { /* SaveProject reports invalid URLs. */ }
}

function reusablePaths(field) {
  const values = new Map();
  for (const project of projects) {
    if (project.id === draft?.id || project.kind === 'node') continue;
    if (field === 'toolPath' && project.kind !== draft?.kind) continue;
    const path = String(project[field] || '').trim();
    if (!path) continue;
    const key = path.replaceAll('/', '\\').toLowerCase();
    if (!values.has(key)) values.set(key, { path, projects: [] });
    values.get(key).projects.push(project.name);
  }
  return [...values.values()];
}

function openReuseDialog(field) {
  const choices = reusablePaths(field);
  if (!choices.length) return;
  const input = $(field === 'javaHome' ? 'java-home' : 'tool-path');
  $('reuse-title').textContent = field === 'javaHome' ? '复用 JDK 目录' : '复用 Maven / Gradle 启动文件';
  const list = $('reuse-options');
  list.replaceChildren();
  for (const choice of choices) {
    const option = document.createElement('button');
    option.type = 'button';
    option.className = 'reuse-option';
    const source = document.createElement('span');
    source.className = 'reuse-source';
    source.textContent = `来自 ${choice.projects.join('、')}`;
    const path = document.createElement('code');
    path.textContent = choice.path;
    option.append(source, path);
    option.onclick = () => {
      input.value = choice.path;
      input.dispatchEvent(new Event('input', { bubbles: true }));
      $('reuse-dialog').close();
      input.focus();
    };
    list.append(option);
  }
  $('reuse-dialog').showModal();
  list.firstElementChild.focus();
}

function renderForm() {
  const p = draft;
  dependencyUI.render(p.dependencies);
  $('wait-backend').checked = !!p.waitForBackend;
  renderHealth('', p.health);
  $('auto-open').checked = !!p.autoOpen;
  for (const [field, value] of Object.entries({ name: p.name, directory: p.directory, kind: p.kind,
    port: p.port, module: p.module, 'java-home': p.javaHome, 'tool-path': p.toolPath, 'node-tool': p.toolPath, 'node-home': p.nodeHome, manager: p.packageManager || 'npm',
    script: p.script || 'dev', 'port-mode': p.portMode || 'none', 'config-file': p.configFile,
    'config-property': p.configProperty || 'application.config.path', 'jvm-args': p.jvmArgs,
    'app-args': p.appArgs, environment: Object.entries(p.environment || {}).map(([k, v]) => `${k}=${v}`).join('\n') })) {
    $(field).value = value ?? '';
  }
  const java = p.kind !== 'node';
  document.querySelectorAll('.java-field').forEach((el) => el.classList.toggle('hidden', !java));
  document.querySelectorAll('.node-field').forEach((el) => el.classList.toggle('hidden', java));
  const f = p.frontend || frontendDraft || blankFrontend();
  renderHealth('frontend-', f.health);
  $('frontend-enabled').checked = !!p.frontend;
  $('frontend-fields').classList.toggle('hidden', !p.frontend);
  for (const [field, value] of Object.entries({ directory: f.directory, port: f.port, script: f.script, manager: f.packageManager,
    'port-mode': f.portMode, 'node-home': f.nodeHome, tool: f.toolPath, args: f.appArgs,
    environment: Object.entries(f.environment || {}).map(([k, v]) => `${k}=${v}`).join('\n'), 'proxy-variable': f.proxyVariable })) {
    $(`frontend-${field}`).value = value ?? '';
  }
  $('frontend-auto-proxy').checked = f.autoProxy;
  renderFrontendDirectoryHint();
  renderProxyHint();
  const active = p.id && groupActive(p);
  document.querySelectorAll('.settings input, .settings select, .settings textarea, .settings button').forEach((el) => { el.disabled = !!active; });
  $('dependencies').querySelector('.dependency-add').disabled = !!active || (p.dependencies || []).length >= 16;
  $('reuse-java').disabled = !!active || reusablePaths('javaHome').length === 0;
  $('reuse-tool').disabled = !!active || reusablePaths('toolPath').length === 0;
  $('duplicate').disabled = false;
  $('delete').disabled = !!active || !p.id;
  $('save').textContent = dirty ? '保存更改' : '保存配置';
}

function renderHeader() {
  const p = draft;
  $('header-kind').textContent = `${kindText(p.kind)}${p.frontend ? ' + VUE / NODE' : ''}`;
  $('header-name').textContent = p.name || '新项目';
  $('header-path').textContent = p.directory || '请选择项目目录';
  const state = groupState(p, statuses);
  const pending = pendingProjects.has(p.id);
  const backendStatus = statuses.get(p.id) || {};
  const sourceVisible = p.kind !== 'node' && isActive(p.id) && (backendStatus.sourceChanged || backendStatus.sourceError);
  $('source-banner').classList.toggle('hidden', !sourceVisible);
  $('source-title').textContent = backendStatus.sourceChanged ? '后端代码已更新，需重启以应用更改' : '源码检查暂不可用';
  $('source-message').textContent = [backendStatus.sourceMessage, backendStatus.sourceError].filter(Boolean).join(' · ');
  $('source-restart').classList.toggle('hidden', !backendStatus.sourceChanged);
  $('source-restart').disabled = pending;
  $('check-project').disabled = pending;
  $('run-history').disabled = !p.id;
  $('status-pill').textContent = stateText(state);
  $('status-pill').className = `status-pill ${state}`;
  $('start').textContent = p.frontend ? (groupActive(p) && canStartGroup(p) ? '▶ 补齐启动' : '▶ 全部启动') : '▶ 启动';
  $('stop').textContent = p.frontend ? '■ 全部停止' : '■ 停止';
  $('restart').textContent = p.frontend ? '↻ 全部重启' : '↻ 重启';
  $('start').disabled = pending || !canStartGroup(p);
  $('restart').disabled = pending || !p.id || !groupActive(p);
  $('stop').disabled = pending || !p.id || !groupActive(p);
  $('rebuild').classList.toggle('hidden', p.kind !== 'spring-maven');
  $('rebuild').disabled = !p.id || pendingProjects.has(p.id);
  const page = p.frontend ? { ...p.frontend, id: `${p.id}:frontend`, kind: 'node' } : p;
  $('open-browser').disabled = !p.id || stateFor(page.id) !== 'running' || !usesPort(page);
  const service = serviceProject();
  const status = statuses.get(service.id) || { state: 'stopped' };
  renderProcessMetrics();
  $('log-summary').textContent = ['failed', 'unready'].includes(status.state) ? (status.error || stateText(status.state)) : `${stateText(status.state)} · ${usesPort(service) ? `端口 ${service.port}` : '未指定端口'}`;
  if (recoveryText(status)) $('log-summary').textContent += ` · ${recoveryText(status)}`;
  $('service-bar').classList.remove('hidden');
  $('service-tabs').replaceChildren();
  {
    const sides = p.frontend ? [['backend', p.id, '后端'], ['frontend', `${p.id}:frontend`, '前端']] : [['backend', p.id, p.kind === 'node' ? '前端' : '后端']];
    for (const [side, id, label] of sides) {
      const tab = document.createElement('button');
      tab.className = `service-tab ${!gitVisible && logSide === side ? 'selected' : ''}`;
      tab.setAttribute('role', 'tab');
      tab.setAttribute('aria-selected', String(!gitVisible && logSide === side));
      tab.textContent = `${label} · ${recoveryText(statuses.get(id)) || stateText(stateFor(id))}${projectProblems.get(id) === 'error' ? ' · 错误' : ''}`;
      tab.onclick = () => selectLogSide(side).catch((error) => notify(error, true));
      $('service-tabs').append(tab);
    }
  }
  const gitTab = document.createElement('button');
  gitTab.className = `service-tab ${gitVisible ? 'selected' : ''}`;
  gitTab.setAttribute('role', 'tab');
  gitTab.setAttribute('aria-selected', String(gitVisible));
  gitTab.textContent = `Git 未提交${gitVisible && gitUI.data ? ` · ${gitUI.data.files.length}` : ''}`;
  gitTab.onclick = openGitChanges;
  $('service-tabs').append(gitTab);
  $('service-start').disabled = pending || isActive(service.id);
  $('service-stop').disabled = pending || !isActive(service.id);
  terminalUI.setContext(terminalContext());
  applyOutputView();
  renderCodexHint();
}

function applyOutputView() {
  $('git-pane').classList.toggle('hidden', !gitVisible);
  document.querySelector('.service-actions').classList.toggle('hidden', gitVisible);
  document.querySelector('.log-toolbar').classList.toggle('hidden', gitVisible);
  document.querySelector('.log-filterbar').classList.toggle('hidden', gitVisible);
  if (gitVisible) { $('log-output').classList.add('hidden'); $('terminal-pane').classList.add('hidden'); }
  renderProcessMetrics();
}

function renderProcessMetrics() {
  const service = serviceProject();
  const status = statuses.get(service?.id);
  metricsLabel.classList.toggle('hidden', gitVisible || !status?.pid || !activeState(status.state));
  metricsLabel.textContent = metricsText(processMetrics.get(service?.id));
}
async function refreshProcessMetrics() {
  if (metricsLoading || document.hidden || !draft || gitVisible || !groupActive(draft)) return;
  metricsLoading = true;
  try {
    const values = await GetProcessMetrics();
    processMetrics.clear();
    for (const value of values || []) {
      if (statuses.get(value.id)?.startedAt === value.startedAt) processMetrics.set(value.id,value);
    }
    renderProcessMetrics();
  } catch { metricsLabel.textContent = '资源信息暂不可用'; }
  finally {metricsLoading = false;}
}

function renderProxyHint() {
  $('frontend-proxy-hint').textContent = $('frontend-auto-proxy').checked ? `${$('frontend-proxy-variable').value || 'VUE_APP_BASE_API_TARGET'} = http://localhost:${$('port').value}` : '使用前端环境变量或项目中的代理配置';
}

function renderFrontendDirectoryHint() {
  $('frontend-directory-hint').textContent = frontendDirectoryLink.relative !== null
    ? `跟随后端目录，保持相对位置：${frontendDirectoryLink.relative || '.'}`
    : '可选择项目内的前端目录自动跟随，或单独指定其他目录';
}

function syncFrontendDirectory() {
  const input = $('frontend-directory');
  input.value = frontendDirectoryLink.resolve($('directory').value, input.value);
  renderFrontendDirectoryHint();
}

function resetFrontendDirectoryLink() {
  frontendDirectoryLink.reset($('directory').value, $('frontend-directory').value);
  renderFrontendDirectoryHint();
}

const logDisplayCache = new WeakMap();
function logDisplay(line) {
  if (!logDisplayCache.has(line)) logDisplayCache.set(line, parseAnsiLog(line.text));
  return logDisplayCache.get(line);
}
function logLevel(line) { return line.level || (line.source === 'system' ? 'system' : 'info'); }
function matchesLog(line, query) {
  const level = logLevel(line);
  if (logMode === 'error' && level !== 'error') return false;
  if (logMode === 'focus' && !['error', 'warn', 'system'].includes(level)) return false;
  return !query || logDisplay(line).text.toLowerCase().includes(query);
}

function renderIssue() {
  const banner = $('issue-banner');
  if (gitVisible) { banner.classList.add('hidden'); $('problem-advice').classList.add('hidden'); return; }
  const status = statuses.get(logServiceID());
  const currentLogs = logs.filter((line) => currentAttemptLog(line, status));
  renderProblemAdvice(currentLogs, status);
  const errors = currentLogs.filter((line) => logLevel(line) === 'error');
  const warnings = currentLogs.filter((line) => logLevel(line) === 'warn');
  const latest = errors.find((line) => actionableError(logDisplay(line).text)) || errors[0] || warnings[0];
  const recovered = !latest && status?.recovery === 'recovered';
  banner.classList.toggle('recovered', recovered);
  banner.classList.toggle('hidden', !latest && !recovered && status?.state !== 'unready');
  banner.classList.toggle('warning', !!latest && !errors.length);
  if (recovered) {
    $('issue-title').textContent = '已自动恢复';
    $('issue-message').textContent = '首次启动出现产物异常，清理重建后启动成功；两次尝试的日志均已保留。';
    $('issue-message').title = $('issue-message').textContent;
    return;
  }
  if (!latest) {
    if (status?.state === 'unready') {
      $('issue-title').textContent = '服务未就绪';
      $('issue-message').textContent = status.error;
      $('issue-message').title = status.error;
    }
    return;
  }
  $('issue-title').textContent = errors.length ? `检测到错误记录 ${errors.length} 行` : `检测到警告 ${warnings.length} 行`;
  $('issue-message').textContent = logDisplay(latest).text.trim();
  $('issue-message').title = logDisplay(latest).text.trim();
}

let adviceKey = '';
function renderProblemAdvice(lines, status) {
  const advice = problemAdvice(lines, status);
  const element = $('problem-advice');
  element.classList.toggle('hidden', !advice);
  const key = JSON.stringify(advice);
  if (key === adviceKey) return;
  adviceKey = key;
  element.replaceChildren();
  if (!advice) return;
  const title = document.createElement('strong');
  title.textContent = advice.title;
  const evidence = document.createElement('code');
  evidence.textContent = advice.evidence;
  evidence.title = advice.evidence;
  const list = document.createElement('ol');
  for (const step of advice.steps) {
    const item = document.createElement('li'); item.textContent = step; list.append(item);
  }
  element.append(title, evidence, list);
}

function renderLogCounts() {
  const errors = logs.filter((line) => logLevel(line) === 'error').length;
  const warnings = logs.filter((line) => logLevel(line) === 'warn').length;
  const folded = logs.filter((line) => !['error', 'warn', 'system'].includes(logLevel(line))).length;
  $('log-filter-counts').textContent = `错误 ${errors} · 警告 ${warnings}${logMode === 'focus' ? ` · 折叠 ${folded} 行普通日志` : ''}`;
  document.querySelectorAll('.log-filter').forEach((button) => {
    const selected = button.dataset.logFilter === logMode && !terminalUI.isVisible();
    button.classList.toggle('selected', selected);
    button.setAttribute('aria-pressed', String(selected));
  });
}

function appendLogRow(line, output) {
  const level = logLevel(line);
  // Spring prefixes each repeat with a new timestamp; ignore that prefix when grouping.
  const display = logDisplay(line);
  const normalized = display.text.trim().replace(/^\d{4}-\d{2}-\d{2}T\S+\s+(?:TRACE|DEBUG|INFO|WARN|ERROR)\s+\d+\s+---\s*/, '');
  const key = `${level}\u0000${normalized}`;
  if (logMode === 'focus' && groupedLogRows.has(key)) {
    const grouped = groupedLogRows.get(key);
    if (grouped.row.isConnected) {
      grouped.count += 1;
      grouped.badge.textContent = `×${grouped.count}`;
      return;
    }
    groupedLogRows.delete(key);
  }
  const row = document.createElement('div');
  row.className = `log-line ${level}`;
  const time = document.createElement('span');
  time.className = 'log-time';
  time.textContent = line.time;
  const message = document.createElement('span');
  message.className = 'log-message';
  const serviceID = logServiceID();
  renderAnsiLog(message, display.runs, BrowserOpenURL, {
    links: sourceLinks, open: location => ideaJumper.open(serviceID, location),
  });
  row.append(time, message);
  if (logMode === 'focus') {
    const badge = document.createElement('span');
    badge.className = 'repeat-badge';
    row.append(badge);
    groupedLogRows.set(key, { row, badge, count: 1 });
  }
  output.append(row);
  while (output.childElementCount > 2000) output.firstElementChild.remove();
}

function renderLogs() {
  const output = $('log-output');
  output.replaceChildren();
  groupedLogRows = new Map();
  const query = $('log-search').value.toLowerCase();
  const visible = logs.filter((line) => matchesLog(line, query));
  renderIssue();
  renderLogCounts();
  if (!visible.length) {
    const placeholder = document.createElement('div');
    placeholder.className = 'log-placeholder';
    placeholder.textContent = query ? '当前视图没有匹配的日志，可切换“全部”查看。' : logs.length ? '当前视图没有日志，可切换“全部”查看完整启动信息。' : '启动项目后，日志将在这里实时显示。';
    output.append(placeholder);
    return;
  }
  for (const line of visible.slice(-2000)) {
    appendLogRow(line, output);
  }
  output.scrollTop = output.scrollHeight;
}

function render() {
  renderList();
  $('empty-state').classList.toggle('hidden', !!draft);
  $('project-view').classList.toggle('hidden', !draft);
  if (!draft) { terminalUI.setContext(null); return; }
  renderHeader();
  renderForm();
  renderLogs();
  setConfigOpen(configOpen);
}

function addProject(copy = false) {
  if (dirty && !confirm('当前配置尚未保存，确定新建吗？')) return;
  gitVisible = false;
  gitUI.reset();
  resetIDEAImport();
  draft = copy && draft ? { ...structuredClone(draft), id: '', name: `${draft.name} 副本` } : blankProject();
  draft.port = nextProjectPort(projects, draft.kind);
  suggestedProjectPort = draft.port;
  frontendDraft = draft.frontend ? { ...structuredClone(draft.frontend), port: nextProjectPort(projects, 'node', [draft.port]) } : blankFrontend();
  if (draft.frontend) draft.frontend = frontendDraft;
  frontendDirectoryLink.reset(draft.directory, frontendDraft?.directory);
  logSide = 'backend';
  ++logLoadVersion;
  current = null;
  dirty = true;
  autoName = '';
  configOpen = true;
  logs = [];
  $('log-search').value = '';
  render();
  $('name').focus();
  if (draft.port === null || draft.frontend?.port === null) notify('历史端口已到 65535，请手动填写可用端口', true);
}

function resetIDEAImport() {
  ideaRequestVersion++;
  ideaContext = null;
  ideaImportedDirectory = '';
  ideaEditedFields.clear();
  $('idea-import').disabled = false;
  $('idea-import-hint').replaceChildren();
  $('idea-import-hint').classList.add('hidden');
  if ($('idea-dialog').open) $('idea-dialog').close();
}

function showIDEAImportHint(title, messages = []) {
  const hint = $('idea-import-hint');
  hint.replaceChildren();
  const heading = document.createElement('strong');
  heading.textContent = title;
  hint.append(heading);
  for (const text of [...new Set(messages)]) {
    const item = document.createElement('span');
    item.textContent = text;
    hint.append(item);
  }
  hint.classList.remove('hidden');
}

function renderIDEASelectionHint() {
  const context = ideaContext;
  if (!context) return;
  const selected = [
    context.choices.primary.find(item => item.id === $('idea-primary').value),
    context.choices.frontend.find(item => item.id === $('idea-frontend').value),
  ].filter(Boolean);
  $('idea-selection-hint').textContent = selected.map(item => item.name + ' · ' + item.source).join('；') +
    (context.force ? '。导入会更新对应字段，请检查后保存。' : '。自动导入保留已手动编辑的字段。');
}

async function importIDEAConfigurations(force) {
  if (!draft) return;
  readForm();
  const directory = draft.directory;
  if (!directory || (!force && ideaImportedDirectory === directory)) return;
  const project = draft;
  const version = ++ideaRequestVersion;
  $('idea-import').disabled = true;
  try {
    const catalog = await ReadIDEAConfigurations(directory);
    if (draft !== project || $('directory').value.trim() !== directory || version !== ideaRequestVersion) return;
    const choices = ideaChoices(catalog, draft.kind);
    if (!choices.primary.length) {
      if (catalog.warnings?.length) showIDEAImportHint('IDEA 配置读取提示', catalog.warnings);
      if (force) notify('未找到可导入的 Spring Boot 或 npm 运行配置');
      return;
    }
    const context = {project, directory, catalog, choices, force, version};
    ideaContext = context;
    if (force || choices.primary.length > 1 || choices.frontend.length > 1) {
      $('idea-primary').replaceChildren(...choices.primary.map(item => new Option(item.name + ' · ' + item.source, item.id)));
      $('idea-frontend').replaceChildren(new Option('不导入配套前端', ''),
        ...choices.frontend.map(item => new Option(item.name + ' · ' + item.source, item.id)));
      $('idea-frontend-field').classList.toggle('hidden', !choices.frontend.length);
      $('idea-frontend').value = choices.frontend.length === 1 ? choices.frontend[0].id : '';
      renderIDEASelectionHint();
      $('idea-dialog').showModal();
    } else {
      applyIDEASelection(context, choices.primary[0], choices.frontend[0]);
    }
  } catch (error) {
    if (draft === project && version === ideaRequestVersion) notify('读取 IDEA 配置失败：' + error, true);
  } finally {
    if (version === ideaRequestVersion) $('idea-import').disabled = false;
  }
}

function applyIDEASelection(context, primary, frontend) {
  if (!primary || context.project !== draft || context.version !== ideaRequestVersion ||
      $('directory').value.trim() !== context.directory) {
    notify('项目目录已变化，请重新读取 IDEA 配置');
    return;
  }
  readForm();
  const merged = mergeIDEAConfiguration(draft, frontendDraft || blankFrontend(), primary, frontend,
    projects, ideaEditedFields, context.force);
  draft = merged.project;
  frontendDraft = merged.frontend;
  suggestedProjectPort = undefined;
  frontendDirectoryLink.reset(draft.directory, frontendDraft?.directory);
  ideaImportedDirectory = draft.directory;
  dirty = true;
  showIDEAImportHint('已读取 IDEA 配置，请检查后保存',
    [primary.name + ' · ' + primary.source, frontend ? frontend.name + ' · ' + frontend.source : '',
      ...(context.catalog.warnings || []), ...merged.messages].filter(Boolean));
  render();
  notify('已导入 IDEA 运行配置，保存后生效');
}

function updateSuggestedProjectPort(kind) {
  if (draft?.id || suggestedProjectPort === undefined || $('port').value !== String(suggestedProjectPort ?? '')) return;
  suggestedProjectPort = nextProjectPort(projects, kind);
  $('port').value = suggestedProjectPort ?? '';
}

async function saveCurrent() {
  readForm();
  const saved = await SaveProject(draft);
  draft = structuredClone(saved);
  frontendDraft = draft.frontend ? structuredClone(draft.frontend) : null;
  frontendDirectoryLink.reset(draft.directory, frontendDraft?.directory);
  current = saved.id;
  dirty = false;
  autoName = '';
  suggestedProjectPort = undefined;
  configOpen = false;
  projects = await ListProjects();
  render();
  notify('运行配置已保存');
  return saved;
}

async function act(action) {
  const id = draft?.id;
  if (!draft || pendingProjects.has(id)) return;
  pendingProjects.add(id);
  renderHeader();
  try {
    if (['start', 'service-start', 'rebuild', 'restart', 'backend-restart'].includes(action)) {
      readForm();
      const project = draft;
      const side = logSide;
      const scope = action === 'backend-restart' ? 'backend' : action === 'service-start' ? side : '';
      if (!await projectTools.check(project, scope, false)) return;
      if (draft !== project || logSide !== side) { notify('已切换项目或服务，请在当前页面重新启动'); return; }
    }
    if (action === 'start') {
      const saved = dirty || !draft.id ? await saveCurrent() : draft;
      await StartProject(saved.id);
    } else if (action === 'stop') {
      await StopProject(draft.id);
    } else if (action === 'restart') {
      await RestartProject(draft.id);
    } else if (action === 'backend-restart') {
      await RestartService(draft.id);
    } else if (action === 'rebuild') {
      const saved = dirty || !draft.id ? await saveCurrent() : draft;
      await RebuildProject(saved.id);
    } else if (action === 'service-start') {
      const saved = dirty || !draft.id ? await saveCurrent() : draft;
      await StartService(logSide === 'frontend' ? `${saved.id}:frontend` : saved.id);
    } else if (action === 'service-stop') {
      await StopService(logServiceID());
    }
  } catch (error) { notify(error, true); }
  finally { pendingProjects.delete(id); if (draft) renderHeader(); renderList(); }
}

async function quickAction(id, action) {
  if (pendingProjects.has(id)) return;
  pendingProjects.add(id);
  renderList();
  try {
    if (action === 'start') {
      const p = draft?.id === id ? (readForm(), draft) : projects.find(project => project.id === id);
      if (!await projectTools.check(p, '', false)) return;
      if (draft?.id === id && dirty) await saveCurrent();
      await StartProject(id);
    } else {
      await StopProject(id);
    }
  } catch (error) {
    const name = projects.find((project) => project.id === id)?.name || '项目';
    notify(`${name}：${error}`, true);
  } finally {
    pendingProjects.delete(id);
    renderList();
  }
}

async function detect() {
  try {
    syncFrontendDirectory();
    const directory = $('directory').value.trim();
    const project = draft;
    const detection = await DetectProject(directory);
    if (draft !== project || $('directory').value.trim() !== directory) return;
    const currentName = $('name').value.trim();
    if (detection.name && (!currentName || currentName === autoName)) {
      $('name').value = detection.name;
      autoName = detection.name;
    }
    if (detection.kind) {
      if (!ideaEditedFields.has('kind')) $('kind').value = detection.kind;
      updateSuggestedProjectPort($('kind').value);
      if (!ideaEditedFields.has('packageManager')) $('manager').value = detection.packageManager || 'npm';
      if (!ideaEditedFields.has('portMode')) $('port-mode').value = detection.portMode || 'none';
      if (!ideaEditedFields.has('module') && detection.kind === 'spring-maven' && detection.module) $('module').value = detection.module;
      $('module-options').replaceChildren();
      for (const module of detection.modules || []) {
        const option = document.createElement('option'); option.value = module; $('module-options').append(option);
      }
    } else {
      notify('已识别工作树名称；未找到启动配置文件，请手动选择启动类型和模块目录');
    }
    $('script-options').replaceChildren();
    for (const script of detection.scripts || []) {
      const option = document.createElement('option'); option.value = script; $('script-options').append(option);
    }
    if (!ideaEditedFields.has('script') && detection.scripts?.length && !detection.scripts.includes($('script').value)) {
      $('script').value = detection.script || detection.scripts[0];
    }
    readForm();
    renderForm();
    renderHeader();
    if (!draft.id) await importIDEAConfigurations(false);
  } catch (error) { notify(error, true); }
}

async function detectPairedFrontend(directory) {
  try {
    readForm();
    const project = draft;
    const backendDirectory = draft.directory;
    const frontendDirectory = $('frontend-directory').value.trim();
    const detection = await DetectFrontend(directory || $('frontend-directory').value.trim() || draft.directory);
    if (draft !== project || $('directory').value.trim() !== backendDirectory || $('frontend-directory').value.trim() !== frontendDirectory) return;
    const details = await DetectProject(detection.directory);
    if (draft !== project || $('directory').value.trim() !== backendDirectory || $('frontend-directory').value.trim() !== frontendDirectory) return;
    frontendDraft = { ...blankFrontend(), ...frontendDraft, ...detection,
      nodeHome: frontendDraft?.nodeHome || detection.nodeHome || '', toolPath: frontendDraft?.toolPath || detection.toolPath || '',
      port: frontendDraft?.port ?? nextProjectPort(projects, 'node', [draft.port]),
      environment: frontendDraft?.environment || {}, appArgs: frontendDraft?.appArgs || '' };
    draft.frontend = frontendDraft;
    frontendDirectoryLink.reset(draft.directory, frontendDraft.directory);
    dirty = true;
    $('frontend-script-options').replaceChildren();
    for (const script of details.scripts || []) {
      const option = document.createElement('option'); option.value = script; $('frontend-script-options').append(option);
    }
    renderForm();
    renderHeader();
    notify(`已识别前端：${detection.script}`);
  } catch (error) { notify(error, true); }
}

function bind() {
  $('desktop-settings').onclick = () => desktopUI.open().catch(error=>notify(error,true));
  $('hide-tray').onclick = () => desktopUI.hide();
  EventsOn('desktop:settings', settings=>desktopUI.apply(settings));
  EventsOn('desktop:tray-error', message=>notify(message,true));
  $('source-restart').onclick = () => act('backend-restart');
  $('templates').onclick = () => projectTools.openLibrary().catch(error => notify(error, true));
  $('check-project').onclick = () => {
    try { readForm(); projectTools.check(draft).catch(error => notify(error, true)); }
    catch (error) { notify(error, true); }
  };
  $('run-history').onclick = () => projectTools.openHistory(logServiceID(), serviceProject()?.name || draft.name).catch(error => notify(error, true));
  $('add-project').onclick = () => addProject();
  $('empty-add').onclick = () => addProject();
  $('duplicate').onclick = () => addProject(true);
  $('idea-import').onclick = () => importIDEAConfigurations(true).catch(error => notify(error, true));
  $('idea-close').onclick = () => $('idea-dialog').close();
  $('idea-primary').onchange = renderIDEASelectionHint;
  $('idea-frontend').onchange = renderIDEASelectionHint;
  $('idea-apply').onclick = () => {
    const context = ideaContext;
    if (!context) return;
    const primary = context.choices.primary.find(item => item.id === $('idea-primary').value);
    const frontend = context.choices.frontend.find(item => item.id === $('idea-frontend').value);
    $('idea-dialog').close();
    applyIDEASelection(context, primary, frontend);
  };
  $('save').onclick = async () => { try { await saveCurrent(); } catch (error) { notify(error, true); } };
  $('delete').onclick = async () => {
    if (!confirm(`删除“${draft.name}”的运行配置？`)) return;
    try { const id = draft.id; await DeleteProject(id); terminalUI.forgetProject(id); projects = await ListProjects(); draft = null; current = null; logs = []; render(); notify('运行配置已删除'); }
    catch (error) { notify(error, true); }
  };
  $('start').onclick = () => act('start');
  $('stop').onclick = () => act('stop');
  $('restart').onclick = () => act('restart');
  $('rebuild').onclick = () => act('rebuild');
  $('service-start').onclick = () => act('service-start');
  $('service-stop').onclick = () => act('service-stop');
  $('open-browser').onclick = () => BrowserOpenURL(`http://127.0.0.1:${draft.frontend?.port || draft.port}`);
  $('browse-directory').onclick = async () => { try { const path = await PickDirectory(); if (path) { $('directory').value = path; dirty = true; await detect(); } } catch (error) { notify(error, true); } };
  $('browse-java').onclick = async () => { try { const path = await PickDirectory(); if (path) { $('java-home').value = path; ideaEditedFields.add('javaHome'); dirty = true; } } catch (error) { notify(error, true); } };
  $('browse-tool').onclick = async () => { try { const path = await PickToolFile(); if (path) { $('tool-path').value = path; ideaEditedFields.add('toolPath'); dirty = true; } } catch (error) { notify(error, true); } };
  for (const [button, input, file] of [['browse-node', 'node-home', false], ['browse-node-tool', 'node-tool', true], ['browse-frontend-node', 'frontend-node-home', false], ['browse-frontend-tool', 'frontend-tool', true]]) {
    $(button).onclick = async () => { try { const path = await (file ? PickToolFile() : PickDirectory()); if (path) { $(input).value = path; if (ideaFieldKeys[input]) ideaEditedFields.add(ideaFieldKeys[input]); dirty = true; } } catch (error) { notify(error, true); } };
  }
  $('detect-frontend').onclick = () => detectPairedFrontend($('directory').value.trim());
  $('browse-frontend').onclick = async () => { try { const path = await PickDirectory(); if (path) { $('frontend-directory').value = path; ideaEditedFields.add('frontend.directory'); resetFrontendDirectoryLink(); dirty = true; await detectPairedFrontend(path); } } catch (error) { notify(error, true); } };
  $('frontend-enabled').addEventListener('change', () => { try { readForm(); renderForm(); renderHeader(); renderLogs(); } catch (error) { notify(error, true); } });
  $('frontend-directory').addEventListener('change', () => detectPairedFrontend());
  $('reuse-java').onclick = () => openReuseDialog('javaHome');
  $('reuse-tool').onclick = () => openReuseDialog('toolPath');
  $('reuse-close').onclick = () => $('reuse-dialog').close();
  $('codex-analyze').onclick = analyzeWithCodex;
  $('codex-settings').onclick = openCodexSettings;
  $('codex-close').onclick = () => $('codex-dialog').close();
  $('codex-model').addEventListener('input', renderCodexEfforts);
  $('codex-save').onclick = () => {
    try {
      window.localStorage.setItem(`project-runner-codex-${codexSettingsProject}`, JSON.stringify(codexSelection({ model: $('codex-model').value, effort: $('codex-effort').value })));
      $('codex-dialog').close();
      renderCodexHint();
      notify('Codex 分析设置已保存');
    } catch (error) { notify(error, true); }
  };
  $('browse-config').onclick = async () => { try { const path = await PickConfigFile(); if (path) { $('config-file').value = path; ideaEditedFields.add('configFile'); dirty = true; } } catch (error) { notify(error, true); } };
  $('browse-config-dir').onclick = async () => { try { const path = await PickDirectory(); if (path) { $('config-file').value = path; ideaEditedFields.add('configFile'); dirty = true; } } catch (error) { notify(error, true); } };
  $('directory').addEventListener('change', detect);
  $('port').addEventListener('input', () => { suggestedProjectPort = undefined; });
  $('kind').addEventListener('change', () => { updateSuggestedProjectPort($('kind').value); readForm(); renderForm(); renderHeader(); });
  document.querySelectorAll('.settings input, .settings select, .settings textarea').forEach((el) => {
    el.addEventListener('input', () => { if (ideaFieldKeys[el.id]) ideaEditedFields.add(ideaFieldKeys[el.id]); });
    if (el.id === 'directory') el.addEventListener('input', syncFrontendDirectory);
    if (el.id === 'frontend-directory') el.addEventListener('input', resetFrontendDirectoryLink);
    el.addEventListener('input', () => { dirty = true; if (el.id === 'name') autoName = ''; if (el.id === 'name' || el.id === 'port') { try { readForm(); renderHeader(); } catch (error) { notify(error, true); } } if (['port', 'frontend-auto-proxy', 'frontend-proxy-variable'].includes(el.id)) renderProxyHint(); $('save').textContent = '保存更改'; });
  });
  $('log-search').addEventListener('input', renderLogs);
  $('toggle-config').onclick = () => setConfigOpen(!configOpen);
  const resizer = $('log-resizer');
  resizer.addEventListener('pointerdown', (event) => {
    if (!configOpen) return;
    event.preventDefault();
    const startY = event.clientY;
    const startHeight = displayedLogHeight;
    resizer.setPointerCapture(event.pointerId);
    resizer.classList.add('dragging');
    const move = (next) => setLogHeight(startHeight + startY - next.clientY);
    const end = () => {
      resizer.classList.remove('dragging');
      resizer.removeEventListener('pointermove', move);
      resizer.removeEventListener('pointerup', end);
      resizer.removeEventListener('pointercancel', end);
      setLogHeight(displayedLogHeight, true);
    };
    resizer.addEventListener('pointermove', move);
    resizer.addEventListener('pointerup', end);
    resizer.addEventListener('pointercancel', end);
  });
  resizer.addEventListener('keydown', (event) => {
    if (!configOpen) return;
    const { min, max } = logHeightBounds();
    const next = ({ ArrowUp: displayedLogHeight + 40, ArrowDown: displayedLogHeight - 40, Home: min, End: max })[event.key];
    if (next === undefined) return;
    event.preventDefault();
    setLogHeight(next, true);
  });
  new ResizeObserver(() => { if (draft && configOpen) setLogHeight(preferredLogHeight); }).observe($('content'));
  document.querySelectorAll('.log-filter').forEach((button) => { button.onclick = () => { logMode = button.dataset.logFilter; terminalUI.showLogs(); }; });
  $('issue-view').onclick = () => { logMode = 'focus'; terminalUI.showLogs(); document.querySelector('.logs').scrollIntoView({ behavior: 'smooth', block: 'start' }); };
  EventsOn('project:status', (status) => {
    if ((status.attempt || 1) > (statuses.get(status.id)?.attempt || 1)) {
      projectProblems.delete(status.id);
      alertedRuns.delete(status.id);
    }
    if (isNewRun(statuses.get(status.id), status)) {
      processMetrics.delete(status.id);
      alertedRuns.delete(status.id);
      projectProblems.delete(status.id);
      if (logServiceID() === status.id) { logs = []; renderLogs(); }
    }
    statuses.set(status.id, status);
    renderList();
    if (draft && serviceIDs(draft).includes(status.id)) { renderHeader(); if (!dirty) renderForm(); }
    if (logServiceID() === status.id) renderIssue();
  });
  EventsOn('project:preflight', report => { if (!report.allowed) projectTools.showReport(report); });
  EventsOn('project:ready', url => BrowserOpenURL(url));
  EventsOn('project:log', ({ id, line }) => {
    const level = logLevel(line);
    const currentAttempt = currentAttemptLog(line, statuses.get(id));
    if (currentAttempt && (level === 'error' || (level === 'warn' && !projectProblems.has(id)))) {
      if (projectProblems.get(id) !== 'error') { projectProblems.set(id, level); renderList(); if (draft && serviceIDs(draft).includes(id)) renderHeader(); }
    }
    if (currentAttempt && level === 'error' && !alertedRuns.has(id)) {
      alertedRuns.add(id);
      const parent = projects.find((project) => serviceIDs(project).includes(id));
      const projectName = parent ? `${parent.name}${id.endsWith(':frontend') ? ' · 前端' : ' · 后端'}` : '项目';
      notify(`${projectName} 检测到运行错误，请查看重点日志`, true);
    }
    if (logServiceID() !== id) return;
    logs.push(line);
    if (logs.length > 2500) logs = logs.slice(-2000);
    renderIssue();
    renderLogCounts();
    const query = $('log-search').value.toLowerCase();
    if (!matchesLog(line, query)) return;
    const output = $('log-output');
    if (output.querySelector('.log-placeholder')) output.replaceChildren();
    appendLogRow(line, output);
    if (output.scrollHeight - output.scrollTop - output.clientHeight < 120) output.scrollTop = output.scrollHeight;
  });
}

bind();
desktopUI.restore().catch(error=>notify(error,true));
setInterval(refreshProcessMetrics,2000);
terminalUI.restore().catch((error) => notify(`恢复终端失败：${error}`, true));
setInterval(refreshStartupTimes, 500);
Promise.all([ListProjects(), GetStatuses()]).then(([items, states]) => {
  projects = items || [];
  statuses = new Map((states || []).map((state) => [state.id, state]));
  render();
}).catch((error) => notify(error, true));
