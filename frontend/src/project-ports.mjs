function validPort(value) {
  const port = Number(value);
  return Number.isInteger(port) && port >= 1 && port <= 65535 ? port : null;
}

// Backends and frontends have separate sequences, but share the local port
// space. Standalone Node projects participate in the frontend sequence.
export function nextProjectPort(projects, kind, reserved = []) {
  const frontend = kind === 'node';
  const used = new Set(reserved.map(validPort).filter(port => port !== null));
  let highest = 0;
  function remember(value, sameSequence) {
    const port = validPort(value);
    if (port === null) return;
    used.add(port);
    if (sameSequence) highest = Math.max(highest, port);
  }
  for (const project of projects) {
    remember(project.port, (project.kind === 'node') === frontend);
    if (project.frontend) remember(project.frontend.port, frontend);
  }
  let candidate = highest ? highest + 1 : frontend ? 82 : 8080;
  while (candidate <= 65535 && used.has(candidate)) candidate++;
  return candidate <= 65535 ? candidate : null;
}
