// Compiler locations only; ordinary stack traces with ambiguous basenames stay text.
export function sourceLinks(text) {
  const paths = String.raw`((?:\/?[A-Za-z]:[\\/]|(?:\.{1,2}[\\/]|[A-Za-z_][\w.-]*[\\/]))[^\r\n<>:"'|?*]+?\.(?:java|kt|kts|groovy|scala|js|jsx|ts|tsx|vue|xml))["']?`;
  const formats = [
    new RegExp(paths + String.raw`(?::\s*)?\[(\d+)(?:,\s*(\d+))?\]`, 'gi'),
    new RegExp(paths + String.raw`:\s*(\d+)(?::(\d+))?`, 'gi'),
    new RegExp(paths + String.raw`:\s*\((\d+),\s*(\d+)\)`, 'gi'),
  ];
  const results = [];
  for (const pattern of formats) {
    for (const match of text.matchAll(pattern)) {
      if (match.index > 0 && /[\w\\/]/.test(text[match.index - 1])) continue;
      const path = match[1];
      if (/^[A-Za-z]:[/\\]{2}/.test(path) || path.includes('://')) continue;
      const line = Number(match[2]), column = Number(match[3] || 0);
      if (line < 1 || line > 10_000_000 || column > 1_000_000) continue;
      results.push({ start: match.index, end: match.index + match[0].length, path, line, column });
    }
  }
  return results.sort((a, b) => a.start - b.start).filter((value, index, all) =>
    !all.slice(0, index).some(previous => previous.end > value.start));
}
