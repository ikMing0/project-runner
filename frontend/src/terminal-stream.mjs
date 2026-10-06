export function terminalBytes(data) {
  return Uint8Array.from(atob(data), (character) => character.charCodeAt(0));
}

// A snapshot overlaps events emitted while the new tab is being created.
// Keep only later chunks, and deliver them once in their original order.
export class TerminalStream {
  constructor(write) {
    this.write = write;
    this.sequence = 0;
    this.initializing = true;
    this.pending = new Map();
  }

  receive(chunk) {
    if (!this.initializing && chunk.sequence <= this.sequence) return;
    this.pending.set(chunk.sequence, chunk);
    this.flush();
  }

  restore(snapshot) {
    if (snapshot.data) this.write(terminalBytes(snapshot.data));
    this.sequence = snapshot.sequence;
    this.initializing = false;
    for (const sequence of this.pending.keys()) {
      if (sequence <= this.sequence) this.pending.delete(sequence);
    }
    this.flush();
  }

  flush() {
    if (this.initializing) return;
    while (this.pending.has(this.sequence + 1)) {
      const chunk = this.pending.get(++this.sequence);
      this.pending.delete(this.sequence);
      this.write(terminalBytes(chunk.data));
    }
  }
}

export function splitTerminalInput(data, size = 8192) {
  const chunks = [];
  for (let start = 0; start < data.length;) {
    let end = Math.min(start + size, data.length);
    // Never split a UTF-16 surrogate pair before the JSON bridge encodes UTF-8.
    if (end < data.length && /[\uD800-\uDBFF]/.test(data[end - 1])) end--;
    chunks.push(data.slice(start, end));
    start = end;
  }
  return chunks;
}
