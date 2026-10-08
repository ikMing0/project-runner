import test from 'node:test';
import assert from 'node:assert/strict';
import { sourceLinks } from './source-links.mjs';
import { parseAnsiLog, renderAnsiLog } from './ansi-log.mjs';

test('Maven Windows locations with spaces and Unicode retain exact file, line and column', () => {
  const path = 'D:/new project/工作树/common/src/main/java/A.java';
  const text = `[ERROR] /${path}:[91,21] cannot implement method`;
  const [link] = sourceLinks(text);
  assert.equal(link.path, '/' + path);
  assert.equal(link.line, 91);
  assert.equal(link.column, 21);
  assert.equal(text.slice(link.start, link.end), '/' + path + ':[91,21]');
  const windows = String.raw`[ERROR] "D:\new project\app\src\main\java\B.java":[22,3]`;
  assert.equal(sourceLinks(windows)[0].path, String.raw`D:\new project\app\src\main\java\B.java`);
  const punctuation = 'D:/project (1)/[branch]/Sample & $(literal).java';
  assert.equal(sourceLinks(punctuation + ':[12,3]')[0].path, punctuation);
});

test('javac relative and Kotlin compiler locations are recognized without linking URLs or ambiguous frames', () => {
  assert.deepEqual(sourceLinks('src/main/java/A.java:42:7: error').map(link => [link.path, link.line, link.column]),
    [['src/main/java/A.java', 42, 7]]);
  assert.deepEqual(sourceLinks('D:/repo/src/main/kotlin/A.kt: (12, 4): error').map(link => [link.path, link.line, link.column]),
    [['D:/repo/src/main/kotlin/A.kt', 12, 4]]);
  assert.equal(sourceLinks('https://example.com/src/main/A.java:42 file:///D:/repo/A.java:91 at app.Main(Main.java:42)').length, 0);
  assert.equal(sourceLinks('D:/repo/A.java:0 D:/repo/B.java:10000001').length, 0);
  assert.equal(sourceLinks('D:/repo/A.java:[10,2] and D:/repo/B.java:20').length, 2);
});

test('ANSI split compiler locations and web URLs remain accessible with distinct callbacks', () => {
  const document = { createElement(tag) {
    const node = { tag, style: {}, children: [], listeners: {},
      append(...children) { this.children.push(...children); },
      addEventListener(type, listener) { this.listeners[type] = listener; } };
    Object.defineProperty(node, 'innerHTML', { set() { throw new Error('Never render log text as HTML'); } });
    return node;
  } };
  const element = { ownerDocument: document, replaceChildren(...children) { this.children = children; } };
  const opened = [], urls = [];
  const parsed = parseAnsiLog('[ERROR] \x1b[31mD:/repo/\x1b[1mA.java:[12,3]\x1b[0m See https://example.com/src/A.java:42 <img>');
  renderAnsiLog(element, parsed.runs, url => urls.push(url), { links: sourceLinks, open: location => opened.push(location) });
  const source = element.children.find(node => node.tag === 'button');
  assert.equal(source.type, 'button');
  assert.equal(source.children.map(node => node.textContent).join(''), 'D:/repo/A.java:[12,3]');
  assert.equal(source.children[0].style.color, '#f4838c');
  assert.equal(source.children[1].style.fontWeight, '700');
  source.listeners.click({ preventDefault() {} });
  assert.equal(opened[0].line, 12);
  const url = element.children.find(node => node.tag === 'a');
  url.listeners.click({ preventDefault() {} });
  assert.deepEqual(urls, ['https://example.com/src/A.java:42']);
  assert.equal(element.children.at(-1).textContent, ' <img>');
});
