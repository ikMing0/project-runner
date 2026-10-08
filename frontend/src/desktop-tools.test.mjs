import test from 'node:test';
import assert from 'node:assert/strict';
import {metricsText} from './desktop-tools.mjs';
import {activeState,groupState,isNewRun} from './run-state.mjs';
test('waiting frontend is active and cancellable, while unready backend never looks ready',()=>{
  const p={id:'backend',frontend:{}};
  const statuses=new Map([['backend',{state:'building'}],['backend:frontend',{state:'waiting'}]]);
  assert.equal(activeState('waiting'),true);
  assert.equal(groupState(p,statuses),'building');
  statuses.set('backend',{state:'unready'});
  assert.equal(groupState(p,statuses),'unready');
  assert.equal(isNewRun({startedAt:1},{state:'waiting',startedAt:2}),true);
});
test('process resource labels distinguish first samples, partial memory and read failures',()=>{
  assert.match(metricsText(null),/采集中/);
  assert.equal(metricsText({cpuPercent:2.345,cpuSampled:true,memoryBytes:64*1024*1024,processCount:3}),
    'CPU 2.3% · 内存 64.0 MiB · 3 个进程');
  assert.match(metricsText({cpuSampled:false,memoryBytes:1024,partial:true}),/采集中 · 内存 ≥/);
  assert.equal(metricsText({error:'无法读取子进程内存'}),'无法读取子进程内存');
});
