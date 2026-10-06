import { test } from 'node:test';
import assert from 'node:assert/strict';
import { isNewRun, actionableError, serviceIDs, groupState, activeState } from './run-state.mjs';

test('switching from build to application preserves build diagnostics', () => {
	const checking = {state:'checking', startedAt:100};
  assert.equal(isNewRun(undefined, checking), true);
  const building = { state: 'building', startedAt: 100, pid: 1 };
  assert.equal(isNewRun(checking, building), false);
  assert.equal(isNewRun(checking, {state:'starting', startedAt:100}), false);
  assert.equal(activeState('checking'), true);
  assert.equal(isNewRun(undefined, building), true);
  assert.equal(isNewRun(building, { ...building, pid: 2 }), false);
  assert.equal(isNewRun(building, { state: 'starting', startedAt: 100, pid: 3 }), false);
  assert.equal(isNewRun(building, { state: 'failed', startedAt: 100 }), false);
  assert.equal(isNewRun(building, { state: 'building', startedAt: 200 }), true);
});

test('failure banner selects the compiler diagnosis instead of Maven help', () => {
  const errors = [
    '[ERROR] COMPILATION ERROR :',
    '[ERROR] AliyunSmsServiceImpl.java:[91,21] attempting to assign weaker access privileges',
    '[INFO] BUILD FAILURE',
    '[ERROR] Failed to execute goal maven-compiler-plugin:compile',
    '[ERROR] -> [Help 1]',
    '[ERROR] [Help 1] http://cwiki.apache.org/confluence/display/MAVEN/MojoFailureException',
  ];
  assert.equal(errors.find(actionableError), errors[1]);
  assert.equal(actionableError('[ERROR]'), false);
  assert.equal(actionableError("'mvn' is not recognized as an internal or external command"), true);
});

test('paired group reports each partial, failed and ready lifecycle accurately', () => {
  const project = { id: 'backend', frontend: {} };
  assert.deepEqual(serviceIDs(project), ['backend', 'backend:frontend']);
  const states = new Map();
  assert.equal(groupState(project, states), 'stopped');
  states.set('backend', { state: 'checking' });
  assert.equal(groupState(project, states), 'checking');
  states.set('backend', { state: 'building' });
  states.set('backend:frontend', { state: 'running' });
  assert.equal(groupState(project, states), 'building');
  states.set('backend', { state: 'starting' });
  assert.equal(groupState(project, states), 'starting');
  states.set('backend', { state: 'failed' });
  assert.equal(groupState(project, states), 'partial');
  states.set('backend:frontend', { state: 'failed' });
  assert.equal(groupState(project, states), 'failed');
  states.set('backend', { state: 'running' });
  states.set('backend:frontend', { state: 'running' });
  assert.equal(groupState(project, states), 'running');
  states.set('backend:frontend', { state: 'stopped' });
  assert.equal(groupState(project, states), 'partial');
  assert.equal(activeState('partial'), false);
  assert.equal(activeState('running'), true);
});

test('existing standalone configs retain their single service state', () => {
  const project = { id: 'node' };
  assert.deepEqual(serviceIDs(project), ['node']);
  assert.equal(groupState(project, new Map([['node', { state: 'running' }]])), 'running');
  assert.equal(groupState(project, new Map([['node', { state: 'failed' }]])), 'failed');
});
