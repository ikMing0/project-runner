import { test } from 'node:test';
import assert from 'node:assert/strict';
import { ideaChoices, mergeIDEAConfiguration } from './idea-import.mjs';

const backend = {kind:'spring-maven',values:{kind:'spring-maven',directory:'D:\\new',module:'ruoyi-admin',port:8081,
  javaHome:'C:\\jdk21',configFile:'D:\\config\\application.properties',configProperty:'application.config.path',
  jvmArgs:'-Xmx512m',appArgs:'--spring.profiles.active=local',environment:{MODE:'local'}},warnings:[]};
const frontend = {kind:'node',values:{kind:'node',directory:'D:\\new\\ruoyi-ui',port:82,script:'dev:vite',
  packageManager:'npm',portMode:'vite',nodeHome:'C:\\node',toolPath:'C:\\node\\npm.cmd',appArgs:'',
  environment:{VUE_APP_BASE_API_TARGET:'http://localhost:8081'}},warnings:[]};
const draft = {id:'',name:'new',kind:'spring-maven',directory:'D:\\new',port:8080,environment:{},frontend:null};

test('IDEA pair imports into an editable draft and conflicting historical ports advance both sides', () => {
  const history = [
    {id:'main',kind:'spring-maven',port:8081,frontend:{port:82}},
    {id:'crm',kind:'spring-maven',port:8084,frontend:{port:85}},
  ];
  const original = structuredClone(draft);
  const result = mergeIDEAConfiguration(draft,{port:86},backend,frontend,history);
  assert.equal(result.project.port,8085);
  assert.equal(result.project.frontend.port,86);
  assert.equal(result.project.frontend.autoProxy,true);
  assert.equal(result.project.frontend.environment.VUE_APP_BASE_API_TARGET,undefined);
  assert.equal(result.project.configFile,backend.values.configFile);
  assert.equal(result.messages.length,2);
  assert.deepEqual(draft,original);
});

test('automatic imports preserve manual fields, including edits made while detection is pending', () => {
  const current = {...draft,port:9000,javaHome:'C:\\manualJDK',environment:{MANUAL:'yes'}};
  const edited = new Set(['port','javaHome','environment','frontend.port','frontend.enabled']);
  const result = mergeIDEAConfiguration(current,{port:3000},backend,frontend,[],edited);
  assert.equal(result.project.port,9000);
  assert.equal(result.project.javaHome,'C:\\manualJDK');
  assert.deepEqual(result.project.environment,{MANUAL:'yes'});
  assert.equal(result.frontend.port,3000);
  assert.equal(result.project.frontend,null);
  const explicit = mergeIDEAConfiguration(current,{port:3000},backend,frontend,[],edited,true);
  assert.equal(explicit.project.port,8081);
  assert.equal(explicit.project.javaHome,'C:\\jdk21');
  assert.equal(explicit.project.frontend.port,82);
});

test('saved configuration reimport excludes its own ports and preserves a remote proxy', () => {
  const current = {...draft,id:'self',port:8081};
  const remote = structuredClone(frontend);
  remote.values.environment.VUE_APP_BASE_API_TARGET = 'https://api.example.com/';
  const result = mergeIDEAConfiguration(current,{port:82},backend,remote,[{...current,frontend:{port:82}}],new Set(),true);
  assert.equal(result.project.port,8081);
  assert.equal(result.project.frontend.port,82);
  assert.equal(result.project.frontend.autoProxy,false);
  assert.equal(result.project.frontend.environment.VUE_APP_BASE_API_TARGET,'https://api.example.com/');
});

test('manual frontend environment remains intact and backend-only import avoids its retained paired port', () => {
  const paired = {port:8081,autoProxy:false,proxyVariable:'VUE_APP_BASE_API_TARGET',environment:{VUE_APP_BASE_API_TARGET:'http://localhost:8081'}};
  const manual = mergeIDEAConfiguration({...draft,frontend:paired},paired,backend,frontend,[],new Set(['frontend.environment']));
  assert.equal(manual.project.frontend.autoProxy,false);
  assert.equal(manual.project.frontend.environment.VUE_APP_BASE_API_TARGET,'http://localhost:8081');
  const backendOnly = mergeIDEAConfiguration({...draft,frontend:paired},paired,backend,null,[]);
  assert.equal(backendOnly.project.frontend.port,8081);
  assert.notEqual(backendOnly.project.port,8081);
});

test('frontend-only projects use Node history suggestions when IDEA omitted the port, and never overflow', () => {
  const noPort = structuredClone(frontend);
  delete noPort.values.port;
  const catalog = {configurations:[backend,noPort]};
  assert.deepEqual(ideaChoices(catalog,'node'),{primary:[noPort],frontend:[]});
  assert.deepEqual(ideaChoices(catalog,'spring-maven'),{primary:[backend],frontend:[noPort]});
  const result = mergeIDEAConfiguration({...draft,kind:'node',port:86},null,noPort,null,[]);
  assert.equal(result.project.port,86);
  assert.equal(result.project.frontend,null);
  const exhausted = mergeIDEAConfiguration(draft,null,backend,null,[{kind:'spring-maven',port:65535},{port:8081}]);
  assert.equal(exhausted.project.port,null);
});
