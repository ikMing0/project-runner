import { test } from 'node:test';
import assert from 'node:assert/strict';
import { nextProjectPort } from './project-ports.mjs';

test('unsorted saved configurations advance backend and frontend ports separately, including stopped projects', () => {
  const projects = [
    {kind:'spring-maven', port:8083, frontend:{port:84}},
    {kind:'spring-maven', port:8081, frontend:{port:82}},
    {kind:'spring-gradle', port:8084, frontend:{port:85}},
    {kind:'spring-maven', port:8082, frontend:{port:83}},
  ];
  const previous = structuredClone(projects);
  assert.equal(nextProjectPort(projects, 'spring-maven'), 8085);
  assert.equal(nextProjectPort(projects, 'node'), 86);
  assert.deepEqual(projects, previous);
});

test('standalone Node ports join the frontend sequence and suggestions avoid ports from either side and the new pair', () => {
  const projects = [{kind:'spring-maven',port:83}, {kind:'node',port:82}];
  assert.equal(nextProjectPort(projects, 'spring-maven'), 84);
  assert.equal(nextProjectPort(projects, 'node'), 84);
  assert.equal(nextProjectPort(projects, 'node', [84]), 85);
  assert.equal(nextProjectPort([{kind:'node',port:5173}], 'node'), 5174);
});

test('empty history retains defaults, malformed historical values are ignored, and port limits never wrap', () => {
  assert.equal(nextProjectPort([], 'spring-maven'), 8080);
  assert.equal(nextProjectPort([], 'node'), 82);
  assert.equal(nextProjectPort([{port:0},{port:-1},{port:2.5},{port:65536}], 'spring-maven'), 8080);
  assert.equal(nextProjectPort([{port:65534}], 'spring-maven'), 65535);
  assert.equal(nextProjectPort([{port:65535}], 'spring-maven'), null);
  assert.equal(nextProjectPort([{kind:'node',port:65535}], 'node'), null);
});
