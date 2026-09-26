import assert from 'node:assert/strict';
import path from 'node:path';

const code = 'if [ -f /etc/nginx/nginx.conf ]; then\n  printf "%s\\n" "<upstream> & ready"\nfi';
const markdown = [
  '## 排查结果',
  '',
  '**服务正常**，建议 *检查超时*，~~旧方案~~。配置文件是 `/etc/nginx/nginx.conf`。',
  '',
  '> 先检查，再修改配置。',
  '',
  '1. 查看日志',
  '   - 检查上游',
  '2. 验证服务',
  '',
  '- [x] 已读取配置',
  '- [ ] 等待执行',
  '',
  '| 服务 | 状态 |',
  '| :--- | ---: |',
  '| nginx | **正常** |',
  '| upstream | 待检查 |',
  '',
  '```bash', code, '```',
  '',
  '[查看文档](https://example.com/docs?q=nginx "服务文档")',
  '',
  '[危险链接](javascript:alert%281%29) ![示意图](https://example.com/diagram.png)',
  '',
  '<script>window.__markdownInjected = true</script>',
  '<img src=x onerror="window.__markdownInjected = true">',
].join('\n');

export async function testMarkdownChat(page, out) {
  // Saved conversations use the same renderer as live replies.
  await page.evaluate((content) => {
    window.__savedConversation = window.go.desktop.Service.GetConversation;
    window.go.desktop.Service.GetConversation = async () => [
      { role: 'assistant', content, time: new Date().toISOString() },
    ];
  }, markdown);
  await page.getByLabel('SSH 与 AI 会话列表').getByRole('button', { name: /检查磁盘空间/ }).click();
  const history = page.getByLabel('会话历史');
  await history.getByRole('heading', { name: '排查结果' }).waitFor();
  assert.equal(await history.locator('table tbody tr').count(), 2);
  assert.equal(await history.locator('.markdown-code pre').textContent(), code + '\n');

  await page.evaluate(() => {
    window.go.desktop.Service.GetConversation = window.__savedConversation;
    window.__test.trusted = true;
  });
  await page.getByRole('button', { name: '连接并继续', exact: true }).click();
  const workbench = page.locator('.native-workbench:visible');
  await workbench.getByText('服务器负载平稳', { exact: false }).waitFor();
  await page.evaluate((content) => {
    const id = window.__test.sessions.at(-1).id;
    const state = window.__test.workbenches[id];
    window.__markdownFixture = { id, saved: structuredClone(state) };
    window.__copiedCode = null;
    window.__openedLinks = [];
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: {
      writeText: async (text) => { window.__copiedCode = text; },
    } });
    window.runtime.BrowserOpenURL = (href) => window.__openedLinks.push(href);
    Object.assign(state, { busy: false, executing: false, proposal: null, status: '就绪', messages: [
      { role: 'user', content: '**保留用户原文**\n`echo "$HOME"`', time: new Date().toISOString() },
      { role: 'assistant', content, time: new Date().toISOString() },
    ] });
    window.__test.publishWorkbench(id);
  }, markdown);
  try {
    const reply = workbench.locator('.chat-message.assistant');
    const rendered = reply.locator('.markdown-content');
    await reply.getByRole('heading', { name: '排查结果' }).waitFor();
    assert.equal(await rendered.locator('strong').first().textContent(), '服务正常');
    assert.equal(await rendered.locator('em').textContent(), '检查超时');
    assert.equal(await rendered.locator('del').textContent(), '旧方案');
    assert.equal(await rendered.locator('blockquote').count(), 1);
    assert.equal(await rendered.locator('ol > li > ul > li').textContent(), '检查上游');
    const tasks = rendered.getByRole('checkbox');
    assert.equal(await tasks.count(), 2);
    assert.equal(await tasks.nth(0).isChecked(), true);
    assert.equal(await tasks.nth(1).isChecked(), false);
    assert.equal(await tasks.nth(0).isDisabled(), true);
    assert.equal(await rendered.locator('table tbody tr').count(), 2);
    assert.ok((await rendered.locator('table tbody tr').first().boundingBox()).height < 45, 'Markdown table rows must not inherit the settings table height');
    assert.equal(await rendered.locator('thead th').nth(1).evaluate(el => getComputedStyle(el).textAlign), 'right');
    assert.equal(await rendered.locator('.markdown-code pre').textContent(), code + '\n');
    await reply.getByRole('button', { name: '复制代码' }).click();
    assert.equal(await page.evaluate(() => window.__copiedCode), code, 'Copy preserves source whitespace and special characters');
    await reply.getByRole('button', { name: '复制代码' }).getByText('已复制').waitFor();
    await reply.getByRole('link', { name: '查看文档' }).click();
    assert.deepEqual(await page.evaluate(() => window.__openedLinks), ['https://example.com/docs?q=nginx']);
    assert.equal(await rendered.getByRole('link', { name: '危险链接' }).count(), 0);
    assert.equal(await rendered.locator('script, img').count(), 0);
    assert.equal(await page.evaluate(() => window.__markdownInjected), undefined);
    assert.equal(await workbench.locator('.chat-message.user .markdown-content').count(), 0);

    const messages = workbench.locator('.native-chat-messages');
    await messages.evaluate(el => { el.scrollTop = 0; el.dispatchEvent(new Event('scroll')); });
    await page.screenshot({ path: path.join(out, 'desktop-markdown-light.png') });
    await rendered.locator('.markdown-code').scrollIntoViewIfNeeded();
    await page.screenshot({ path: path.join(out, 'desktop-markdown-code.png') });

    // A fence can span many snapshots. Keep a single code block until it closes,
    // then render following table content without duplicating the partial reply.
    await page.evaluate(() => {
      const { id } = window.__markdownFixture;
      Object.assign(window.__test.workbenches[id], { busy: true, messages: [
        { role: 'assistant_stream', content: '### 正在生成\n\n```bash\nprintf "hello', time: new Date().toISOString() },
      ] });
      window.__test.publishWorkbench(id);
    });
    const streaming = workbench.locator('.assistant_stream');
    await streaming.getByRole('heading', { name: '正在生成' }).waitFor();
    assert.equal(await streaming.locator('pre').textContent(), 'printf "hello\n');
    await streaming.locator('pre').evaluate(el => { el.dataset.preserved = 'yes'; });
    await page.evaluate(() => {
      const { id } = window.__markdownFixture;
      const state = window.__test.workbenches[id];
      state.messages[0].content += ' world"\n```\n\n| 项目 | 结果 |\n| --- | --- |\n| 输出 | 完成 |';
      window.__test.publishWorkbench(id);
    });
    await streaming.getByRole('cell', { name: '完成', exact: true }).waitFor();
    assert.equal(await streaming.locator('pre').count(), 1);
    assert.equal(await streaming.locator('pre').getAttribute('data-preserved'), 'yes', 'Streaming preserves existing code DOM');
    assert.equal(await streaming.locator('pre').textContent(), 'printf "hello world"\n');
    await page.evaluate(() => {
      const { id } = window.__markdownFixture;
      const state = window.__test.workbenches[id];
      state.busy = false; state.messages[0].role = 'assistant';
      window.__test.publishWorkbench(id);
    });
    await streaming.waitFor({ state: 'detached' });
    assert.equal(await workbench.locator('.chat-message.assistant .markdown-code').count(), 1);

    await page.setViewportSize({ width: 940, height: 700 });
    await page.emulateMedia({ colorScheme: 'dark' });
    const wide = '### 长代码与宽表格\n\n```text\n' + 'nginx upstream '.repeat(40) + '\n```\n\n|' + ' 字段 |'.repeat(10) + '\n|' + ' --- |'.repeat(10) + '\n|' + ' 值 |'.repeat(10);
    await page.evaluate((content) => {
      const { id } = window.__markdownFixture;
      window.__test.workbenches[id].messages[0].content = content;
      window.__test.publishWorkbench(id);
    }, wide);
    await workbench.getByRole('heading', { name: '长代码与宽表格' }).waitFor();
    const bounds = await workbench.evaluate(el => {
      const chat = el.querySelector('.native-chat-messages');
      const code = el.querySelector('.markdown-code pre');
      const table = el.querySelector('.markdown-table');
      return { chatOverflow: chat.scrollWidth - chat.clientWidth, codeOverflow: code.scrollWidth - code.clientWidth,
        tableOverflow: table.scrollWidth - table.clientWidth, pageOverflow: document.documentElement.scrollWidth - innerWidth };
    });
    assert.ok(bounds.chatOverflow <= 1 && bounds.pageOverflow <= 1, JSON.stringify(bounds));
    assert.ok(bounds.codeOverflow > 100 && bounds.tableOverflow > 100, 'Wide content scrolls within its block');
    await page.screenshot({ path: path.join(out, 'desktop-markdown-dark-narrow.png') });
    console.log('PASS: Markdown in saved/live replies, streaming fences, exact code copy, links, HTML escaping and light/dark overflow layout');
  } finally {
    await page.evaluate(() => {
      const { id, saved } = window.__markdownFixture;
      window.__test.workbenches[id] = saved;
      window.__test.publishWorkbench(id);
      delete window.__markdownFixture;
    });
  }
}
