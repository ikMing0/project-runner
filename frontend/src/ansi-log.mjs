// Render terminal colours as text spans; never execute control sequences or
// interpret log text as HTML. The actual terminal keeps its original stream.
const palette = ['#1b2535', '#f4838c', '#71dbaa', '#e9c886', '#78aef4', '#bfa2ef', '#81d4e0', '#dbe5f4',
  '#8193ac', '#ff9aa2', '#91efbe', '#ffe2a1', '#9bc3ff', '#d7b8ff', '#a1eaf2', '#ffffff'];
const controls = /(?:\x1b\[|\x9b)([0-?]*)([ -/]*)([@-~])|(?:\x1b\]|\x9d)[^\x07\x1b\x9c]*(?:\x07|\x1b\\|\x9c)|\x1b[ -/]*[@-Z\\-_]|(?:\x1b\[|\x9b)[0-?]*[ -/]*$|(?:\x1b\]|\x9d)[^\x07\x1b\x9c]*$/g;
const invisible = /[\x00-\x08\x0b-\x1f\x7f-\x9f]/g;

function colour(index) {
  if (!Number.isInteger(index) || index < 0 || index > 255) return undefined;
  if (index < 16) return palette[index];
  if (index >= 232) return `rgb(${Array(3).fill(8 + (index - 232) * 10).join(', ')})`;
  const levels = [0, 95, 135, 175, 215, 255];
  const value = index - 16;
  return `rgb(${[Math.floor(value / 36), Math.floor(value / 6) % 6, value % 6].map(i => levels[i]).join(', ')})`;
}

function applySGR(state, parameters) {
  // Both semicolon and colon forms occur in modern terminal output.
  const values = parameters === '' ? [0] : parameters.split(';').flatMap(value => {
    const parts = value.split(':');
    if (parts.length > 1 && (parts[0] === '38' || parts[0] === '48') && parts[1] === '2' && parts.length === 6) parts.splice(2, 1);
    return parts.map(part => part === '' ? 0 : Number(part));
  });
  for (let i = 0; i < values.length; i++) {
    const code = values[i];
    if (code === 0) { for (const key of Object.keys(state)) delete state[key]; }
    else if (code === 1) state.bold = true;
    else if (code === 2) state.dim = true;
    else if (code === 3) state.italic = true;
    else if (code === 4) state.underline = true;
    else if (code === 7) state.inverse = true;
    else if (code === 22) { delete state.bold; delete state.dim; }
    else if (code === 23) delete state.italic;
    else if (code === 24) delete state.underline;
    else if (code === 27) delete state.inverse;
    else if (code === 39) delete state.foreground;
    else if (code === 49) delete state.background;
    else if (code >= 30 && code <= 37) state.foreground = palette[code - 30];
    else if (code >= 90 && code <= 97) state.foreground = palette[code - 90 + 8];
    else if (code >= 40 && code <= 47) state.background = palette[code - 40];
    else if (code >= 100 && code <= 107) state.background = palette[code - 100 + 8];
    else if (code === 38 || code === 48) {
      let value;
      if (values[i + 1] === 5) { value = colour(values[i + 2]); i += 2; }
      else if (values[i + 1] === 2) {
        const channels = values.slice(i + 2, i + 5);
        if (channels.length === 3 && channels.every(channel => Number.isInteger(channel) && channel >= 0 && channel <= 255)) value = `rgb(${channels.join(', ')})`;
        i += 4;
      }
      if (value) state[code === 38 ? 'foreground' : 'background'] = value;
    }
  }
}

export function parseAnsiLog(value) {
  const input = String(value ?? '');
  const runs = [];
  const state = {};
  let offset = 0;
  function append(value) {
    const text = value.replace(invisible, '');
    if (text) runs.push({ text, ...state });
  }
  for (const match of input.matchAll(controls)) {
    append(input.slice(offset, match.index));
    if (match[3] === 'm' && match[2] === '') applySGR(state, match[1]);
    offset = match.index + match[0].length;
  }
  append(input.slice(offset));
  return { text: runs.map(run => run.text).join(''), runs };
}

export function logLinks(text) {
  const links = [];
  for (const match of text.matchAll(/\bhttps?:\/\/[^\s<>"'`，。；：！？、（）【】《》]+/gi)) {
    let url = match[0];
    // Log messages often wrap a URL in brackets or end it with punctuation.
    // Keep balanced brackets, including IPv6 hosts and parentheses in paths.
    while (url) {
      const last = url.at(-1);
      const opening = { ')': '(', ']': '[', '}': '{' }[last];
      if (/[.,;:!?，。；：！？、…）】》]/.test(last) ||
          (opening && url.split(last).length > url.split(opening).length)) url = url.slice(0, -1);
      else break;
    }
    try {
      const parsed = new URL(url);
      if (parsed.hostname && ['http:', 'https:'].includes(parsed.protocol) && !url.includes('\\')) {
        links.push({ start: match.index, end: match.index + url.length, url });
      }
    } catch { /* Invalid addresses stay ordinary log text. */ }
  }
  return links;
}

export function renderAnsiLog(element, runs, openURL) {
  const document = element.ownerDocument;
  function styledSpan(run, text) {
    const span = document.createElement('span');
    span.textContent = text;
    const foreground = run.inverse ? (run.background || palette[0]) : run.foreground;
    const background = run.inverse ? (run.foreground || palette[7]) : run.background;
    if (foreground) span.style.color = foreground;
    if (background) span.style.backgroundColor = background;
    if (run.bold) span.style.fontWeight = '700';
    if (run.dim) span.style.opacity = '0.8';
    if (run.italic) span.style.fontStyle = 'italic';
    if (run.underline) span.style.textDecoration = 'underline';
    return span;
  }
  if (typeof openURL !== 'function') {
    element.replaceChildren(...runs.map(run => styledSpan(run, run.text)));
    return;
  }
  const text = runs.map(run => run.text).join('');
  const links = logLinks(text);
  if (!links.length) {
    element.replaceChildren(...runs.map(run => styledSpan(run, run.text)));
    return;
  }
  function styledRange(start, end) {
    const spans = [];
    let offset = 0;
    for (const run of runs) {
      const next = offset + run.text.length;
      if (next > start && offset < end) spans.push(styledSpan(run, run.text.slice(Math.max(0, start - offset), end - offset)));
      offset = next;
      if (offset >= end) break;
    }
    return spans;
  }
  const children = [];
  let offset = 0;
  for (const link of links) {
    children.push(...styledRange(offset, link.start));
    const anchor = document.createElement('a');
    anchor.className = 'log-link';
    anchor.href = link.url;
    anchor.title = `用默认浏览器打开 ${link.url}`;
    anchor.target = '_blank';
    anchor.rel = 'noopener noreferrer';
    anchor.append(...styledRange(link.start, link.end));
    anchor.addEventListener('click', event => {
      event.preventDefault();
      openURL(link.url);
    });
    children.push(anchor);
    offset = link.end;
  }
  children.push(...styledRange(offset, text.length));
  element.replaceChildren(...children);
}
