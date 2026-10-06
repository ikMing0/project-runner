import './style.css';
import { isNewRun, actionableError } from './run-state.mjs';
import {
  ListProjects, SaveProject, DeleteProject, GetStatuses, GetLogs,
  PickDirectory, PickConfigFile, PickToolFile, DetectProject, StartProject, StopProject, RestartProject, RebuildProject,
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
              <label class="field wide java-field"><span>本地配置文件 / 目录</span><div class="input-action"><input id="config-file" placeholder="例如 D:\\project\\file\\config\\application.properties"><button id="browse-config" class="button small browse-button" type="button">${browseIcon}选文件</button><button id="browse-config-dir" class="button small browse-button" type="button">${browseIcon}选目录</button></div></label>
              <label class="field java-field"><span>配置路径属性名</span><input id="config-property" value="application.config.path"></label>
              <label class="field java-field"><span>其他 JVM 参数</span><textarea id="jvm-args" rows="3" placeholder="每行一个参数，例如 -Xmx512m"></textarea></label>
              <label class="field"><span>其他应用参数</span><textarea id="app-args" rows="3" placeholder="每行一个参数"></textarea></label>
              <label class="field"><span>环境变量</span><textarea id="environment" rows="3" placeholder="每行 KEY=VALUE"></textarea></label>
            </div>
          </section>
          <div id="log-resizer" class="log-resizer" role="separator" aria-label="调整运行日志高度" aria-orientation="horizontal" aria-controls="log-output" tabindex="0" title="上下拖动，调整运行日志高度"><span></span></div>
          <section class="logs panel">
            <div class="log-toolbar"><div><h2>运行日志</h2><span id="log-summary">等待启动</span></div><div class="log-actions"><input id="log-search" placeholder="搜索日志"><button id="open-browser" class="button small">打开页面</button></div></div>
            <div class="log-filterbar"><div class="log-filters"><button class="log-filter selected" data-log-filter="focus" type="button">重点</button><button class="log-filter" data-log-filter="error" type="button">仅错误</button><button class="log-filter" data-log-filter="all" type="button">全部</button></div><span id="log-filter-counts">普通日志会在重点视图中折叠</span></div>
            <div id="log-output" class="log-output"><div class="log-placeholder">启动项目后，日志将在这里实时显示。</div></div>
          </section>
        </div>
      </div>
    </main>
  </div>
  <dialog id="reuse-dialog" class="reuse-dialog" aria-labelledby="reuse-title"><div class="reuse-dialog-head"><div><h2 id="reuse-title">复用配置</h2><p>选择已保存项目中的路径</p></div><button id="reuse-close" class="button small" type="button" aria-label="关闭">✕</button></div><div id="reuse-options" class="reuse-options"></div></dialog>
  <div id="toast" class="toast hidden"></div>
`;

const $ = (id) => document.getElementById(id);
let projects = [];
let statuses = new Map();
let current = null;
let draft = null;
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
  return { min: 170, max: Math.max(170, Math.floor(available - 104)) };
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
    packageManager: 'npm', script: 'dev', portMode: 'vite', jvmArgs: '', appArgs: '', environment: {} };
}

function notify(message, error = false) {
  $('toast').textContent = String(message);
  $('toast').className = `toast ${error ? 'error' : ''}`;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => $('toast').classList.add('hidden'), 4200);
}

function stateFor(id) { return statuses.get(id)?.state || 'stopped'; }
function isActive(id) { return ['building', 'starting', 'running'].includes(stateFor(id)); }
function usesPort(project) { return project.kind !== 'node' || project.portMode !== 'none'; }
function stateText(state) { return ({ building: '构建中', starting: '启动中', running: '运行中', failed: '启动失败', stopped: '未运行' })[state] || '未运行'; }
function kindText(kind) { return ({ 'spring-maven': 'SPRING · MAVEN', 'spring-gradle': 'SPRING · GRADLE', node: 'VUE / NODE' })[kind] || '运行配置'; }
function formatStartupDuration(ms) {
  const seconds = Math.max(0, ms) / 1000;
  if (seconds < 60) return `${seconds.toFixed(1)} 秒`;
  return `${Math.floor(seconds / 60)} 分 ${Math.floor(seconds % 60)} 秒`;
}
function startupText(status) {
  if (['building', 'starting'].includes(status?.state) && status.startedAt) {
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
  const list = $('project-list');
  list.replaceChildren();
  for (const project of projects) {
    const item = document.createElement('div');
    item.className = `project-item ${current === project.id ? 'selected' : ''}`;
    const select = document.createElement('button');
    select.type = 'button';
    select.className = 'project-select';
    select.title = `查看 ${project.name} 的日志与配置`;
    select.setAttribute('aria-label', select.title);
    const top = document.createElement('div');
    top.className = 'project-item-top';
    const dot = document.createElement('span');
    dot.className = `state-dot ${stateFor(project.id)}`;
    const name = document.createElement('strong');
    name.textContent = project.name;
    top.append(dot, name);
    const problem = projectProblems.get(project.id);
    if (problem) {
      const mark = document.createElement('span');
      mark.className = `project-alert-mark ${problem}`;
      mark.textContent = problem === 'error' ? '错误' : '警告';
      top.append(mark);
    }
    const sub = document.createElement('small');
    sub.textContent = `${project.kind === 'node' ? 'Vue / Node' : 'Spring Boot'} · :${project.port}`;
    select.append(top, sub);
    const status = statuses.get(project.id);
    const timing = startupText(status);
    if (timing) {
      const duration = document.createElement('span');
      duration.className = `project-startup-time ${['building', 'starting'].includes(status.state) ? 'starting' : ''}`;
      duration.dataset.projectId = project.id;
      duration.textContent = timing;
      duration.title = timing;
      select.append(duration);
    }
    select.onclick = () => selectProject(project.id);
    const active = isActive(project.id);
    const quick = document.createElement('button');
    quick.type = 'button';
    quick.className = `project-quick-action ${active ? 'stop' : 'start'}`;
    quick.textContent = pendingProjects.has(project.id) ? '处理中' : active ? '■ 停止' : '▶ 启动';
    quick.title = `${active ? '停止' : '启动'} ${project.name}`;
    quick.setAttribute('aria-label', quick.title);
    quick.disabled = pendingProjects.has(project.id);
    quick.onclick = () => quickAction(project.id, active ? 'stop' : 'start');
    item.append(select, quick);
    list.append(item);
  }
}

async function selectProject(id) {
  if (dirty && !confirm('当前配置尚未保存，确定切换项目吗？')) return;
  current = id;
  draft = structuredClone(projects.find((p) => p.id === id));
  dirty = false;
  autoName = '';
  configOpen = false;
  logs = await GetLogs(id);
  render();
}

function readForm() {
  if (!draft) return;
  draft.name = $('name').value.trim();
  draft.directory = $('directory').value.trim();
  draft.kind = $('kind').value;
  draft.port = Number($('port').value);
  draft.module = $('module').value.trim();
  draft.javaHome = $('java-home').value.trim();
  draft.toolPath = $('tool-path').value.trim();
  draft.packageManager = $('manager').value;
  draft.script = $('script').value.trim();
  draft.portMode = $('port-mode').value;
  draft.configFile = $('config-file').value.trim();
  draft.configProperty = $('config-property').value.trim();
  draft.jvmArgs = $('jvm-args').value;
  draft.appArgs = $('app-args').value;
  const environment = {};
  for (const line of $('environment').value.split('\n')) {
    if (!line.trim()) continue;
    const equals = line.indexOf('=');
    if (equals < 1) throw new Error(`环境变量格式应为 KEY=VALUE：${line}`);
    environment[line.slice(0, equals).trim()] = line.slice(equals + 1);
  }
  draft.environment = environment;
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
    port: p.port, module: p.module, 'java-home': p.javaHome, 'tool-path': p.toolPath, manager: p.packageManager || 'npm',
    script: p.script || 'dev', 'port-mode': p.portMode || 'none', 'config-file': p.configFile,
    'config-property': p.configProperty || 'application.config.path', 'jvm-args': p.jvmArgs,
    'app-args': p.appArgs, environment: Object.entries(p.environment || {}).map(([k, v]) => `${k}=${v}`).join('\n') })) {
    $(field).value = value ?? '';
  }
  const java = p.kind !== 'node';
  document.querySelectorAll('.java-field').forEach((el) => el.classList.toggle('hidden', !java));
  document.querySelectorAll('.node-field').forEach((el) => el.classList.toggle('hidden', java));
  const active = p.id && isActive(p.id);
  document.querySelectorAll('.settings input, .settings select, .settings textarea, .settings button').forEach((el) => { el.disabled = !!active; });
  $('reuse-java').disabled = !!active || reusablePaths('javaHome').length === 0;
  $('reuse-tool').disabled = !!active || reusablePaths('toolPath').length === 0;
  $('duplicate').disabled = false;
  $('delete').disabled = !!active || !p.id;
  $('save').textContent = dirty ? '保存更改' : '保存配置';
}

function renderHeader() {
  const p = draft;
  $('header-kind').textContent = kindText(p.kind);
  $('header-name').textContent = p.name || '新项目';
  $('header-path').textContent = p.directory || '请选择项目目录';
  const status = statuses.get(p.id) || { state: 'stopped' };
  $('status-pill').textContent = stateText(status.state);
  $('status-pill').className = `status-pill ${status.state}`;
  $('start').disabled = !!p.id && isActive(p.id);
  $('restart').disabled = !p.id || !isActive(p.id);
  $('stop').disabled = !p.id || !isActive(p.id);
  $('rebuild').classList.toggle('hidden', p.kind !== 'spring-maven');
  $('rebuild').disabled = !p.id || pendingProjects.has(p.id);
  $('open-browser').disabled = !p.id || status.state !== 'running' || !usesPort(p);
  $('log-summary').textContent = status.state === 'failed' ? (status.error || '启动失败') : `${stateText(status.state)} · ${usesPort(p) ? `端口 ${p.port}` : '未指定端口'}`;
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
    const selected = button.dataset.logFilter === logMode;
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
  if (!draft) return;
  renderHeader();
  renderForm();
  renderLogs();
  setConfigOpen(configOpen);
}

function addProject(copy = false) {
  if (dirty && !confirm('当前配置尚未保存，确定新建吗？')) return;
  draft = copy && draft ? { ...structuredClone(draft), id: '', name: `${draft.name} 副本` } : blankProject();
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
    const detection = await DetectProject($('directory').value.trim());
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
      $('script').value = detection.scripts.includes('dev') ? 'dev' : detection.scripts.includes('serve') ? 'serve' : detection.scripts[0];
    }
    readForm();
    renderForm();
    renderHeader();
  } catch (error) { notify(error, true); }
}

function bind() {
  $('add-project').onclick = () => addProject();
  $('empty-add').onclick = () => addProject();
  $('duplicate').onclick = () => addProject(true);
  $('save').onclick = async () => { try { await saveCurrent(); } catch (error) { notify(error, true); } };
  $('delete').onclick = async () => {
    if (!confirm(`删除“${draft.name}”的运行配置？`)) return;
    try { await DeleteProject(draft.id); projects = await ListProjects(); draft = null; current = null; logs = []; render(); notify('运行配置已删除'); }
    catch (error) { notify(error, true); }
  };
  $('start').onclick = () => act('start');
  $('stop').onclick = () => act('stop');
  $('restart').onclick = () => act('restart');
  $('rebuild').onclick = () => act('rebuild');
  $('open-browser').onclick = () => BrowserOpenURL(`http://127.0.0.1:${draft.port}`);
  $('browse-directory').onclick = async () => { try { const path = await PickDirectory(); if (path) { $('directory').value = path; dirty = true; await detect(); } } catch (error) { notify(error, true); } };
  $('browse-java').onclick = async () => { try { const path = await PickDirectory(); if (path) { $('java-home').value = path; dirty = true; } } catch (error) { notify(error, true); } };
  $('browse-tool').onclick = async () => { try { const path = await PickToolFile(); if (path) { $('tool-path').value = path; dirty = true; } } catch (error) { notify(error, true); } };
  $('reuse-java').onclick = () => openReuseDialog('javaHome');
  $('reuse-tool').onclick = () => openReuseDialog('toolPath');
  $('reuse-close').onclick = () => $('reuse-dialog').close();
  $('browse-config').onclick = async () => { try { const path = await PickConfigFile(); if (path) { $('config-file').value = path; dirty = true; } } catch (error) { notify(error, true); } };
  $('browse-config-dir').onclick = async () => { try { const path = await PickDirectory(); if (path) { $('config-file').value = path; dirty = true; } } catch (error) { notify(error, true); } };
  $('directory').addEventListener('change', detect);
  $('kind').addEventListener('change', () => { readForm(); renderForm(); renderHeader(); });
  document.querySelectorAll('.settings input, .settings select, .settings textarea').forEach((el) => {
    el.addEventListener('input', () => { dirty = true; if (el.id === 'name') autoName = ''; if (el.id === 'name' || el.id === 'port') { readForm(); renderHeader(); } $('save').textContent = '保存更改'; });
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
  document.querySelectorAll('.log-filter').forEach((button) => { button.onclick = () => { logMode = button.dataset.logFilter; renderLogs(); }; });
  $('issue-view').onclick = () => { logMode = 'focus'; renderLogs(); document.querySelector('.logs').scrollIntoView({ behavior: 'smooth', block: 'start' }); };
  EventsOn('project:status', (status) => {
    if (isNewRun(statuses.get(status.id), status)) {
      alertedRuns.delete(status.id);
      projectProblems.delete(status.id);
      if (draft?.id === status.id) { logs = []; renderLogs(); }
    }
    statuses.set(status.id, status);
    renderList();
    if (draft?.id === status.id) { renderHeader(); renderForm(); }
  });
  EventsOn('project:log', ({ id, line }) => {
    const level = logLevel(line);
    if (level === 'error' || (level === 'warn' && !projectProblems.has(id))) {
      if (projectProblems.get(id) !== 'error') { projectProblems.set(id, level); renderList(); }
    }
    if (level === 'error' && !alertedRuns.has(id)) {
      alertedRuns.add(id);
      const projectName = projects.find((project) => project.id === id)?.name || '项目';
      notify(`${projectName} 检测到运行错误，请查看重点日志`, true);
    }
    if (draft?.id !== id) return;
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
setInterval(refreshStartupTimes, 500);
Promise.all([ListProjects(), GetStatuses()]).then(([items, states]) => {
  projects = items || [];
  statuses = new Map((states || []).map((state) => [state.id, state]));
  render();
}).catch((error) => notify(error, true));
