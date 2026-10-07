import { nextProjectPort } from './project-ports.mjs';

export const ideaFieldKeys = {
  name:'name', kind:'kind', port:'port', module:'module', 'java-home':'javaHome', 'tool-path':'toolPath',
  'config-file':'configFile', 'config-property':'configProperty', 'jvm-args':'jvmArgs', 'app-args':'appArgs',
  environment:'environment', manager:'packageManager', script:'script', 'port-mode':'portMode',
  'node-home':'nodeHome', 'node-tool':'toolPath', 'frontend-enabled':'frontend.enabled',
  'frontend-directory':'frontend.directory', 'frontend-port':'frontend.port', 'frontend-script':'frontend.script',
  'frontend-manager':'frontend.packageManager', 'frontend-port-mode':'frontend.portMode',
  'frontend-node-home':'frontend.nodeHome', 'frontend-tool':'frontend.toolPath', 'frontend-args':'frontend.appArgs',
  'frontend-environment':'frontend.environment', 'frontend-auto-proxy':'frontend.autoProxy',
  'frontend-proxy-variable':'frontend.proxyVariable',
};

const fields = new Set(['kind', 'directory', 'module', 'javaHome', 'toolPath', 'configFile', 'configProperty',
  'jvmArgs', 'appArgs', 'environment', 'packageManager', 'script', 'portMode', 'nodeHome']);
const frontendFields = new Set(['directory', 'toolPath', 'appArgs', 'environment', 'packageManager', 'script', 'portMode', 'nodeHome']);

export function ideaChoices(catalog, kind) {
  const configs = catalog.configurations || [];
  const backend = configs.filter(item => item.kind !== 'node');
  const frontend = configs.filter(item => item.kind === 'node');
  return kind === 'node' || !backend.length
    ? { primary:frontend, frontend:[] } : { primary:backend, frontend };
}

// Automatic detection preserves manual edits; an explicit import updates the
// selected fields for review before saving. Neither path mutates the saved data.
export function mergeIDEAConfiguration(draft, frontendDraft, primary, frontend, projects, edited = new Set(), force = false) {
  const project = structuredClone(draft);
  let paired = structuredClone(frontendDraft || draft.frontend || {});
  const messages = [];
  const previous = projects.filter(item => !draft.id || item.id !== draft.id);
  const protectedField = key => !force && edited.has(key);
  function apply(target, values, allowed, prefix = '') {
    for (const [key, value] of Object.entries(values || {})) {
      if (allowed.has(key) && !protectedField(prefix + key)) target[key] = structuredClone(value);
    }
  }
  function allocate(values, current, kind, prefix = '', reserved = []) {
    if (protectedField(prefix + 'port')) return current;
    const proposed = Number(values?.port) || Number(current);
    const used = new Set(reserved);
    for (const item of previous) {
      used.add(Number(item.port));
      if (item.frontend) used.add(Number(item.frontend.port));
    }
    if (Number.isInteger(proposed) && proposed >= 1 && proposed <= 65535 && !used.has(proposed)) return proposed;
    const next = nextProjectPort(previous, kind, reserved);
    if (values?.port) messages.push((prefix || kind === 'node' ? '前端' : '后端') + ' IDEA 端口 ' + values.port + ' 已被配置使用，建议 ' + (next ?? '手动选择可用端口'));
    if (next === null) messages.push('历史端口已到 65535，请手动填写可用端口');
    return next;
  }
  if (primary) {
    apply(project, primary.values, fields);
    const retainedFrontendPort = !frontend && project.frontend ? [project.frontend.port] : [];
    project.port = allocate(primary.values, project.port, project.kind, '', retainedFrontendPort);
    messages.push(...(primary.warnings || []));
  }
  if (frontend && project.kind !== 'node') {
    apply(paired, frontend.values, frontendFields, 'frontend.');
    paired.port = allocate(frontend.values, paired.port, 'node', 'frontend.', [project.port]);
    if (!paired.proxyVariable) paired.proxyVariable = 'VUE_APP_BASE_API_TARGET';
    if (paired.autoProxy === undefined) paired.autoProxy = true;
    if (!protectedField('frontend.autoProxy') && !protectedField('frontend.environment')) {
      const proxyKey = Object.keys(paired.environment || {}).find(key => key.toLowerCase() === paired.proxyVariable.toLowerCase());
      if (proxyKey) {
        let followsBackend = false;
        try {
          const url = new URL(paired.environment[proxyKey]);
          followsBackend = url.protocol === 'http:' && ['localhost','127.0.0.1','[::1]'].includes(url.hostname) &&
            Number(url.port || 80) === Number(primary?.values?.port || project.port) && url.pathname === '/' && !url.search && !url.hash;
        } catch { /* Keep explicitly configured remote/custom proxy addresses. */ }
        paired.autoProxy = followsBackend;
        if (followsBackend) delete paired.environment[proxyKey];
      }
    }
    if (!protectedField('frontend.enabled') || project.frontend) project.frontend = paired;
    messages.push(...(frontend.warnings || []));
  } else if (project.kind === 'node') {
    project.frontend = null;
  }
  return { project, frontend:paired, messages:[...new Set(messages)] };
}
