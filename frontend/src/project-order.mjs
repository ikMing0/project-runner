export function moveProject(items, sourceID, targetID, position) {
  if (sourceID === targetID || !items.some(item => item.id === sourceID) || !items.some(item => item.id === targetID)) return items;
  const source = items.find(item => item.id === sourceID);
  const ordered = items.filter(item => item.id !== sourceID);
  const index = ordered.findIndex(item => item.id === targetID) + (position === 'after' ? 1 : 0);
  ordered.splice(index, 0, source);
  return ordered;
}

export class ProjectOrderController {
  constructor(list, { onMove, onRelease }) {
    this.list = list;
    this.onMove = onMove;
    this.onRelease = onRelease;
    this.drag = null;
    this.saving = false;
    this.ignoreClick = false;
    list.addEventListener('pointerdown', event => this.begin(event));
    window.addEventListener('pointermove', event => this.move(event));
    window.addEventListener('pointerup', event => this.end(event));
    window.addEventListener('pointercancel', event => this.end(event, true));
    list.addEventListener('lostpointercapture', event => this.end(event, true));
    list.addEventListener('click', event => {
      if (!this.ignoreClick) return;
      this.ignoreClick = false;
      event.preventDefault();
      event.stopPropagation();
    }, true);
    list.addEventListener('keydown', event => {
      if (!event.altKey || !['ArrowUp', 'ArrowDown'].includes(event.key) || !event.target.closest('.project-drag-handle')) return;
      event.preventDefault();
      if (this.saving || this.drag) return;
      const item = event.target.closest('.project-item');
      const neighbor = event.key === 'ArrowUp' ? item.previousElementSibling : item.nextElementSibling;
      if (neighbor) this.save(item.dataset.projectId, neighbor.dataset.projectId, event.key === 'ArrowUp' ? 'before' : 'after', true);
    });
  }

  isDragging() { return this.drag !== null; }

  begin(event) {
    this.ignoreClick = false;
    if (event.button !== 0 || !event.isPrimary || this.saving || this.drag || this.list.children.length < 2) return;
    if (event.target.closest('.project-quick-action')) return;
    const item = event.target.closest('.project-item');
    if (!item) return;
    this.drag = { id: item.dataset.projectId, pointerID: event.pointerId, startX: event.clientX, startY: event.clientY,
      x: event.clientX, y: event.clientY, moved: false, target: null };
  }

  move(event) {
    const drag = this.drag;
    if (!drag || drag.pointerID !== event.pointerId) return;
    drag.x = event.clientX;
    drag.y = event.clientY;
    if (!drag.moved) {
      if (Math.hypot(drag.x - drag.startX, drag.y - drag.startY) < 6) return;
      drag.moved = true;
      this.list.setPointerCapture(drag.pointerID);
      this.list.classList.add('sorting');
      this.list.querySelector(`[data-project-id="${CSS.escape(drag.id)}"]`)?.classList.add('dragging');
      this.frame = requestAnimationFrame(() => this.tick());
    }
    event.preventDefault();
    this.updateTarget();
  }

  updateTarget() {
    const drag = this.drag;
    this.list.querySelectorAll('.drop-before, .drop-after').forEach(item => item.classList.remove('drop-before', 'drop-after'));
    drag.target = null;
    const bounds = this.list.getBoundingClientRect();
    if (drag.x < bounds.left || drag.x > bounds.right || drag.y < bounds.top - 24 || drag.y > bounds.bottom + 24) return;
    const items = [...this.list.children].filter(item => item.dataset.projectId !== drag.id);
    for (const item of items) {
      const rect = item.getBoundingClientRect();
      if (drag.y < rect.top + rect.height / 2) {
        drag.target = { id: item.dataset.projectId, position: 'before' };
        item.classList.add('drop-before');
        return;
      }
    }
    const last = items.at(-1);
    if (last) {
      drag.target = { id: last.dataset.projectId, position: 'after' };
      last.classList.add('drop-after');
    }
  }

  tick() {
    if (!this.drag?.moved) return;
    const bounds = this.list.getBoundingClientRect();
    if (this.drag.x >= bounds.left && this.drag.x <= bounds.right) {
      const distance = this.drag.y < bounds.top + 28 ? -8 : this.drag.y > bounds.bottom - 28 ? 8 : 0;
      if (distance) { this.list.scrollTop += distance; this.updateTarget(); }
    }
    this.frame = requestAnimationFrame(() => this.tick());
  }

  end(event, canceled = false) {
    const drag = this.drag;
    if (!drag || drag.pointerID !== event.pointerId) return;
    this.drag = null;
    cancelAnimationFrame(this.frame);
    if (this.list.hasPointerCapture(drag.pointerID)) this.list.releasePointerCapture(drag.pointerID);
    this.list.classList.remove('sorting');
    this.list.querySelectorAll('.dragging, .drop-before, .drop-after').forEach(item => item.classList.remove('dragging', 'drop-before', 'drop-after'));
    this.ignoreClick = drag.moved;
    if (!canceled && drag.moved && drag.target) this.save(drag.id, drag.target.id, drag.target.position);
    else if (drag.moved) this.onRelease();
    else requestAnimationFrame(() => this.onRelease());
  }

  async save(id, target, position, restoreFocus = false) {
    this.saving = true;
    try { await this.onMove(id, target, position); }
    finally {
      this.saving = false;
      this.onRelease();
      if (restoreFocus) this.list.querySelector(`[data-project-id="${CSS.escape(id)}"] .project-drag-handle`)?.focus();
    }
  }
}
