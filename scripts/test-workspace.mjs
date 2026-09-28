import assert from 'node:assert/strict';
import path from 'node:path';

// Run through test-desktop-ui.mjs with WORKSPACE_ONLY=1.
export async function testWorkspace(page, out) {
  await page.locator('.session-select').first().waitFor();
  await page.evaluate(() => { window.__test.trusted = true; });
  await page.getByRole('button', { name: '新建会话', exact: true }).first().click();
  const creation = page.getByRole('dialog', { name: '新建 AI 对话' });
  await creation.getByLabel('服务器', { exact: true }).selectOption('host-one');
  await creation.getByRole('button', { name: '选择工作目录', exact: true }).click();
  assert.equal(await creation.getByLabel('AI 工作目录（本机）', { exact: true }).inputValue(), '/Users/example/project');
  await creation.getByRole('button', { name: '创建对话', exact: true }).click();
  await creation.waitFor({ state: 'detached' });
  assert.equal(await page.evaluate(() => window.__test.conversations[0].workspace), '/Users/example/project');
  assert.equal(await page.evaluate(() => window.__test.data.hosts[0].workspace), '', 'per-chat directory must preserve the host default');
  await page.getByRole('button', { name: '连接并开始', exact: true }).click();
  const workbench = page.locator('.native-workbench:visible');
  const directory = workbench.getByLabel('当前 AI 工作目录', { exact: true });
  await directory.getByText('/Users/example/project', { exact: true }).waitFor();
  await directory.getByRole('button', { name: '切换目录' }).click();
  const editor = page.getByRole('dialog', { name: '切换 AI 工作目录' });
  const input = editor.getByLabel('AI 工作目录（本机）', { exact: true });
  await input.fill('/missing');
  await editor.getByRole('button', { name: '应用到当前对话' }).click();
  await editor.getByRole('alert').getByText('无法访问本机工作目录').waitFor();
  assert.equal(await input.inputValue(), '/missing');
  await input.fill('/Users/example/another project');
  await editor.getByRole('button', { name: '应用到当前对话' }).click();
  await editor.waitFor({ state: 'detached' });
  await directory.getByText('/Users/example/another project', { exact: true }).waitFor();
  await directory.getByText('只读', { exact: true }).waitFor();
  await page.screenshot({ path: path.join(out, 'workspace-current.png') });

  // Simulate backend task states to verify controls cannot switch mid-turn.
  for (const state of [{ busy: true }, { executing: true }, { proposal: { id: 'pending', command: 'uptime', risk: 'low' } }]) {
    await page.evaluate(state => {
      const id = window.__test.sessions.at(-1).id;
      Object.assign(window.__test.workbenches[id], state);
      window.__test.publishWorkbench(id);
    }, state);
    await page.waitForFunction(() => document.querySelector('.native-workbench:not([style*="display: none"]) .chat-workspace button')?.disabled);
    await page.evaluate(() => {
      const id = window.__test.sessions.at(-1).id;
      Object.assign(window.__test.workbenches[id], { busy: false, executing: false, proposal: undefined });
      window.__test.publishWorkbench(id);
    });
  }
  await directory.getByRole('button', { name: '切换目录' }).click();
  await editor.getByRole('button', { name: '清空目录' }).click();
  await editor.getByRole('button', { name: '应用到当前对话' }).click();
  await editor.waitFor({ state: 'detached' });
  await directory.getByText('未设置本机工作目录', { exact: true }).waitFor();
  assert.equal(await page.evaluate(() => window.__test.conversations[0].workspace), '');
  assert.deepEqual(await page.evaluate(() => window.__test.writes), [], 'directory changes must never reach the SSH shell');
  await page.evaluate(() => {
    const id = window.__test.sessions.at(-1).id;
    window.__test.workbenches[id].workspaceSupported = false;
    window.__test.publishWorkbench(id);
  });
  await directory.getByText('当前 AI 不支持本机工作目录', { exact: true }).waitFor();
  assert.equal(await directory.getByRole('button').count(), 0);

  await page.setViewportSize({ width: 940, height: 650 });
  await page.getByRole('button', { name: '新建会话', exact: true }).first().click();
  await creation.getByRole('button', { name: '直接连接', exact: true }).click();
  await creation.getByLabel('主机地址', { exact: true }).fill('192.0.2.88');
  await creation.getByRole('button', { name: '选择工作目录', exact: true }).click();
  await creation.getByLabel('AI 工作目录（本机）', { exact: true }).fill('/Users/example/direct project');
  await page.screenshot({ path: path.join(out, 'workspace-new-direct.png') });
  const create = creation.getByRole('button', { name: '连接并新建对话' });
  assert.equal(await create.evaluate(el => el.getBoundingClientRect().bottom <= innerHeight), true);
  await create.click();
  await creation.waitFor({ state: 'detached' });
  await page.locator('.native-workbench:visible').getByLabel('当前 AI 工作目录', { exact: true }).getByText('/Users/example/direct project', { exact: true }).waitFor();
  assert.equal(await page.evaluate(() => window.__test.data.conversationHosts.at(-1).workspace), '/Users/example/direct project');
  console.log('PASS: new existing/direct conversation directories, current directory edits and errors, disabled access, task guards, unsupported providers, and no SSH injection');
}
