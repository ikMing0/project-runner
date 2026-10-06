import { test } from 'node:test';
import assert from 'node:assert/strict';
import { moveProject } from './project-order.mjs';

test('moving first to last and last to first retains the original projects and services', () => {
  const projects = [{id:'main'}, {id:'inventory', frontend:{port:84}}, {id:'after-sale'}];
  const down = moveProject(projects, 'main', 'after-sale', 'after');
  assert.deepEqual(down.map(item => item.id), ['inventory', 'after-sale', 'main']);
  assert.equal(down[0], projects[1]);
  const up = moveProject(down, 'main', 'inventory', 'before');
  assert.deepEqual(up, projects);
  assert.deepEqual(projects.map(item => item.id), ['main', 'inventory', 'after-sale']);
});

test('adjacent moves use the insertion side and invalid or canceled targets preserve all entries', () => {
  const projects = [{id:'a'}, {id:'b'}, {id:'c'}];
  assert.deepEqual(moveProject(projects, 'a', 'b', 'before'), projects);
  assert.deepEqual(moveProject(projects, 'a', 'b', 'after').map(item => item.id), ['b', 'a', 'c']);
  assert.deepEqual(moveProject(projects, 'c', 'b', 'before').map(item => item.id), ['a', 'c', 'b']);
  for (const [source, target] of [['a','a'], ['missing','b'], ['a','missing']]) {
    assert.equal(moveProject(projects, source, target, 'after'), projects);
  }
});
