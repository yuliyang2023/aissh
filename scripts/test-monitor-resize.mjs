import assert from 'node:assert/strict';
import path from 'node:path';

export async function testMonitorResize(page, out) {
  await page.locator('.session-select').first().waitFor();
  await page.evaluate(() => { window.__test.trusted = true; });
  await page.getByRole('button', { name: '连接并继续', exact: true }).click();
  const workbench = page.locator('.native-workbench:visible');
  await workbench.getByText('服务器负载平稳', { exact: false }).waitFor();
  const actions = workbench.getByRole('group', { name: '监控操作' });
  assert.equal(await workbench.locator('.monitor-footer').count(), 0);
  assert.equal(await workbench.getByText('prod-app-01', { exact: true }).count(), 0);
  assert.equal(await workbench.getByRole('button', { name: '刷新当前监控', exact: true }).count(), 0);
  assert.ok(await actions.getByRole('button', { name: '当前会话告警监控', exact: true }).isVisible());
  assert.ok(await actions.getByRole('button', { name: '采集配置', exact: true }).isVisible());
  assert.equal(await workbench.locator('.native-chat-heading').getByRole('button', { name: '当前会话告警监控', exact: true }).count(), 0);
  await page.screenshot({ path: path.join(out, 'monitor-toolbar-normal.png') });
  const output = 'PID   USER   COMMAND   CPU\n' + Array.from({ length: 40 }, (_, i) => `${1000 + i}  ops    worker-${i}    ${i}.0`).join('\n');
  await page.evaluate((content) => {
    const id = window.__test.sessions[0].id;
    window.__test.workbenches[id].panels.push({ id: 'ai-custom', title: '自定义进程', kind: 'monitor', content, error: '', intervalMs: 2000, updatedAt: new Date().toISOString(), pinned: false });
    window.__test.publishWorkbench(id);
  }, output);
  const tabs = workbench.getByRole('group', { name: '监控项目' });
  await tabs.getByRole('button', { name: '自定义进程', exact: true }).click();
  await workbench.getByLabel('向 AI 提问').fill('保留这个未发送的输入');
  const normal = await workbench.locator('.monitor-output').boundingBox();
  await workbench.locator('.monitor-output').evaluate(el => { window.__monitorOutput = el; });
  await workbench.locator('.terminal-screen').evaluate(el => { window.__monitorTerminal = el; });
  const starts = await page.evaluate(() => window.__test.starts.length);

  async function checkLayout(maximized) {
    const monitor = await workbench.locator('.native-monitor').boundingBox();
    const workspace = await workbench.boundingBox();
    assert.ok(Math.abs(monitor.height - workspace.height * (maximized ? 1 : 0.5)) < 2, `monitor should use the selected share of the workspace: ${JSON.stringify({ monitor, workspace, maximized })}`);
    const toolbar = await workbench.locator('.monitor-toolbar').boundingBox();
    assert.ok(toolbar.y + toolbar.height <= monitor.y + monitor.height + 1, 'monitor actions must stay visible');
    assert.equal(await workbench.locator('.native-chat').isVisible(), !maximized);
    assert.equal(await workbench.locator('.native-ssh').isVisible(), !maximized);
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true);
  }

  await workbench.getByRole('button', { name: '放大监控', exact: true }).click();
  await checkLayout(false);
  assert.ok((await workbench.locator('.monitor-output').boundingBox()).height > normal.height + 60, 'large mode must enlarge the output, not just its container');
  await page.screenshot({ path: path.join(out, 'monitor-large.png') });
  const updated = output + '\n实时更新：worker-40';
  await page.evaluate(content => {
    const id = window.__test.sessions[0].id;
    window.__test.workbenches[id].panels[1].content = content;
    window.__test.publishWorkbench(id);
  }, updated);
  await workbench.locator('.monitor-output').getByText('实时更新：worker-40', { exact: false }).waitFor();
  await workbench.getByLabel('添加监控').selectOption('custom');
  await workbench.getByLabel('只读监控命令').fill('ps -ef');
  await workbench.getByRole('button', { name: '最大化监控', exact: true }).click();
  await checkLayout(true);
  assert.equal(await workbench.getByLabel('只读监控命令').inputValue(), 'ps -ef');
  assert.equal(await workbench.locator('.monitor-output').evaluate(el => el === window.__monitorOutput), true, 'resizing must not remount the output');
  assert.equal(await workbench.locator('.terminal-screen').evaluate(el => el === window.__monitorTerminal), true, 'resizing must not remount SSH');
  await page.screenshot({ path: path.join(out, 'monitor-maximized.png') });
  await tabs.getByRole('button', { name: '系统', exact: true }).click();
  assert.equal(await workbench.locator('.monitor-metric').count(), 4);
  await tabs.getByRole('button', { name: '自定义进程', exact: true }).click();
  await actions.getByRole('button', { name: '采集配置', exact: true }).click();
  await page.getByRole('dialog', { name: '采集配置', exact: true }).waitFor();
  await page.keyboard.press('Escape');
  await page.getByRole('dialog', { name: '采集配置', exact: true }).waitFor({ state: 'detached' });
  await checkLayout(true);
  await workbench.getByRole('button', { name: '还原监控', exact: true }).click();
  assert.equal(await workbench.getByLabel('向 AI 提问').inputValue(), '保留这个未发送的输入');
  assert.equal(await workbench.locator('.monitor-output').textContent(), updated);
  assert.equal(await workbench.locator('.native-ssh').isVisible(), true);
  await workbench.getByRole('button', { name: '取消添加监控', exact: true }).click();

  for (const viewport of [{ width: 940, height: 650 }, { width: 760, height: 640 }]) {
    await page.setViewportSize(viewport);
    await page.emulateMedia({ colorScheme: 'dark' });
    await workbench.getByRole('button', { name: '放大监控', exact: true }).click();
    await checkLayout(false);
    await workbench.getByRole('button', { name: '还原监控', exact: true }).click();
    await workbench.getByRole('button', { name: '放大监控', exact: true }).click();
    await workbench.getByRole('button', { name: '最大化监控', exact: true }).click();
    await checkLayout(true);
    await page.screenshot({ path: path.join(out, `monitor-maximized-${viewport.width}-dark.png`) });
    await workbench.getByRole('button', { name: '还原监控', exact: true }).press('Escape');
    assert.equal(await workbench.getByRole('button', { name: '放大监控', exact: true }).evaluate(el => el === document.activeElement), true);
  }
  await page.setViewportSize({ width: 1280, height: 840 });
  await workbench.locator('.monitor-heading').click();
  assert.equal(await workbench.locator('.monitor-details').count(), 0);
  await workbench.getByRole('button', { name: '放大监控', exact: true }).click();
  assert.equal(await workbench.locator('.monitor-output').textContent(), updated);
  await workbench.getByRole('button', { name: '还原监控', exact: true }).click();
  assert.equal(await workbench.locator('.monitor-details').count(), 0, 'restore must preserve the original collapsed preference');
  assert.equal(await page.evaluate(() => localStorage.getItem('aissh.monitor.expanded')), 'false');
  assert.equal(await page.evaluate(() => window.__test.starts.length), starts, 'resizing must not restart SSH');
  assert.deepEqual(await page.evaluate(() => window.__test.writes), [], 'resize shortcuts must not send SSH input');
  console.log('PASS: grouped monitoring controls, no hostname/footer/refresh, normal/large/maximized layouts, live data, config dialog, restore/Escape, drafts, SSH continuity, collapsed preference and narrow/dark layouts');
}
