import { test } from 'node:test';
import assert from 'node:assert/strict';
import { problemAdvice } from './problem-advice.mjs';
import { activeState, groupState } from './run-state.mjs';

test('diagnosis finds root causes in exception chains and excludes previous attempts', () => {
  const lines = [{ text: 'UnsatisfiedDependencyException Error creating bean' },
    { text: 'Caused by: ClassNotFoundException: com.demo.MissingClass' }];
  assert.equal(problemAdvice(lines).id, 'class-missing');
  assert.match(problemAdvice(lines, { recovery: 'failed' }).steps[1], /仍未成功/);
  assert.equal(problemAdvice(lines, { recovery: 'recovered' }), null);
  assert.equal(problemAdvice(lines, { attempt: 2 }), null);
  assert.equal(problemAdvice([{ text: 'COMPILATION ERROR BUILD FAILURE' }]), null);
});
test('common failures suggest targeted actions without treating proxy failures as compilation', () => {
  const examples = [
    ['Error: Port 82 is already in use', 'port'],
    ['attempting to assign weaker access privileges; was public', 'compile'],
    ['UnsupportedClassVersionError: compiled by newer Java', 'java-version'],
    ['Could not resolve dependencies for project', 'dependency'],
    ["Cannot find module 'vite'", 'node-dependency'],
    ['[vite] http proxy error: /login\nECONNREFUSED', 'proxy'],
    ['等待就绪超过 90 秒：健康检查返回 503，预期 200', 'unready'],
  ];
  for (const [text, id] of examples) assert.equal(problemAdvice([{ text }]).id, id);
  assert.match(problemAdvice([{ text: '[vite] http proxy error: /login\nECONNREFUSED' }]).title, /前端代理/);
});
test('timed out process remains stoppable and group never reports it as ready', () => {
  assert.equal(activeState('unready'), true);
  assert.equal(groupState({ id: 'app', frontend: {} }, new Map([
    ['app', { state: 'unready' }], ['app:frontend', { state: 'running' }],
  ])), 'unready');
});
