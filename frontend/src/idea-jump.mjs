export class IDEAJumper {
  constructor(api, notify) {
    this.api = api; this.notify = notify;
    this.dialog = document.createElement('dialog');
    this.dialog.className = 'reuse-dialog tools-dialog';
    this.dialog.innerHTML = '<div class="reuse-dialog-head"><div><h2>IDEA 跳转设置</h2><p>点击编译日志里的文件位置，打开对应源码行。</p></div><button type="button" class="button small" aria-label="关闭 IDEA 跳转设置">✕</button></div><label class="field editor-field"><span>IDEA 启动程序（留空自动识别）</span><div class="input-action"><input aria-label="IDEA 启动程序" placeholder="IDEA 安装目录 / bin / idea64.exe"><button type="button" class="button small editor-browse">浏览</button></div></label><p class="field-hint editor-hint"></p><div class="tools-actions"><button type="button" class="button primary editor-save">保存设置</button></div>';
    document.body.append(this.dialog);
    this.input = this.dialog.querySelector('input');
    this.save = this.dialog.querySelector('.editor-save');
    this.dialog.querySelector('[aria-label="关闭 IDEA 跳转设置"]').onclick = () => this.dialog.close();
    this.dialog.querySelector('.editor-browse').onclick = async () => {
      try { const path = await api.PickIDEAPath(); if (path) this.input.value = path; }
      catch (error) { notify(error, true); }
    };
    this.save.onclick = async () => {
      this.save.disabled = true;
      try {
        await api.SetIDEAPath(this.input.value.trim());
        if (this.pending) await api.OpenSourceInIDEA(this.serviceID, this.pending.path, this.pending.line, this.pending.column);
        this.dialog.close(); notify(this.pending ? '已跳转到 IDEA' : '已保存 IDEA 跳转设置');
      } catch (error) { notify(error, true); }
      finally { this.save.disabled = false; }
    };
  }
  async settings(serviceID, pending = null) {
    const settings = await this.api.GetEditorSettings(serviceID);
    this.serviceID = serviceID; this.pending = pending;
    this.input.value = settings.ideaPath || '';
    this.dialog.querySelector('.editor-hint').textContent = settings.detectedPath
      ? '自动识别：' + settings.detectedPath : '尚未自动找到 IDEA，请选择 bin 下的 idea64.exe。设置仅保存在本机。';
    this.save.textContent = pending ? '保存并跳转' : '保存设置';
    if (!this.dialog.open) this.dialog.showModal();
  }
  async open(serviceID, location) {
    try { await this.api.OpenSourceInIDEA(serviceID, location.path, location.line, location.column); }
    catch (error) {
      if (String(error).includes('IDEA_NOT_FOUND:')) {
        try { await this.settings(serviceID, location); } catch (settingError) { this.notify(settingError, true); }
      } else this.notify(error, true);
    }
  }
}
