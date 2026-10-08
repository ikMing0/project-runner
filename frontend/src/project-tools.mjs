const node = (tag, className, text) => {
  const element = document.createElement(tag);
  element.className = className || '';
  if (text !== undefined) element.textContent = text;
  return element;
};
const button = (label, action) => {
  const element = node('button', 'button small', label);
  element.type = 'button';
  element.onclick = action;
  return element;
};
function dialog(title, hint) {
  const element = node('dialog', 'reuse-dialog tools-dialog');
  const head = node('div', 'reuse-dialog-head');
  const copy = node('div');
  copy.append(node('h2', '', title), node('p', '', hint));
  const close = button('✕', () => element.close());
  close.setAttribute('aria-label', '关闭' + title);
  head.append(copy, close);
  element.append(head);
  document.body.append(element);
  return element;
}
const stamp = value => value ? new Date(value).toLocaleString() : '未知';
const stateNames = { stopped: '已停止', failed: '启动失败', running: '运行中', unready: '未就绪' };

export class ProjectTools {
  constructor({ api, notify, getDraft, applyDraft, selectOwner, clipboard }) {
    Object.assign(this, { api, notify, getDraft, applyDraft, selectOwner, clipboard });
    this.checkDialog = dialog('启动检查', '集中检查目录、工具、脚本、依赖和端口；检查不会启动项目。');
    this.checkActions = node('div', 'tools-actions');
    this.checkSummary = node('p', 'tools-summary');
    this.checkList = node('div', 'check-list');
    this.checkDialog.append(this.checkActions, this.checkSummary, this.checkList);
    this.historyDialog = dialog('启动历史', '每个服务保留最近 10 次已结束的运行，查看和复制的日志经过常见凭据脱敏。');
    this.historyActions = node('div', 'tools-actions');
    this.historySelect = node('select');
    this.historySelect.setAttribute('aria-label', '历史启动记录');
    this.historySelect.onchange = () => this.readHistory();
    this.historyActions.append(this.historySelect, button('刷新', () => this.loadHistory()),
      button('复制日志', () => this.copyHistory()));
    this.historyMeta = node('p', 'tools-summary');
    this.historyLog = node('pre', 'history-log', '请选择启动记录');
    this.historyDialog.append(this.historyActions, this.historyMeta, this.historyLog);
    this.libraryDialog = dialog('配置模板', '本机模板保留本机配置；分享模板剔除本机路径、全部环境变量、启动参数、健康地址和依赖服务地址。');
    this.libraryActions = node('div', 'tools-actions');
    this.libraryActions.append(button('从当前配置保存', () => this.saveTemplate()),
      button('导出分享模板', () => this.exportTemplate()), button('导入并预览', () => this.importTemplates()));
    this.templateName = node('input');
    this.templateName.placeholder = '本机模板名称（保存前可修改）';
    this.templateName.setAttribute('aria-label', '模板名称');
    this.templateList = node('div', 'template-list');
    this.templatePreview = node('p', 'tools-summary', '选择一个模板后，指定项目目录并应用到表单。');
    this.directoryInput = node('input');
    this.directoryInput.placeholder = '选择新项目 / 工作树目录';
    this.directoryInput.setAttribute('aria-label', '模板目标项目目录');
    this.directoryActions = node('div', 'tools-actions');
    this.applyButton = button('应用到新配置', () => this.useTemplate());
    this.directoryActions.append(this.directoryInput, button('浏览', () => this.pickDirectory()), this.applyButton);
    this.libraryDialog.append(this.libraryActions, this.templateName, this.templateList, this.templatePreview, this.directoryActions);
    this.imported = [];
    this.historyVersion = 0;
  }
  async check(project, side = '', show = true) {
    const snapshot = structuredClone(project);
    this.checkContext = { project: snapshot, side };
    const report = await this.api.CheckProject(snapshot, side);
    if (show || !report.allowed) this.showReport(report);
    return report.allowed;
  }
  showReport(report) {
    this.checkSummary.textContent = report.allowed ? '检查通过，可以启动。' : '请处理以下问题后重新检查。';
    this.checkSummary.classList.toggle('check-error', !report.allowed);
    this.checkActions.replaceChildren(button('重新检查', () => {
      const context = this.checkContext;
      if (context) this.check(context.project, context.side).catch(error => this.notify(error, true));
    }));
    this.checkList.replaceChildren();
    for (const check of report.checks || []) {
      const row = node('div', 'check-row ' + check.state);
      row.append(node('span', 'check-state', ({ ok: '通过', error: '问题', info: '提示', warn: '注意' })[check.state] || '提示'),
        node('strong', '', (check.serviceName || '新项目') + ' · ' + check.label),
        node('p', '', check.detail));
      for (const owner of check.owners || []) {
        const occupant = node('div', 'port-occupant');
        occupant.append(node('code', '', (owner.name || '进程') + ' · PID ' + owner.pid + ' · ' + (owner.path || owner.address)));
        if (owner.serviceId) {
          occupant.append(node('span', '', '运行台配置：' + owner.serviceName),
            button('查看配置', () => {
              this.checkDialog.close();
              this.selectOwner(owner.serviceId).catch(error => this.notify(error, true));
            }),
            button('停止占用服务', async () => {
              try {
                await this.api.StopService(owner.serviceId);
                this.notify('已请求停止，请稍后重新检查端口');
              } catch (error) { this.notify(error, true); }
            }));
        } else occupant.append(node('span', 'field-hint', '外部进程，请通过所属软件停止或修改配置端口。'));
        row.append(occupant);
      }
      this.checkList.append(row);
    }
    if (!this.checkDialog.open) this.checkDialog.showModal();
  }
  async openHistory(serviceID, name) {
    this.historyService = serviceID;
    this.historyName = name;
    this.historyDialog.showModal();
    await this.loadHistory();
  }
  async loadHistory() {
    const version = ++this.historyVersion;
    this.historyLog.textContent = '加载中…';
    try {
      const records = await this.api.ListRunHistory(this.historyService);
      if (version !== this.historyVersion) return;
      this.historySelect.replaceChildren(...(records || []).map(record =>
        new Option(stamp(record.startedAt) + ' · ' + (stateNames[record.status.state] || record.status.state), record.id)));
      this.historyMeta.textContent = this.historyName || '';
      if (!records?.length) { this.historyLog.textContent = '暂无已结束的启动记录。当前运行将在停止或退出后保存。'; return; }
      await this.readHistory();
    } catch (error) { this.historyLog.textContent = String(error); }
  }
  async readHistory() {
    const id = this.historySelect.value;
    if (!id) return;
    const version = ++this.historyVersion;
    try {
      const record = await this.api.ReadRunHistory(id);
      if (version !== this.historyVersion) return;
      this.historyMeta.textContent = record.name + ' · ' + stamp(record.startedAt) + ' → ' + stamp(record.endedAt) + ' · ' + (record.status.error || stateNames[record.status.state] || record.status.state);
      this.historyLog.textContent = (record.logs || []).map(line => line.time + ' [' + (line.level || line.source) + '] ' + line.text).join('\n') || '本次没有日志';
    } catch (error) { if (version === this.historyVersion) this.historyLog.textContent = String(error); }
  }
  async copyHistory() {
    try {
      if (!this.historySelect.value) return;
      const text = await this.api.RunHistoryText(this.historySelect.value);
      await this.clipboard(text);
      this.notify('已复制脱敏历史日志');
    } catch (error) { this.notify(error, true); }
  }
  async openLibrary() {
    this.templateName.value = this.getDraft()?.name || '';
    this.directoryInput.value = '';
    this.selectedTemplate = null;
    this.applyButton.disabled = true;
    this.templatePreview.textContent = '选择模板后指定项目目录；应用只填入表单，保存后生效。';
    this.libraryDialog.showModal();
    await this.loadTemplates();
  }
  async loadTemplates() {
    try {
      this.saved = await this.api.ListTemplates() || [];
      this.renderTemplates();
    } catch (error) { this.notify(error, true); }
  }
  renderTemplates() {
    this.templateList.replaceChildren();
    const entries = [...this.saved || [], ...this.imported];
    if (!entries.length) this.templateList.append(node('p', 'field-hint', '尚无模板，可从当前配置保存或导入分享文件。'));
    for (const template of entries) {
      const row = node('div', 'template-row');
      const choose = button(template.name + (template.id ? ' · 本机' : ' · 导入预览'), () => {
        this.selectedTemplate = template;
        this.applyButton.disabled = false;
        const p = template.project;
        this.templatePreview.textContent = '类型：' + p.kind + ' · 模块：' + (p.module || '根目录') +
          ' · 端口：' + p.port + (p.frontend ? ' / ' + p.frontend.port + ' · 前端：' + template.frontendRelative : '') +
          (template.id ? '。本机工具、环境和参数会复用，请在表单检查。' : '。导入配置已剔除本机路径、环境变量和启动参数，需要在表单补充。');
        this.templateList.querySelectorAll('button').forEach(el => el.classList.remove('selected'));
        choose.classList.add('selected');
      });
      row.append(choose);
      if (template.id) row.append(button('删除', async () => {
        try {
          await this.api.DeleteTemplate(template.id);
          if (this.selectedTemplate?.id === template.id) { this.selectedTemplate = null; this.applyButton.disabled = true; }
          await this.loadTemplates();
        } catch (error) { this.notify(error, true); }
      }));
      this.templateList.append(row);
    }
  }
  async saveTemplate() {
    try {
      const p = this.getDraft();
      if (!p) throw new Error('请先选择或填写一个运行配置');
      await this.api.SaveTemplate(p, this.templateName.value.trim() || p.name);
      await this.loadTemplates();
      this.notify('已保存本机模板');
    } catch (error) { this.notify(error, true); }
  }
  async exportTemplate() {
    try {
      const p = this.getDraft();
      if (!p) throw new Error('请先选择或填写一个运行配置');
      const path = await this.api.ExportProjectTemplate(p);
      if (path) this.notify('已导出脱敏分享模板：' + path);
    } catch (error) { this.notify(error, true); }
  }
  async importTemplates() {
    try {
      const imported = await this.api.ReadTemplateFile();
      if (!imported?.length) return;
      this.imported = imported;
      this.renderTemplates();
      this.notify('已读取模板，请选择并预览后应用');
    } catch (error) { this.notify(error, true); }
  }
  async pickDirectory() {
    try { const path = await this.api.PickDirectory(); if (path) this.directoryInput.value = path; }
    catch (error) { this.notify(error, true); }
  }
  async useTemplate() {
    if (!this.selectedTemplate) return;
    try {
      const project = await this.api.ApplyTemplate(this.selectedTemplate, this.directoryInput.value.trim());
      if (this.applyDraft(project)) this.libraryDialog.close();
    } catch (error) { this.notify(error, true); }
  }
}
