import './style.css';
import { isNewRun, actionableError, serviceIDs, activeState, groupState } from './run-state.mjs';
import { TerminalConsole } from './terminal-console.mjs';
import { FrontendDirectoryLink } from './frontend-directory.mjs';
import { moveProject, ProjectOrderController } from './project-order.mjs';
import {
  ListProjects, SaveProject, DeleteProject, ReorderProjects, GetStatuses, GetLogs,
  PickDirectory, PickConfigFile, PickToolFile, DetectProject, DetectFrontend, StartProject, StopProject, RestartProject, RebuildProject, StartService, StopService,
  NewTerminal, GetTerminals, GetTerminalOutput, WriteTerminal, ResizeTerminal, CloseTerminal,
} from '../wailsjs/go/main/App';
import { EventsOn, BrowserOpenURL } from '../wailsjs/runtime/runtime';

const app = document.querySelector('#app');
const browseIcon = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M3.5 7.5A2.5 2.5 0 0 1 6 5h4l2 2h6a2.5 2.5 0 0 1 2.5 2.5v8A2.5 2.5 0 0 1 18 20H6a2.5 2.5 0 0 1-2.5-2.5z"/><path d="M3.5 10h17"/></svg>`;
app.innerHTML = `
  <div class="shell">
    <aside class="sidebar">
      <div class="brand"><div class="brand-icon">▶</div><div><strong>项目运行台</strong><small>Windows · 本地运行</small></div></div>
      <button id="add-project" class="button primary full">＋ 添加项目</button>
      <div class="side-label"><span>运行配置</span><span id="project-count">0</span></div>
      <div id="project-list" class="project-list"></div>
      <div class="side-footer"><span class="pulse-dot"></span>关闭窗口时停止所有项目</div>
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
          <div class="header-actions"><span id="status-pill" class="status-pill">未运行</span><button id="start" class="button primary">▶ 启动</button><button id="restart" class="button">↻ 重启</button><button id="rebuild" class="button" title="清理产物后重新构建并启动">重新构建</button><button id="stop" class="button danger">■ 停止</button><button id="toggle-config" class="button config-toggle" type="button" aria-controls="settings-panel" aria-expanded="false">配置</button></div>
        </header>
        <div id="issue-banner" class="issue-banner hidden" role="status" aria-live="polite"><span class="issue-icon">!</span><div class="issue-copy"><strong id="issue-title"></strong><span id="issue-message"></span></div><button id="issue-view" class="button small">查看日志</button></div>
        <div id="content" class="content config-collapsed">
          <section id="settings-panel" class="settings panel">
            <div class="section-title"><div><h2>启动配置</h2><p>界面设置会在启动时转换为相应参数</p></div><div class="section-actions"><button id="duplicate" class="text-button">复制配置</button><button id="delete" class="text-button destructive">删除</button><button id="save" class="button secondary">保存配置</button></div></div>
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
            </div>
            <div id="frontend-section" class="frontend-section java-field">
              <div class="section-title"><div><label class="check-field"><input id="frontend-enabled" type="checkbox"><strong>启用配套前端</strong></label><p>作为一组启停，也可在日志页单独操作前端或后端</p></div><button id="detect-frontend" class="button small" type="button">识别配套前端</button></div>
              <div id="frontend-fields" class="form-grid hidden">
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
              </div>
            </div>
          </section>
          <div id="log-resizer" class="log-resizer" role="separator" aria-label="调整运行日志高度" aria-orientation="horizontal" aria-controls="log-output" tabindex="0" title="上下拖动，调整运行日志高度"><span></span></div>
          <section class="logs panel">
            <div id="service-bar" class="service-bar hidden"><div id="service-tabs" class="service-tabs" role="tablist" aria-label="服务日志"></div><div class="service-actions"><button id="service-start" class="button small">启动当前服务</button><button id="service-stop" class="button small danger">停止当前服务</button></div></div>
            <div class="log-toolbar"><div class="output-heading"><h2 id="output-title">运行日志</h2><span id="log-summary">等待启动</span></div><div class="log-actions"><input id="log-search" placeholder="搜索日志"><button id="open-browser" class="button small">打开页面</button></div></div>
            <div class="log-filterbar"><div class="log-view-controls"><div class="log-filters"><button class="log-filter selected" data-log-filter="focus" type="button">重点</button><button class="log-filter" data-log-filter="error" type="button">仅错误</button><button class="log-filter" data-log-filter="all" type="button">全部</button></div><button id="terminal-toggle" class="terminal-toggle" type="button" aria-controls="terminal-pane" aria-pressed="false"><span aria-hidden="true">&gt;_</span> 终端</button></div><span id="log-filter-counts">普通日志会在重点视图中折叠</span><span id="terminal-shortcuts" class="hidden">Ctrl+C 中断 · Ctrl+V 粘贴</span></div>
            <div id="log-output" class="log-output"><div class="log-placeholder">启动项目后，日志将在这里实时显示。</div></div>
            <div id="terminal-pane" class="terminal-pane hidden"><div class="terminal-tabbar"><div id="terminal-tabs" class="terminal-tabs" role="tablist" aria-label="终端标签页"></div><button id="terminal-new" class="button small" type="button">＋ 新标签页</button></div><div id="terminal-workspace" class="terminal-workspace"></div></div>
          </section>
        </div>
      </div>
    </main>
  </div>
  <dialog id="reuse-dialog" class="reuse-dialog" aria-labelledby="reuse-title"><div class="reuse-dialog-head"><div><h2 id="reuse-title">复用配置</h2><p>选择已保存项目中的路径</p></div><button id="reuse-close" class="button small" type="button" aria-label="关闭">✕</button></div><div id="reuse-options" class="reuse-options"></div></dialog>
  <div id="toast" class="toast hidden"></div>
`;

const $ = (id) => document.getElementById(id);
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
let dirty = false;
let autoName = '';
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
  return { id: '', name: '', directory: '', kind: 'spring-maven', port: 8080,
    configFile: '', configProperty: 'application.config.path', javaHome: '', toolPath: '', module: '',
    packageManager: 'npm', script: 'dev', portMode: 'vite', nodeHome: '', jvmArgs: '', appArgs: '', environment: {}, frontend: null };
}

function blankFrontend() {
  return { directory: '', port: 82, packageManager: 'npm', script: 'dev:vite', portMode: 'vite', nodeHome: '', toolPath: '',
    appArgs: '', environment: {}, autoProxy: true, proxyVariable: 'VUE_APP_BASE_API_TARGET' };
}

function logServiceID() { return logSide === 'frontend' && draft?.frontend ? `${draft.id}:frontend` : draft?.id; }
function groupActive(project) { return serviceIDs(project).some(isActive); }
function canStartGroup(project) { return serviceIDs(project).some((id) => !isActive(id)); }
function serviceProject() { return logSide === 'frontend' && draft?.frontend ? { ...draft.frontend, id: logServiceID(), kind: 'node' } : draft; }
async function selectLogSide(side) {
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

function notify(message, error = false) {
  $('toast').textContent = String(message);
  $('toast').className = `toast ${error ? 'error' : ''}`;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => $('toast').classList.add('hidden'), 4200);
}

function stateFor(id) { return statuses.get(id)?.state || 'stopped'; }
function isActive(id) { return activeState(stateFor(id)); }
function usesPort(project) { return project.kind !== 'node' || project.portMode !== 'none'; }
function stateText(state) { return ({ checking: '检查中', building: '构建中', starting: '启动中', running: '运行中', partial: '部分运行', failed: '启动失败', stopped: '未运行' })[state] || '未运行'; }
function kindText(kind) { return ({ 'spring-maven': 'SPRING · MAVEN', 'spring-gradle': 'SPRING · GRADLE', node: 'VUE / NODE' })[kind] || '运行配置'; }
function formatStartupDuration(ms) {
  const seconds = Math.max(0, ms) / 1000;
  if (seconds < 60) return `${seconds.toFixed(1)} 秒`;
  return `${Math.floor(seconds / 60)} 分 ${Math.floor(seconds % 60)} 秒`;
}
function startupText(status) {
  if (['checking', 'building', 'starting'].includes(status?.state) && status.startedAt) {
    return `${stateText(status.state)} · ${formatStartupDuration(Date.now() - status.startedAt)}`;
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
  current = id;
  draft = structuredClone(projects.find((p) => p.id === id));
  frontendDraft = draft.frontend ? structuredClone(draft.frontend) : null;
  frontendDirectoryLink.reset(draft.directory, frontendDraft?.directory);
  dirty = false;
  autoName = '';
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
  frontendDraft = {
    directory: $('frontend-directory').value.trim(), port: Number($('frontend-port').value),
    script: $('frontend-script').value.trim(), packageManager: $('frontend-manager').value, portMode: $('frontend-port-mode').value,
    nodeHome: $('frontend-node-home').value.trim(), toolPath: $('frontend-tool').value.trim(), appArgs: $('frontend-args').value,
    environment: draft.kind !== 'node' && $('frontend-enabled').checked ? readEnvironment('frontend-environment') : (frontendDraft?.environment || {}), autoProxy: $('frontend-auto-proxy').checked, proxyVariable: $('frontend-proxy-variable').value.trim(),
  };
  draft.frontend = draft.kind !== 'node' && $('frontend-enabled').checked ? frontendDraft : null;
  if (!draft.frontend) logSide = 'backend';
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
  $('log-summary').textContent = status.state === 'failed' ? (status.error || '启动失败') : `${stateText(status.state)} · ${usesPort(service) ? `端口 ${service.port}` : '未指定端口'}`;
  $('service-bar').classList.toggle('hidden', !p.frontend);
  $('service-tabs').replaceChildren();
  if (p.frontend) {
    for (const [side, id, label] of [['backend', p.id, '后端'], ['frontend', `${p.id}:frontend`, '前端']]) {
      const tab = document.createElement('button');
      tab.className = `service-tab ${logSide === side ? 'selected' : ''}`;
      tab.setAttribute('role', 'tab');
      tab.setAttribute('aria-selected', String(logSide === side));
      tab.textContent = `${label} · ${stateText(stateFor(id))}${projectProblems.get(id) === 'error' ? ' · 错误' : ''}`;
      tab.onclick = () => selectLogSide(side).catch((error) => notify(error, true));
      $('service-tabs').append(tab);
    }
  }
  $('service-start').disabled = pending || isActive(service.id);
  $('service-stop').disabled = pending || !isActive(service.id);
  terminalUI.setContext(terminalContext());
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

function logLevel(line) { return line.level || (line.source === 'system' ? 'system' : 'info'); }
function matchesLog(line, query) {
  const level = logLevel(line);
  if (logMode === 'error' && level !== 'error') return false;
  if (logMode === 'focus' && !['error', 'warn', 'system'].includes(level)) return false;
  return !query || line.text.toLowerCase().includes(query);
}

function renderIssue() {
  const banner = $('issue-banner');
  const errors = logs.filter((line) => logLevel(line) === 'error');
  const warnings = logs.filter((line) => logLevel(line) === 'warn');
  const latest = errors.find((line) => actionableError(line.text)) || errors[0] || warnings[0];
  banner.classList.toggle('hidden', !latest);
  if (!latest) return;
  banner.classList.toggle('warning', !errors.length);
  $('issue-title').textContent = errors.length ? `检测到错误记录 ${errors.length} 行` : `检测到警告 ${warnings.length} 行`;
  $('issue-message').textContent = latest.text.trim();
  $('issue-message').title = latest.text.trim();
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
  const normalized = line.text.trim().replace(/^\d{4}-\d{2}-\d{2}T\S+\s+(?:TRACE|DEBUG|INFO|WARN|ERROR)\s+\d+\s+---\s*/, '');
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
  message.textContent = line.text;
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
  draft = copy && draft ? { ...structuredClone(draft), id: '', name: `${draft.name} 副本` } : blankProject();
  frontendDraft = draft.frontend ? structuredClone(draft.frontend) : null;
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
  configOpen = false;
  projects = await ListProjects();
  render();
  notify('运行配置已保存');
  return saved;
}

async function act(action) {
  const id = draft?.id;
  if (id && pendingProjects.has(id)) return;
  if (id) pendingProjects.add(id);
  renderHeader();
  try {
    if (action === 'start') {
      const saved = dirty || !draft.id ? await saveCurrent() : draft;
      await StartProject(saved.id);
    } else if (action === 'stop') {
      await StopProject(draft.id);
    } else if (action === 'restart') {
      await RestartProject(draft.id);
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
  finally { if (id) pendingProjects.delete(id); renderHeader(); renderList(); }
}

async function quickAction(id, action) {
  if (pendingProjects.has(id)) return;
  pendingProjects.add(id);
  renderList();
  try {
    if (action === 'start') {
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
      $('kind').value = detection.kind;
      $('manager').value = detection.packageManager || 'npm';
      $('port-mode').value = detection.portMode || 'none';
      if (detection.kind === 'spring-maven' && detection.module) $('module').value = detection.module;
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
    if (detection.scripts?.length && !detection.scripts.includes($('script').value)) {
      $('script').value = detection.script || detection.scripts[0];
    }
    readForm();
    renderForm();
    renderHeader();
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
      port: draft.frontend?.port || detection.port,
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
  $('add-project').onclick = () => addProject();
  $('empty-add').onclick = () => addProject();
  $('duplicate').onclick = () => addProject(true);
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
  $('browse-java').onclick = async () => { try { const path = await PickDirectory(); if (path) { $('java-home').value = path; dirty = true; } } catch (error) { notify(error, true); } };
  $('browse-tool').onclick = async () => { try { const path = await PickToolFile(); if (path) { $('tool-path').value = path; dirty = true; } } catch (error) { notify(error, true); } };
  for (const [button, input, file] of [['browse-node', 'node-home', false], ['browse-node-tool', 'node-tool', true], ['browse-frontend-node', 'frontend-node-home', false], ['browse-frontend-tool', 'frontend-tool', true]]) {
    $(button).onclick = async () => { try { const path = await (file ? PickToolFile() : PickDirectory()); if (path) { $(input).value = path; dirty = true; } } catch (error) { notify(error, true); } };
  }
  $('detect-frontend').onclick = () => detectPairedFrontend($('directory').value.trim());
  $('browse-frontend').onclick = async () => { try { const path = await PickDirectory(); if (path) { $('frontend-directory').value = path; resetFrontendDirectoryLink(); dirty = true; await detectPairedFrontend(path); } } catch (error) { notify(error, true); } };
  $('frontend-enabled').addEventListener('change', () => { try { readForm(); renderForm(); renderHeader(); renderLogs(); } catch (error) { notify(error, true); } });
  $('frontend-directory').addEventListener('change', () => detectPairedFrontend());
  $('reuse-java').onclick = () => openReuseDialog('javaHome');
  $('reuse-tool').onclick = () => openReuseDialog('toolPath');
  $('reuse-close').onclick = () => $('reuse-dialog').close();
  $('browse-config').onclick = async () => { try { const path = await PickConfigFile(); if (path) { $('config-file').value = path; dirty = true; } } catch (error) { notify(error, true); } };
  $('browse-config-dir').onclick = async () => { try { const path = await PickDirectory(); if (path) { $('config-file').value = path; dirty = true; } } catch (error) { notify(error, true); } };
  $('directory').addEventListener('change', detect);
  $('kind').addEventListener('change', () => { readForm(); renderForm(); renderHeader(); });
  document.querySelectorAll('.settings input, .settings select, .settings textarea').forEach((el) => {
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
    if (isNewRun(statuses.get(status.id), status)) {
      alertedRuns.delete(status.id);
      projectProblems.delete(status.id);
      if (logServiceID() === status.id) { logs = []; renderLogs(); }
    }
    statuses.set(status.id, status);
    renderList();
    if (draft && serviceIDs(draft).includes(status.id)) { renderHeader(); if (!dirty) renderForm(); }
  });
  EventsOn('project:log', ({ id, line }) => {
    const level = logLevel(line);
    if (level === 'error' || (level === 'warn' && !projectProblems.has(id))) {
      if (projectProblems.get(id) !== 'error') { projectProblems.set(id, level); renderList(); if (draft && serviceIDs(draft).includes(id)) renderHeader(); }
    }
    if (level === 'error' && !alertedRuns.has(id)) {
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
terminalUI.restore().catch((error) => notify(`恢复终端失败：${error}`, true));
setInterval(refreshStartupTimes, 500);
Promise.all([ListProjects(), GetStatuses()]).then(([items, states]) => {
  projects = items || [];
  statuses = new Map((states || []).map((state) => [state.id, state]));
  render();
}).catch((error) => notify(error, true));
