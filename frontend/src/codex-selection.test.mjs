import { test } from 'node:test';
import assert from 'node:assert/strict';
import { codexSelection, codexEfforts } from './codex-selection.mjs';

test('analysis defaults to low even when the CLI default is high', () => {
  assert.deepEqual(codexSelection(null), {model:'',effort:'low'});
  assert.deepEqual(codexSelection({effort:'invalid'}), {model:'',effort:'low'});
  assert.deepEqual(codexSelection({model:' custom-model ',effort:'medium'}), {model:'custom-model',effort:'medium'});
});

test('reasoning choices follow the selected CLI model while manual models remain usable', () => {
  const options = {defaultModel:'default',models:[{id:'default',reasoning:['low','medium','bogus']},{id:'other',reasoning:['low','high']}]};
  assert.deepEqual(codexEfforts(options,''), ['low','medium']);
  assert.deepEqual(codexEfforts(options,'other'), ['low','high']);
  assert.deepEqual(codexEfforts(options,'manual'), ['low','medium','high']);
  assert.deepEqual(codexEfforts(null,''), ['low','medium','high']);
});
