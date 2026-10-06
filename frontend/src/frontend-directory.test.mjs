import { test } from 'node:test';
import assert from 'node:assert/strict';
import { FrontendDirectoryLink } from './frontend-directory.mjs';

test('copied checkout follows the backend repeatedly while keeping the frontend subdirectory', () => {
  const link = new FrontendDirectoryLink();
  link.reset('D:\\new-project\\data-visual-manage-global-inventory', 'D:\\new-project\\data-visual-manage-global-inventory\\ruoyi-ui');
  assert.equal(link.relative, 'ruoyi-ui');
  assert.equal(link.resolve('D:\\new-project\\data-visual-manage', ''), 'D:\\new-project\\data-visual-manage\\ruoyi-ui');
  assert.equal(link.resolve('E:\\工作树 空格\\branch-b', ''), 'E:\\工作树 空格\\branch-b\\ruoyi-ui');
  assert.equal(link.resolve('D:\\new-project\\data-visual-manage-global-inventory', ''), 'D:\\new-project\\data-visual-manage-global-inventory\\ruoyi-ui');
});

test('clearing and retyping the backend does not lose the link or replace it with a relative path', () => {
  const link = new FrontendDirectoryLink();
  const frontend = 'D:\\old\\web\\client';
  link.reset('D:\\old', frontend);
  for (const partial of ['', 'E', 'E:', 'relative\\path', 'E:\\bad?name']) {
    assert.equal(link.resolve(partial, frontend), frontend);
  }
  assert.equal(link.resolve('E:\\new', frontend), 'E:\\new\\web\\client');
});

test('Windows separators, case, roots and UNC paths preserve a normalized relationship', () => {
  const link = new FrontendDirectoryLink();
  link.reset('d:/Projects/App/', 'D:\\projects\\app\\web\\..\\ruoyi-ui\\');
  assert.equal(link.resolve('E:/Projects/Next/', ''), 'E:\\Projects\\Next\\ruoyi-ui');
  link.reset('D:\\', 'd:\\ruoyi-ui');
  assert.equal(link.resolve('E:\\', ''), 'E:\\ruoyi-ui');
  link.reset('\\\\server\\share\\app', '\\\\SERVER\\SHARE\\app\\ui');
  assert.equal(link.resolve('\\\\other\\worktrees\\new-app', ''), '\\\\other\\worktrees\\new-app\\ui');
  link.reset('D:\\app', 'D:\\app');
  assert.equal(link.resolve('E:\\next', ''), 'E:\\next');
});

test('independent or similarly named frontend paths remain unchanged', () => {
  const link = new FrontendDirectoryLink();
  for (const frontend of ['D:\\app-copy\\ruoyi-ui', 'E:\\app\\ruoyi-ui', 'D:\\app\\..\\shared-ui', 'D:\\shared-ui', '']) {
    link.reset('D:\\app', frontend);
    assert.equal(link.relative, null);
    assert.equal(link.resolve('D:\\next-app', frontend), frontend);
  }
});

test('manually selecting a new frontend refreshes or removes the link', () => {
  const link = new FrontendDirectoryLink();
  link.reset('D:\\app', 'D:\\app\\ruoyi-ui');
  link.reset('D:\\app', 'D:\\app\\packages\\web');
  assert.equal(link.resolve('D:\\next', ''), 'D:\\next\\packages\\web');
  link.reset('D:\\next', 'D:\\independent-ui');
  assert.equal(link.resolve('D:\\third', 'D:\\independent-ui'), 'D:\\independent-ui');
  link.reset('D:\\third', 'D:\\third\\client');
  assert.equal(link.resolve('D:\\fourth', ''), 'D:\\fourth\\client');
});
