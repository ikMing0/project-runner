export class DependencyEditor {
  constructor(element, onChange) {
    this.element = element;
    this.onChange = onChange;
    this.render([]);
  }
  read() {
    return [...this.element.querySelectorAll('.dependency-row')].map(row => ({
      name: row.querySelector('[data-field=name]').value.trim(),
      host: row.querySelector('[data-field=host]').value.trim(),
      port: Number(row.querySelector('[data-field=port]').value),
      required: row.querySelector('[data-field=required]').checked,
    }));
  }
  render(checks) {
    this.element.replaceChildren();
    const title = document.createElement('strong');
    title.textContent = '依赖服务检查（可选）';
    const hint = document.createElement('small');
    hint.className = 'field-hint';
    hint.textContent = '填写 MySQL、Redis 等主机和端口。必需项连接失败会阻止启动；只检查 TCP 连通性，无需账号密码。';
    const rows = document.createElement('div');
    rows.className = 'dependency-rows';
    const add = document.createElement('button');
    add.type = 'button'; add.className = 'button small dependency-add'; add.textContent = '＋ 添加依赖服务';
    add.onclick = () => {
      const current = this.read();
      if (current.length >= 16) return;
      this.render([...current, { name: '', host: '127.0.0.1', port: 3306, required: true }]);
      this.onChange();
      this.element.querySelector('.dependency-row:last-child input').focus();
    };
    for (const [index, check] of (checks || []).entries()) {
      const row = document.createElement('div'); row.className = 'dependency-row';
      for (const [field, label, type] of [['name', '服务名称', 'text'], ['host', '主机 / IP', 'text'], ['port', '端口', 'number']]) {
        const input = document.createElement('input');
        input.dataset.field = field; input.type = type; input.value = check[field] ?? '';
        input.placeholder = label; input.setAttribute('aria-label', `依赖 ${index + 1} ${label}`);
        if (type === 'number') { input.min = 1; input.max = 65535; }
        input.oninput = () => this.onChange();
        row.append(input);
      }
      const required = document.createElement('label'); required.className = 'check-field';
      const checkbox = document.createElement('input'); checkbox.type = 'checkbox'; checkbox.dataset.field = 'required';
      checkbox.checked = !!check.required; checkbox.onchange = () => this.onChange();
      checkbox.setAttribute('aria-label', `依赖 ${index + 1} 为必需服务`);
      required.append(checkbox, document.createTextNode('必需'));
      const remove = document.createElement('button'); remove.type = 'button'; remove.className = 'button small';
      remove.textContent = '移除'; remove.setAttribute('aria-label', `移除依赖 ${index + 1}`);
      remove.onclick = () => { const current = this.read(); current.splice(index, 1); this.render(current); this.onChange(); };
      row.append(required, remove); rows.append(row);
    }
    add.disabled = (checks || []).length >= 16;
    this.element.append(title, hint, rows, add);
  }
}
