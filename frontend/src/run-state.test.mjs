import { test } from 'node:test';
import assert from 'node:assert/strict';
import { isNewRun, actionableError } from './run-state.mjs';

test('switching from build to application preserves build diagnostics', () => {
  const building = { state: 'building', startedAt: 100, pid: 1 };
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
