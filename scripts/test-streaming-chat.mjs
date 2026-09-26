import assert from 'node:assert/strict';
import path from 'node:path';

export async function testStreamingChat(page, workbench, out) {
  await page.evaluate(() => {
    const id = window.__test.sessions.at(-1).id;
    const state = window.__test.workbenches[id];
    window.__streamFixture = { id, saved: structuredClone(state) };
    Object.assign(state, {
      busy: true, status: '正在分析服务器', executing: false, proposal: null,
      messages: Array.from({ length: 24 }, (_, i) => ({
        role: i % 2 ? 'assistant' : 'user', content: '历史消息 ' + i,
        time: new Date(Date.now() - (24 - i) * 1000).toISOString(),
      })).concat([
        { role: 'activity', content: 'cat /etc/nginx/nginx.conf', time: new Date().toISOString(), activity: { id: 'tool', title: 'cat /etc/nginx/nginx.conf', kind: 'commandExecution', status: 'running', detail: '正在读取配置' } },
        { role: 'assistant_stream', content: '已经找到', time: new Date().toISOString() },
      ]),
    });
    window.__test.publishWorkbench(id);
  });
  try {
    const reply = workbench.locator('.assistant_stream');
    await reply.getByText('已经找到', { exact: true }).waitFor();
    assert.equal(await reply.getAttribute('aria-busy'), 'true');
    const row = workbench.locator('.chat-activity');
    assert.equal(await row.count(), 1);
    assert.equal(await row.getAttribute('open'), null);
    assert.ok((await row.boundingBox()).height < 40, 'Collapsed command occupies one line');
    await row.locator('summary').click();
    await page.evaluate(() => {
      const { id } = window.__streamFixture;
      const state = window.__test.workbenches[id];
      state.messages.at(-1).content += '上游超时的原因。';
      state.messages.at(-2).activity.detail += '\n读取完成：proxy_read_timeout 60s';
      state.messages.at(-2).activity.status = 'completed';
      window.__test.publishWorkbench(id);
    });
    await reply.getByText('已经找到上游超时的原因。', { exact: true }).waitFor();
    assert.equal(await row.count(), 1, 'Progress updates must replace the existing row');
    assert.notEqual(await row.getAttribute('open'), null, 'An expanded row stays open across events');
    await row.getByText('已完成', { exact: true }).waitFor();
    await row.locator('summary').click();
    await page.screenshot({ path: path.join(out, 'desktop-streaming-chat.png') });

    const messages = workbench.locator('.native-chat-messages');
    await messages.evaluate(el => { el.scrollTop = 0; el.dispatchEvent(new Event('scroll')); });
    await workbench.getByRole('button', { name: '回到最新消息' }).waitFor();
    await page.evaluate(() => {
      const { id } = window.__streamFixture;
      window.__test.workbenches[id].messages.at(-1).content += '\n下一步检查连接池。';
      window.__test.publishWorkbench(id);
    });
    await reply.getByText('下一步检查连接池。', { exact: false }).waitFor();
    assert.equal(await messages.evaluate(el => el.scrollTop), 0, 'Reading history must not snap back');
    await workbench.getByRole('button', { name: '回到最新消息' }).click();
    await page.waitForFunction(() => {
      const el = document.querySelector('.native-workbench:not([style*="none"]) .native-chat-messages');
      return el && el.scrollHeight - el.scrollTop - el.clientHeight < 2;
    });
    await page.evaluate(() => {
      const { id } = window.__streamFixture;
      const state = window.__test.workbenches[id];
      state.busy = false;
      state.messages.at(-1).role = 'assistant';
      state.messages.push({ role: 'error', content: '连接中断，请重试', time: new Date().toISOString() });
      window.__test.publishWorkbench(id);
    });
    await workbench.getByText('连接中断，请重试', { exact: true }).waitFor();
    assert.equal(await workbench.locator('.assistant_stream').count(), 0);
    assert.equal(await workbench.getByText('已经找到上游超时的原因。', { exact: false }).count(), 1, 'Partial answer survives failure without duplication');
  } finally {
    await page.evaluate(() => {
      const { id, saved } = window.__streamFixture;
      window.__test.workbenches[id] = saved;
      window.__test.publishWorkbench(id);
      delete window.__streamFixture;
    });
  }
}
