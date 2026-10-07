import { test } from 'node:test';
import assert from 'node:assert/strict';
import { parseAnsiLog, renderAnsiLog, logLinks } from './ansi-log.mjs';

test('Vite startup retains colours and emphasis without leaking escape codes into visible text', () => {
  const parsed = parseAnsiLog('\x1b[32m\x1b[1mVITE\x1b[22m v6.4.3\x1b[39m  \x1b[2mready in \x1b[0m\x1b[1m1374\x1b[22m ms');
  assert.equal(parsed.text, 'VITE v6.4.3  ready in 1374 ms');
  const vite = parsed.runs.find(run => run.text === 'VITE');
  assert.equal(vite.foreground, '#71dbaa');
  assert.equal(vite.bold, true);
  assert.equal(parsed.runs.find(run => run.text === ' v6.4.3').bold, undefined);
  assert.equal(parsed.runs.find(run => run.text === 'ready in ').dim, true);
  assert.equal(parsed.runs.at(-1).foreground, undefined);
});

test('standard, bright, 256-colour and true-colour styles reset independently', () => {
  const parsed = parseAnsiLog('\x1b[94;43;3;4m地址\x1b[39mplain\x1b[49;23;24mnormal\x1b[38;5;196mred\x1b[38;2;12;34;56mRGB\x1b[48:2::65:43:21mbg\x1b[0mreset');
  assert.equal(parsed.text, '地址plainnormalredRGBbgreset');
  assert.deepEqual(parsed.runs[0], {text:'地址',foreground:'#9bc3ff',background:'#e9c886',italic:true,underline:true});
  assert.equal(parsed.runs[1].foreground, undefined);
  assert.equal(parsed.runs[1].background, '#e9c886');
  assert.deepEqual(parsed.runs[2], {text:'normal'});
  assert.equal(parsed.runs[3].foreground, 'rgb(255, 0, 0)');
  assert.equal(parsed.runs[4].foreground, 'rgb(12, 34, 56)');
  assert.equal(parsed.runs[5].background, 'rgb(65, 43, 21)');
  assert.deepEqual(parsed.runs.at(-1), {text:'reset'});
});

test('OSC hyperlinks, title changes, cursor controls and incomplete escapes never appear in the log', () => {
  const parsed = parseAnsiLog('\x1b]0;unwanted title\x07\x1b[2K中文\x1b[1G \x1b]8;;https://example.invalid\x1b\\地址\x1b]8;;\x1b\\\x00\x9b31m错误\x9b0m\x1b[31;');
  assert.equal(parsed.text, '中文 地址错误');
  assert.equal(parsed.runs.at(-1).foreground, '#f4838c');
});

test('malformed colours cannot insert arbitrary CSS and ordinary text stays intact', () => {
  const input = '前端\t启动 <img src=x onerror=alert(1)>\x1b[38;2;999;0;0m text\x1b[999m!';
  const parsed = parseAnsiLog(input);
  assert.equal(parsed.text, '前端\t启动 <img src=x onerror=alert(1)> text!');
  assert.ok(parsed.runs.every(run => run.foreground === undefined));
});

test('renderer treats HTML-looking log output as text while applying the decoded colour', () => {
  const document = {createElement() {
    const span = {style:{}};
    Object.defineProperty(span, 'innerHTML', {set() { throw new Error('Log text must not become HTML'); }});
    return span;
  }};
  const element = {ownerDocument:document, replaceChildren(...children) { this.children = children; }};
  renderAnsiLog(element, parseAnsiLog('\x1b[31m<img onerror=alert(1)>\x1b[0m').runs);
  assert.equal(element.children[0].textContent, '<img onerror=alert(1)>');
  assert.equal(element.children[0].style.color, '#f4838c');
});

test('log addresses keep ports, IPv6, query strings and balanced paths without trailing punctuation', () => {
  const text = 'Local: http://localhost:82/ Network: http://192.168.6.15:82/ [http://[::1]:82/] (https://example.com/a_(b)?x=1&y=2#section)。';
  const expected = ['http://localhost:82/', 'http://192.168.6.15:82/', 'http://[::1]:82/', 'https://example.com/a_(b)?x=1&y=2#section'];
  const links = logLinks(text);
  assert.deepEqual(links.map(link => link.url), expected);
  assert.deepEqual(links.map(link => text.slice(link.start, link.end)), expected);
  assert.deepEqual(logLinks('地址：http://localhost:82/，服务已启动。').map(link => link.url), ['http://localhost:82/']);
  assert.deepEqual(logLinks('javascript:alert(1) file:///C:/secret http:// https://:82/ http://localhost:99999/'), []);
});

test('a URL split by ANSI colours remains one keyboard accessible external link and log markup stays text', () => {
  const document = {createElement(tag) {
    const node = {tag, style:{}, children:[], listeners:{}, append(...children) { this.children.push(...children); },
      addEventListener(name, listener) { this.listeners[name] = listener; }};
    Object.defineProperty(node, 'innerHTML', {set() { throw new Error('Log text must not become HTML'); }});
    return node;
  }};
  const element = {ownerDocument:document, replaceChildren(...children) { this.children = children; }};
  const opened = [];
  renderAnsiLog(element, parseAnsiLog('Local: \x1b[36mhttp://local\x1b[1mhost:82/\x1b[0m <img onerror=alert(1)>').runs, url => opened.push(url));
  const anchors = element.children.filter(node => node.tag === 'a');
  assert.equal(anchors.length, 1);
  const anchor = anchors[0];
  assert.equal(anchor.href, 'http://localhost:82/');
  assert.equal(anchor.children.map(node => node.textContent).join(''), anchor.href);
  assert.equal(anchor.children[0].style.color, '#81d4e0');
  assert.equal(anchor.children[1].style.fontWeight, '700');
  assert.equal(anchor.rel, 'noopener noreferrer');
  let prevented = false;
  anchor.listeners.click({preventDefault() { prevented = true; }});
  assert.equal(prevented, true);
  assert.deepEqual(opened, ['http://localhost:82/']);
  assert.equal(element.children.at(-1).textContent, ' <img onerror=alert(1)>');
});
