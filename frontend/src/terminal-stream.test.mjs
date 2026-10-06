import test from 'node:test';
import assert from 'node:assert/strict';
import { TerminalStream, splitTerminalInput } from './terminal-stream.mjs';

const data = (text) => Buffer.from(text).toString('base64');

test('initial snapshot and concurrent events render once in sequence', () => {
  const written = [];
  const stream = new TerminalStream((bytes) => written.push(Buffer.from(bytes).toString()));
  stream.receive({ sequence: 2, data: data('overlap') });
  stream.receive({ sequence: 4, data: data('four') });
  stream.restore({ sequence: 2, data: data('snapshot') });
  assert.deepEqual(written, ['snapshot']);
  stream.receive({ sequence: 3, data: data('three') });
  stream.receive({ sequence: 3, data: data('duplicate') });
  stream.receive({ sequence: 1, data: data('old') });
  assert.deepEqual(written, ['snapshot', 'three', 'four']);
});

test('split UTF-8 characters reach the terminal without JSON corruption', () => {
  const decoder = new TextDecoder();
  let written = '';
  const stream = new TerminalStream((bytes) => { written += decoder.decode(bytes, { stream: true }); });
  const bytes = Buffer.from('中文😀');
  stream.receive({ sequence: 1, data: bytes.subarray(0, 2).toString('base64') });
  stream.restore({ sequence: 0, data: '' });
  stream.receive({ sequence: 2, data: bytes.subarray(2, 7).toString('base64') });
  stream.receive({ sequence: 3, data: bytes.subarray(7).toString('base64') });
  assert.equal(written, '中文😀');
});

test('large pasted input preserves surrogate pairs and order', () => {
  const pasted = 'x'.repeat(8191) + '😀中文' + 'y'.repeat(20000);
  const chunks = splitTerminalInput(pasted);
  assert.equal(chunks.join(''), pasted);
  assert.ok(chunks.every((chunk) => chunk.length <= 8192));
  assert.ok(chunks.every((chunk) => Buffer.from(chunk).toString() === chunk));
});
