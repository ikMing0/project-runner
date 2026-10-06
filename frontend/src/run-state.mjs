export function isNewRun(previous, next) {
  return ['building', 'starting'].includes(next.state) && previous?.startedAt !== next.startedAt;
}

export function actionableError(text) {
  const value = text.replace(/^\[(?:ERROR|INFO)\]\s*/, '').trim();
  return !!value && !/^(?:COMPILATION ERROR|BUILD FAILURE|Failed to execute goal|\[Help|To see the full|Re-run Maven|For more information|->|→)/i.test(value);
}
