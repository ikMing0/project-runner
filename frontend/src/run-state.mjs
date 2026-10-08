export function isNewRun(previous, next) {
  return ['waiting', 'checking', 'building', 'starting'].includes(next.state) && previous?.startedAt !== next.startedAt;
}

export function actionableError(text) {
  const value = text.replace(/^\[(?:ERROR|INFO)\]\s*/, '').trim();
  return !!value && !/^(?:COMPILATION ERROR|BUILD FAILURE|Failed to execute goal|\[Help|To see the full|Re-run Maven|For more information|->|→)/i.test(value);
}

export function serviceIDs(project) {
  return project.frontend ? [project.id, `${project.id}:frontend`] : [project.id];
}

export function activeState(state) {
  return ['waiting', 'checking', 'building', 'starting', 'running', 'unready'].includes(state);
}

export function currentAttemptLog(line, status) {
  return (line.attempt || 1) >= (status?.attempt || 1);
}

export function recoveryText(status) {
  return ({building:'自动清理重建中', starting:'重建后再次启动', recovered:'已自动恢复', failed:'自动恢复未成功', cancelled:'自动恢复已取消'})[status?.recovery] || '';
}

export function groupState(project, statuses) {
  const states = serviceIDs(project).map((id) => statuses.get(id)?.state || 'stopped');
  if (states.every((state) => state === 'running')) return 'running';
  if (states.includes('building')) return 'building';
  if (states.includes('checking')) return 'checking';
  if (states.includes('starting')) return 'starting';
  if (states.includes('unready')) return 'unready';
  if (states.includes('waiting')) return 'starting';
  if (states.some(activeState)) return 'partial';
  if (states.includes('failed')) return 'failed';
  return 'stopped';
}
