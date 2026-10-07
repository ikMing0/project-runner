const $ = id => document.getElementById(id);
const statusName = code => ({A:'新增',M:'修改',D:'删除',R:'重命名',C:'复制',T:'类型变化',U:'冲突'})[code] || '变更';

export function gitChangeLabel(file) {
  if (file.conflict) return '合并冲突';
  const labels = [];
  if (file.staged) labels.push(`已暂存 · ${statusName(file.indexStatus)}`);
  if (file.unstaged) labels.push(`未暂存 · ${statusName(file.worktreeStatus)}`);
  if (file.untracked) labels.push('未跟踪');
  return labels.join(' / ');
}

export class GitChangesView {
  constructor({api, directory, onChange}) {
    this.api = api;
    this.directory = directory;
    this.onChange = onChange;
    this.request = 0;
    this.diffRequest = 0;
    this.data = null;
    this.selected = null;
    this.view = 'working';
    $('git-refresh').onclick = () => this.refresh();
    $('git-search').oninput = () => this.renderFiles();
    for (const view of ['working','staged']) $('git-'+view).onclick = () => {this.view=view;this.loadDiff();};
  }

  reset() {
    this.request++;
    this.diffRequest++;
    this.data = null;
    this.selected = null;
    $('git-search').value = '';
  }

  async refresh() {
    const request = ++this.request;
    this.diffRequest++;
    const directory = this.directory();
    const selectedPath = this.selected?.path;
    this.selected = null;
    this.data = null;
    $('git-refresh').disabled = true;
    $('git-summary').textContent = '正在读取未提交文件…';
    $('git-root').textContent = directory || '请选择项目目录';
    $('git-files').replaceChildren();
    this.showText('正在读取当前工作树…');
    try {
      const data = await this.api.GetGitChanges(directory);
      if (request !== this.request) return;
      if (directory !== this.directory()) {
        $('git-summary').textContent = '项目目录已变更，请刷新';
        this.showText('请刷新以读取新目录的 Git 状态。');
        return;
      }
      this.data = data;
      $('git-summary').textContent = `${data.detached ? '分离 HEAD · ' : '分支 · '}${data.branch} · 未提交 ${data.files.length} 个文件`;
      $('git-root').textContent = data.root;
      this.selected = data.files.find(file => file.path === selectedPath) || data.files[0] || null;
      this.renderFiles();
      if (this.selected) this.select(this.selected);
      else this.showText('当前工作树没有未提交文件。');
    } catch (error) {
      if (request !== this.request) return;
      this.data = null;
      this.selected = null;
      $('git-summary').textContent = '无法读取 Git 状态';
      this.showText(String(error));
    } finally {
      if (request === this.request) {$('git-refresh').disabled=false;this.onChange();}
    }
  }

  renderFiles() {
    const query = $('git-search').value.toLowerCase();
    const list = $('git-files');
    list.replaceChildren();
    for (const file of this.data?.files || []) {
      if (!`${file.path} ${file.oldPath || ''}`.toLowerCase().includes(query)) continue;
      const button = document.createElement('button');
      button.className = `git-file ${this.selected?.path === file.path ? 'selected' : ''}`;
      button.setAttribute('aria-label', `查看 ${file.path} 的 Git 差异`);
      button.setAttribute('aria-pressed', String(this.selected?.path === file.path));
      const path = document.createElement('span');
      path.className = 'git-file-path';
      path.textContent = file.oldPath ? `${file.oldPath} → ${file.path}` : file.path;
      const status = document.createElement('small');
      status.textContent = gitChangeLabel(file);
      button.append(path,status);
      button.onclick = () => this.select(file);
      list.append(button);
    }
    if (!list.children.length) {
      const empty = document.createElement('p');
      empty.className='git-empty';
      empty.textContent = query ? '没有匹配的文件' : '没有未提交文件';
      list.append(empty);
    }
  }

  select(file) {
    this.selected = file;
    this.view = file.staged && !file.unstaged && !file.untracked ? 'staged' : 'working';
    this.renderFiles();
    this.loadDiff();
  }

  showText(text) {
    $('git-diff').textContent = text;
    $('git-diff-note').textContent = '';
    if (!this.selected) {
      $('git-file-title').textContent = '文件差异';
      $('git-working').disabled = true;
      $('git-staged').disabled = true;
    }
  }

  async loadDiff() {
    const file = this.selected;
    const root = this.data?.root;
    if (!file || !root) return;
    const request = ++this.diffRequest;
    const view = this.view;
    $('git-file-title').textContent = file.path;
    $('git-working').textContent = file.untracked ? '未跟踪内容' : '工作区差异';
    $('git-working').disabled = !file.unstaged && !file.untracked;
    $('git-staged').disabled = !file.staged;
    for (const mode of ['working','staged']) {
      $('git-'+mode).classList.toggle('selected', mode===view);
      $('git-'+mode).setAttribute('aria-pressed',String(mode===view));
    }
    this.showText('正在读取文件差异…');
    try {
      const diff = await this.api.GetGitFileDiff(root,file.path,view);
      if (request!==this.diffRequest || root!==this.data?.root || file!==this.selected) return;
      const output = $('git-diff');
      output.replaceChildren();
      const lines = diff.text.split('\n');
      if (diff.binary) this.showText('二进制或非 UTF-8 文件，暂不显示文本差异。');
      else if (!diff.text) this.showText('该视图没有文本差异；文件可能已在外部修改，请刷新列表。');
      else for (const line of lines.slice(0,5000)) {
        const span = document.createElement('span');
        span.className = 'git-diff-line';
        if (!diff.untracked && line.startsWith('+') && !line.startsWith('+++')) span.classList.add('added');
        else if (!diff.untracked && line.startsWith('-') && !line.startsWith('---')) span.classList.add('removed');
        else if (!diff.untracked && line.startsWith('@@')) span.classList.add('hunk');
        span.textContent = line+'\n';
        output.append(span);
      }
      $('git-diff-note').textContent = diff.truncated || lines.length>5000 ? '内容较大，仅预览前 1 MiB / 5000 行。' : diff.untracked ? '未跟踪文件内容（只读）' : '只读差异';
    } catch (error) {if(request===this.diffRequest)this.showText(String(error));}
  }
}
