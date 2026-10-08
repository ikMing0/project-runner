export function metricsText(value) {
  if (!value) return '资源信息采集中…';
  if (value.error) return value.error;
  const cpu = value.cpuSampled ? Number(value.cpuPercent || 0).toFixed(1) + '%' : '采集中';
  const memory = ((value.memoryBytes || 0) / 1024 / 1024).toFixed(1);
  return `CPU ${cpu} · 内存 ${value.partial ? '≥ ' : ''}${memory} MiB · ${value.processCount || 0} 个进程`;
}
export class DesktopTools {
  constructor({api,notify,openURL,onSettings}) {
    Object.assign(this,{api,notify,openURL,onSettings});
    this.settings = {closeToTray:false,trayAvailable:false};
    this.dialog = document.createElement('dialog');
    this.dialog.className = 'reuse-dialog tools-dialog desktop-dialog';
    this.dialog.innerHTML = '<div class="reuse-dialog-head"><div><h2>应用设置与更新</h2><p>关闭行为保存在本机；更新检查由你手动触发。</p></div><button class="button small desktop-close" type="button" aria-label="关闭应用设置">✕</button></div><div class="desktop-section"><label class="check-field"><input type="checkbox" class="close-to-tray"><span>关闭窗口时收起到托盘，项目继续运行</span></label><p class="field-hint tray-hint"></p><div class="tools-actions"><button class="button secondary desktop-save" type="button">保存关闭设置</button><button class="button desktop-hide" type="button">收起到托盘</button><button class="button danger desktop-quit" type="button">退出并停止所有项目</button></div></div><div class="desktop-section"><h3>版本与更新</h3><p class="build-info"></p><p class="update-message">点击检查更新，或在 GitHub 查看发布版本。</p><div class="tools-actions"><button class="button primary update-check" type="button">检查更新</button><button class="button release-open" type="button">打开 Release 页面</button></div></div>';
    document.body.append(this.dialog);
    this.dialog.querySelector('.desktop-close').onclick = () => this.dialog.close();
    this.dialog.querySelector('.desktop-save').onclick = async () => {
      try {
        const closeToTray = this.dialog.querySelector('.close-to-tray').checked;
        await api.SaveDesktopSettings({closeToTray});
        this.apply({...this.settings,closeToTray}); notify('已保存关闭行为');
      } catch(error) {notify(error,true)}
    };
    this.dialog.querySelector('.desktop-hide').onclick = () => this.hide();
    this.dialog.querySelector('.desktop-quit').onclick = () => api.QuitApplication().catch(error=>notify(error,true));
    this.dialog.querySelector('.update-check').onclick = () => this.checkUpdates();
    this.dialog.querySelector('.release-open').onclick = () => {
      if (this.releaseURL) openURL(this.releaseURL);
    };
  }
  async restore() {this.apply(await this.api.GetDesktopSettings())}
  apply(settings) {
    this.settings = settings;
    this.dialog.querySelector('.close-to-tray').checked = !!settings.closeToTray;
    this.dialog.querySelector('.close-to-tray').disabled = !settings.trayAvailable;
    this.dialog.querySelector('.desktop-hide').disabled = !settings.trayAvailable;
    this.dialog.querySelector('.tray-hint').textContent = settings.trayAvailable
      ? '托盘图标可点击恢复窗口，右键“退出”会停止所有项目。未勾选时，关闭窗口仍会退出。'
      : '托盘当前不可用，窗口关闭时会退出并停止所有项目。';
    this.onSettings(settings);
  }
  async open() {
    await this.restore();
    const info = await this.api.GetBuildInfo();
    this.releaseURL = info.releasesUrl;
    this.dialog.querySelector('.build-info').textContent =
      `当前版本：${info.version === 'dev' ? '开发版' : info.version} · 源码：${info.commit === 'unknown' ? '未标记' : info.commit.slice(0,12)}`;
    if (!this.dialog.open) this.dialog.showModal();
  }
  async hide() {
    try {await this.api.HideToTray();this.dialog.close()}
    catch(error) {this.notify(error,true)}
  }
  async checkUpdates() {
    const button = this.dialog.querySelector('.update-check');
    const message = this.dialog.querySelector('.update-message');
    button.disabled = true; message.textContent = '正在检查 GitHub Release…';
    try {
      const result = await this.api.CheckForUpdates();
      this.releaseURL = result.releaseUrl;
      message.textContent = result.message + (result.publishedAt ? ' · 发布于 ' + new Date(result.publishedAt).toLocaleDateString() : '');
      this.dialog.querySelector('.release-open').textContent = result.hasUpdate ? '查看新版本 / 下载' : '打开 Release 页面';
    } catch(error) {message.textContent = String(error)}
    finally {button.disabled = false}
  }
}
