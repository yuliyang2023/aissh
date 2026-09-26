import assert from 'node:assert/strict';
import path from 'node:path';

// Run through test-desktop-ui.mjs with COMMAND_REVIEW_ONLY=1.
export async function testCommandReview(page, out) {
  await page.evaluate(() => {
    window.__test.trusted = true;
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: { writeText: async (text) => { window.__test.copiedCommand = text; } },
    });
  });
  await page.getByRole('button', { name: '连接并继续', exact: true }).click();
  await page.locator('.native-workbench:visible .native-chat-messages').getByText('服务器负载平稳', { exact: false }).waitFor();
  const sample = [
    '# 检查服务与最近日志（只读）',
    'service="nginx"',
    'log_file="/var/log/nginx/error.log"',
    '',
    'if command -v systemctl >/dev/null 2>&1; then',
    '  systemctl status "$service" --no-pager',
    'fi',
    '',
    '# 筛选错误、超时和上游连接异常',
    'sudo tail -n 200 "$log_file" | grep -Ei \'error|timeout|upstream\' | head -n 40',
    'printf "检查完成：%s\\n" "$(date +%FT%T)"',
    '',
  ].join('\n');
  const command = sample + '\n' + Array.from({ length: 120 }, (_, i) => `  printf '%s\\n' '日志 ${i}: ${'x'.repeat(160)}'`).join('\n');
  const publish = async (patch) => page.evaluate((patch) => {
    const id = window.__test.sessions[0].id;
    Object.assign(window.__test.workbenches[id], patch);
    window.__test.publishWorkbench(id);
  }, patch);
  const proposal = { id: 'review-long', command, explanation: '逐项读取服务诊断信息。', expectedEffect: '输出诊断结果，便于排查问题。', risk: 'read_only' };
  await publish({ proposal, canApprove: true });
  const dialog = page.getByRole('dialog', { name: '命令审计', exact: true });
  const code = dialog.getByLabel('完整命令', { exact: true });
  const approve = dialog.getByRole('button', { name: '批准执行', exact: true });
  await dialog.waitFor();
  assert.equal(await code.textContent(), command, 'long commands must not be truncated or rewritten');
  assert.equal(await code.locator('.command-code-line').count(), command.split('\n').length);
  for (const token of ['comment', 'string', 'variable', 'keyword', 'builtin', 'operator']) {
    const span = code.locator('.shell-token.' + token).first();
    assert.equal(await span.count(), 1, 'missing Shell highlighting for ' + token);
    assert.equal(await span.evaluate(el => getComputedStyle(el).color !== getComputedStyle(el.closest('pre')).color), true);
  }
  const firstLines = await code.locator('.command-code-line').evaluateAll(lines => lines.slice(0, 4).map(el => el.getBoundingClientRect().height));
  assert.ok(firstLines.every(height => Math.abs(height - firstLines[0]) < 1), 'line numbers and blank lines must stay aligned');
  assert.equal(await code.evaluate(el => {
    const selection = window.getSelection();
    selection.selectAllChildren(el);
    const text = selection.toString();
    selection.removeAllRanges();
    return text;
  }), command, 'selecting highlighted code must exclude line numbers and preserve whitespace');
  assert.equal(await dialog.locator('.command-risk-badge').textContent(), '只读');
  assert.equal(await code.evaluate(el => el.scrollHeight > el.clientHeight), true);
  assert.equal(await approve.evaluate(el => el === document.activeElement), false, 'opening review must not focus execution');
  await dialog.getByRole('button', { name: '复制命令', exact: true }).click();
  await dialog.getByRole('button', { name: '已复制', exact: true }).waitFor();
  assert.equal(await page.evaluate(() => window.__test.copiedCommand), command);
  await dialog.getByLabel('自动换行').uncheck();
  assert.equal(await code.evaluate(el => getComputedStyle(el).whiteSpace), 'pre');
  await dialog.getByLabel('自动换行').check();
  await page.screenshot({ path: path.join(out, 'command-review-long.png') });

  await page.keyboard.press('Escape');
  await dialog.waitFor({ state: 'detached' });
  assert.equal(await page.evaluate(() => window.__test.actions.length), 0, 'dismissal must not approve or reject');
  await publish({});
  assert.equal(await dialog.count(), 0, 'state refresh must not reopen a dismissed review');
  await page.getByRole('button', { name: '查看并审计', exact: true }).click();
  await dialog.waitFor();
  await publish({ canApprove: false, approvalStatus: '等待 Shell 空闲' });
  await dialog.getByText('等待 Shell 空闲', { exact: true }).waitFor();
  assert.equal(await approve.isEnabled(), false);
  await publish({ canApprove: true, approvalStatus: '' });
  await page.evaluate(() => {
    window.__test.originalAction = window.go.desktop.Service.WorkbenchAction;
    window.go.desktop.Service.WorkbenchAction = async () => { throw new Error('测试：确认失败'); };
  });
  await approve.click();
  await dialog.getByRole('alert').getByText('测试：确认失败', { exact: true }).waitFor();
  assert.equal(await code.textContent(), command, 'failed approval must keep the review open');
  await page.evaluate(() => { window.go.desktop.Service.WorkbenchAction = window.__test.originalAction; });

  for (const viewport of [{ width: 720, height: 520 }, { width: 390, height: 640 }]) {
    await page.setViewportSize(viewport);
    await page.emulateMedia({ colorScheme: 'dark' });
    const bounds = await dialog.boundingBox();
    const footer = await dialog.locator('.command-review-footer').boundingBox();
    assert.ok(bounds.x >= 0 && bounds.y >= 0 && bounds.x + bounds.width <= viewport.width && bounds.y + bounds.height <= viewport.height);
    assert.ok(footer.y >= bounds.y && footer.y + footer.height <= bounds.y + bounds.height, 'actions must stay visible with long commands');
    assert.equal(await dialog.evaluate(el => el.scrollWidth <= el.clientWidth), true);
  }
  await page.screenshot({ path: path.join(out, 'command-review-small-dark.png') });
  await dialog.getByRole('button', { name: '拒绝', exact: true }).click();
  await dialog.waitFor({ state: 'detached' });
  assert.equal(await page.evaluate(() => window.__test.actions.at(-1).proposalId), proposal.id);
  assert.equal(await page.getByLabel('待审批命令').count(), 0);
  await page.setViewportSize({ width: 1280, height: 840 });
  await page.emulateMedia({ colorScheme: 'light' });

  await publish({ proposal: { ...proposal, id: 'next', command: 'systemctl restart nginx', risk: ' DANGEROUS ' }, canApprove: true });
  await dialog.getByText('高风险', { exact: true }).waitFor();
  assert.equal(await dialog.evaluate(el => el.classList.contains('high')), true);
  await dialog.getByRole('button', { name: '稍后处理', exact: true }).click();
  await publish({ proposal: { ...proposal, id: 'next', command: 'uptime' } });
  await dialog.waitFor();
  assert.equal(await code.textContent(), 'uptime', 'changed commands must open a fresh review');
  const literal = "cat <<'EOF'\r\n<img src=x onerror=alert(1)>\r\n  $HOME && echo '<script>example</script>'\r\nEOF\r\n\r\nprintf '%s' 'unfinished";
  await publish({ proposal: { ...proposal, id: 'literal', command: literal } });
  await dialog.waitFor();
  assert.equal(await code.textContent(), literal, 'heredocs, CRLF, blank lines and incomplete quotes must preserve the original text');
  assert.equal(await code.locator('img, script').count(), 0, 'command text must never be parsed as HTML');
  await dialog.getByRole('button', { name: '复制命令', exact: true }).click();
  assert.equal(await page.evaluate(() => window.__test.copiedCommand), literal);
  await page.keyboard.press('Escape');

  await page.getByLabel('SSH 与 AI 会话列表').locator('.session-select').filter({ hasText: '检查磁盘空间' }).click();
  await page.getByRole('region', { name: '会话历史', exact: true }).waitFor();
  await publish({ proposal: { ...proposal, id: 'background', command: 'whoami' } });
  assert.equal(await dialog.count(), 0, 'background sessions must not display a modal');
  await page.getByLabel('SSH 与 AI 会话列表').locator('.session-select').filter({ hasText: '排查 nginx 超时' }).click();
  await dialog.waitFor();
  assert.equal(await code.textContent(), 'whoami');
  await approve.click();
  await dialog.waitFor({ state: 'detached' });
  assert.equal(await page.evaluate(() => window.__test.actions.at(-1).proposalId), 'background');
  assert.equal(await page.evaluate(() => window.__test.writes.length), 0, 'review UI must use structured approval, never terminal input');
  console.log('PASS: command review modal, full long-command text/copy/wrapping, dismissal/reopen, error and disabled states, responsive layout and session/proposal identity');
}
