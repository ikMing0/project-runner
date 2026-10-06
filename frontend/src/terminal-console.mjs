import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import '@xterm/xterm/css/xterm.css';
import { TerminalStream, splitTerminalInput } from './terminal-stream.mjs';

const $ = (id) => document.getElementById(id);

export class TerminalConsole {
  constructor({ api, prepare, notify, onLogs, onLayout, events }) {
    this.api = api;
    this.prepare = prepare;
    this.notify = notify;
    this.onLogs = onLogs;
    this.onLayout = onLayout;
    this.context = null;
    this.entries = new Map();
    this.views = new Map();
    this.earlyOutput = new Map();
    this.earlyExits = new Map();
    this.creating = false;
    $('terminal-toggle').onclick = () => this.open().catch((error) => notify(error, true));
    $('terminal-new').onclick = () => this.newTab().catch((error) => notify(error, true));
    events('terminal:output', (chunk) => {
      const entry = this.entries.get(chunk.id);
      if (entry) entry.stream.receive(chunk);
      else if (this.creating) {
        const pending = this.earlyOutput.get(chunk.id) || [];
        if (pending.length < 128) pending.push(chunk);
        this.earlyOutput.set(chunk.id, pending);
      }
    });
    events('terminal:exit', (info) => {
      const entry = this.entries.get(info.id);
      if (entry) this.markExited(entry, info);
      else if (this.creating) this.earlyExits.set(info.id, info);
    });
    new ResizeObserver(() => this.scheduleFit()).observe($('terminal-workspace'));
  }

  async restore() {
    const items = await this.api.GetTerminals();
    for (const info of items || []) await this.addEntry(info);
    this.render();
  }

  setContext(context) {
    this.context = context;
    this.render();
  }

  view(projectID = this.context?.projectId) {
    if (!this.views.has(projectID)) this.views.set(projectID, { mode: 'logs', active: null });
    return this.views.get(projectID);
  }

  isVisible() {
    return !!this.context?.projectId && this.view().mode === 'terminal';
  }

  showLogs() {
    this.view().mode = 'logs';
    this.render();
    this.onLogs();
  }

  async open() {
    if (this.creating) return;
    const tabs = this.projectEntries();
    if (!tabs.length) return this.newTab();
    this.view().mode = 'terminal';
    this.view().active ||= tabs[0].info.id;
    this.render();
    this.focus();
  }

  async newTab(create = context => this.api.NewTerminal(context.serviceId, 80, 24)) {
    if (this.creating) return;
    this.creating = true;
    this.render();
    try {
      const context = await this.prepare();
      const info = await create(context);
      await this.addEntry(info);
      const view = this.view(context.projectId);
      view.mode = 'terminal';
      view.active = info.id;
    } finally {
      this.creating = false;
      this.earlyOutput.clear();
      this.earlyExits.clear();
      this.render();
      this.focus();
    }
  }

  async addEntry(info) {
    const screen = document.createElement('div');
    screen.className = 'terminal-screen hidden';
    screen.dataset.terminalId = info.id;
    $('terminal-workspace').append(screen);
    const terminal = new Terminal({
      cursorBlink: true, scrollback: 2000, fontSize: 12,
      fontFamily: '"Cascadia Code", Consolas, "Microsoft YaHei", monospace',
      theme: { background: '#0b1422', foreground: '#dbe5f4', cursor: '#8cb9f8',
        selectionBackground: '#32527f', black: '#1b2535', brightBlack: '#8193ac',
        red: '#f4838c', green: '#71dbaa', yellow: '#e9c886', blue: '#78aef4',
        magenta: '#bfa2ef', cyan: '#81d4e0', white: '#dbe5f4' },
    });
    const fit = new FitAddon();
    terminal.loadAddon(fit);
    terminal.open(screen);
    const entry = { info, terminal, fit, screen, input: Promise.resolve(), resize: Promise.resolve(), closing: false, closed: false };
    entry.stream = new TerminalStream((bytes) => terminal.write(bytes));
    this.entries.set(info.id, entry);
    terminal.onData((data) => {
      for (const chunk of splitTerminalInput(data)) {
        entry.input = entry.input.then(() => {
          if (!entry.closed && !entry.closing && entry.info.state === 'running') return this.api.WriteTerminal(info.id, chunk);
        }).catch((error) => {
          if (!entry.closed && !entry.closing && entry.info.state === 'running') this.notify(`终端输入失败：${error}`, true);
        });
      }
    });
    terminal.onResize(({ cols, rows }) => {
      entry.resize = entry.resize.then(() => {
        if (!entry.closed && !entry.closing && entry.info.state === 'running') return this.api.ResizeTerminal(info.id, cols, rows);
      }).catch(() => {});
    });
    terminal.attachCustomKeyEventHandler((event) => {
      if (event.type === 'keydown' && event.ctrlKey && event.key.toLowerCase() === 'c' && terminal.hasSelection()) {
        navigator.clipboard.writeText(terminal.getSelection()).catch((error) => this.notify(`复制失败：${error}`, true));
        return false;
      }
      return true;
    });
    for (const chunk of this.earlyOutput.get(info.id) || []) entry.stream.receive(chunk);
    this.earlyOutput.delete(info.id);
    try {
      const snapshot = await this.api.GetTerminalOutput(info.id);
      entry.stream.restore(snapshot);
      const exit = this.earlyExits.get(info.id);
      this.earlyExits.delete(info.id);
      if (entry.info.state !== 'exited') entry.info = snapshot.info;
      if (exit || entry.exitInfo || entry.info.state === 'exited') this.markExited(entry, exit || entry.exitInfo || entry.info);
    } catch (error) {
      this.removeEntry(entry);
      await this.api.CloseTerminal(info.id).catch(() => {});
      throw error;
    }
    return entry;
  }

  markExited(entry, info) {
    entry.info = info;
    entry.terminal.options.disableStdin = true;
    if (entry.stream.initializing) {
      entry.exitInfo = info;
      this.render();
      return;
    }
    if (!entry.exitShown) {
      entry.exitShown = true;
      entry.terminal.writeln(`\r\n\x1b[90m[终端进程已退出，退出码 ${info.exitCode ?? '未知'}]\x1b[0m`);
    }
    this.render();
  }

  projectEntries() {
    return [...this.entries.values()].filter((entry) => entry.info.projectId === this.context?.projectId);
  }

  async closeTab(entry) {
    if (entry.closing) return;
    entry.closing = true;
    this.render();
    try {
      await this.api.CloseTerminal(entry.info.id);
      const view = this.view(entry.info.projectId);
      const tabs = [...this.entries.values()].filter((item) => item.info.projectId === entry.info.projectId);
      const index = tabs.indexOf(entry);
      this.removeEntry(entry);
      if (view.active === entry.info.id) view.active = (tabs[index + 1] || tabs[index - 1])?.info.id || null;
      if (!view.active) view.mode = 'logs';
      this.render();
      if (!this.isVisible()) this.onLogs();
      this.focus();
    } catch (error) {
      entry.closing = false;
      this.render();
      this.notify(`关闭终端失败：${error}`, true);
    }
  }

  removeEntry(entry) {
    entry.closed = true;
    entry.terminal.dispose();
    entry.screen.remove();
    this.entries.delete(entry.info.id);
  }

  forgetProject(projectID) {
    for (const entry of this.entries.values()) {
      if (entry.info.projectId === projectID) this.removeEntry(entry);
    }
    this.views.delete(projectID);
    this.render();
  }

  render() {
    const visible = this.isVisible();
    const tabs = this.projectEntries();
    const view = this.view();
    if (!tabs.some((entry) => entry.info.id === view.active)) view.active = tabs[0]?.info.id || null;
    $('terminal-pane').classList.toggle('hidden', !visible);
    $('log-output').classList.toggle('hidden', visible);
    $('log-search').classList.toggle('hidden', visible);
    $('log-filter-counts').classList.toggle('hidden', visible);
    $('terminal-shortcuts').classList.toggle('hidden', !visible);
    $('terminal-toggle').classList.toggle('selected', visible);
    $('terminal-toggle').setAttribute('aria-pressed', String(visible));
    $('terminal-toggle').disabled = this.creating;
    $('terminal-new').disabled = this.creating;
    const codexButton = $('codex-analyze');
    if (codexButton) codexButton.disabled = this.creating;
    $('output-title').textContent = visible ? '终端控制台' : '运行日志';
    const active = this.entries.get(view.active);
    if (visible && active) {
      $('log-summary').textContent = `${active.info.shell} · ${active.info.state === 'exited' ? '已退出' : active.info.directory}`;
      $('log-summary').title = active.info.directory;
    } else $('log-summary').title = '';
    document.querySelectorAll('.log-filter').forEach((button) => {
      if (visible) { button.classList.remove('selected'); button.setAttribute('aria-pressed', 'false'); }
    });
    $('terminal-tabs').replaceChildren();
    for (const entry of this.entries.values()) entry.screen.classList.toggle('hidden', !visible || entry !== active);
    for (const entry of tabs) {
      const wrapper = document.createElement('div');
      wrapper.className = `terminal-tab ${entry === active ? 'selected' : ''}`;
      const select = document.createElement('button');
      select.className = 'terminal-tab-select';
      select.setAttribute('role', 'tab');
      select.setAttribute('aria-selected', String(entry === active));
      select.title = `${entry.info.shell} · ${entry.info.directory}`;
      const side = this.context?.paired ? (entry.info.serviceId.endsWith(':frontend') ? ' · 前端' : ' · 后端') : '';
      select.textContent = `${entry.info.title}${side}${entry.info.state === 'exited' ? ' · 已退出' : ''}`;
      select.onclick = () => { view.active = entry.info.id; this.render(); this.focus(); };
      const close = document.createElement('button');
      close.className = 'terminal-tab-close';
      close.textContent = '×';
      close.title = '关闭此终端及其子进程';
      close.setAttribute('aria-label', `关闭${entry.info.title}`);
      close.disabled = entry.closing;
      close.onclick = () => this.closeTab(entry);
      wrapper.append(select, close);
      $('terminal-tabs').append(wrapper);
    }
    const layout = `${this.context?.projectId}/${this.context?.paired}/${visible}`;
    if (this.layout !== layout) { this.layout = layout; this.onLayout(); }
    this.scheduleFit();
  }

  scheduleFit() {
    if (this.fitFrame) cancelAnimationFrame(this.fitFrame);
    this.fitFrame = requestAnimationFrame(() => {
      this.fitFrame = null;
      if (!this.isVisible()) return;
      const entry = this.entries.get(this.view().active);
      if (entry && entry.screen.clientWidth > 60 && entry.screen.clientHeight > 24) entry.fit.fit();
    });
  }

  focus() {
    requestAnimationFrame(() => {
      if (this.isVisible()) this.entries.get(this.view().active)?.terminal.focus();
    });
  }
}
