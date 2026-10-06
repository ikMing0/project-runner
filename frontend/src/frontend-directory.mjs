// The desktop app runs on Windows. Compare path components rather than string
// prefixes so a nearby checkout (e.g. app-copy) is never treated as part of app.
function absoluteDirectory(value) {
  const path = String(value || '').trim().replaceAll('/', '\\');
  if (/[\x00-\x1f<>"|?*]/.test(path)) return null;
  const root = path.match(/^[a-z]:\\/i) || path.match(/^\\\\[^\\:]+\\[^\\:]+(?:\\|$)/);
  if (!root) return null;
  const parts = [];
  for (const part of path.slice(root[0].length).split('\\')) {
    if (!part || part === '.') continue;
    if (part === '..') parts.pop();
    else if (part.includes(':')) return null;
    else parts.push(part);
  }
  return { root: root[0].replace(/\\?$/, '\\'), parts };
}

export class FrontendDirectoryLink {
  constructor() { this.relative = null; }

  reset(backendDirectory, frontendDirectory) {
    const backend = absoluteDirectory(backendDirectory);
    const frontend = absoluteDirectory(frontendDirectory);
    this.relative = null;
    if (!backend || !frontend || backend.root.toLowerCase() !== frontend.root.toLowerCase()) return;
    if (backend.parts.length > frontend.parts.length) return;
    if (!backend.parts.every((part, i) => part.toLowerCase() === frontend.parts[i].toLowerCase())) return;
    this.relative = frontend.parts.slice(backend.parts.length).join('\\');
  }

  resolve(backendDirectory, frontendDirectory) {
    const backend = absoluteDirectory(backendDirectory);
    // Retain the relationship while the user clears/retypes the backend path.
    // An independent frontend remains exactly as entered by the user.
    if (!backend || this.relative === null) return frontendDirectory;
    const base = backend.root + backend.parts.join('\\');
    return this.relative ? `${base.replace(/\\$/, '')}\\${this.relative}` : base;
  }
}
