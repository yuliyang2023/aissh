import assert from 'node:assert/strict';

// Run through test-desktop-ui.mjs with MODAL_ESCAPE_ONLY=1.
export async function testModalEscape(page) {
  await page.locator('.session-select').first().waitFor();
  await page.evaluate(() => {
    window.__modalEscapes = [];
    window.__escapedToWindow = 0;
    window.addEventListener('keydown', event => {
      if (event.key === 'Escape') window.__modalEscapes.push(event);
    }, true);
    window.addEventListener('keydown', event => {
      if (event.key === 'Escape') window.__escapedToWindow++;
    });
    window.go.desktop.Service.ListKnowledge = async () => [];
  });
  async function closeWithEscape(dialog) {
    const escaped = await page.evaluate(() => window.__escapedToWindow);
    await page.keyboard.press('Escape');
    await dialog.waitFor({ state: 'detached' });
    assert.equal(await page.evaluate(() => window.__modalEscapes.at(-1).defaultPrevented), true,
      'modal Escape must cancel the native window default action');
    assert.equal(await page.evaluate(() => window.__escapedToWindow), escaped,
      'modal Escape must not reach outer keyboard shortcuts');
  }

  await page.getByRole('button', { name: '设置', exact: true }).click();
  await page.getByRole('navigation', { name: '设置分类' }).getByRole('button', { name: '主机', exact: true }).click();
  await page.getByRole('button', { name: '添加主机', exact: true }).click();
  const host = page.locator('dialog[aria-label="添加主机"]');
  await host.getByLabel('主机地址', { exact: true }).fill('192.0.2.55');
  await host.getByRole('button', { name: '新建', exact: true }).click();
  const credential = page.locator('dialog[aria-label="添加凭据"]');
  await credential.getByLabel('凭据名称', { exact: true }).fill('未保存的凭据');
  await closeWithEscape(credential);
  assert.equal(await host.getByLabel('主机地址', { exact: true }).inputValue(), '192.0.2.55');
  await page.evaluate(() => document.activeElement.blur());
  await closeWithEscape(host);

  await page.getByRole('button', { name: '本地知识', exact: true }).click();
  await page.getByRole('button', { name: '新建知识', exact: true }).click();
  const editor = page.locator('dialog[aria-label="新建本地知识"]');
  await editor.getByLabel('标题', { exact: true }).fill('保留父弹框草稿');
  await editor.getByLabel(/^类型/).selectOption('data');
  const pasteButton = editor.getByRole('button', { name: '粘贴 Excel 数据', exact: true });
  await pasteButton.focus();
  await pasteButton.click();
  const paste = page.locator('dialog[aria-label="粘贴 Excel / CSV 数据"]');
  const input = paste.getByLabel(/^表格内容/);
  await input.fill('资产\t部门');
  // Escape belongs to the IME while composing, and holding it must not peel
  // away multiple modal layers after focus returns to the parent.
  await input.dispatchEvent('keydown', { key: 'Escape', isComposing: true });
  assert.equal(await paste.isVisible(), true);
  await closeWithEscape(paste);
  assert.equal(await editor.getByLabel('标题', { exact: true }).inputValue(), '保留父弹框草稿');
  await pasteButton.dispatchEvent('keydown', { key: 'Escape', repeat: true });
  assert.equal(await editor.isVisible(), true);
  assert.equal(await page.evaluate(() => window.__modalEscapes.at(-1).defaultPrevented), true);

  // Native dialog cancel remains a fallback and must not close the parent.
  await pasteButton.click();
  await paste.dispatchEvent('cancel', { bubbles: false, cancelable: true });
  await paste.waitFor({ state: 'detached' });
  assert.equal(await editor.isVisible(), true);
  await closeWithEscape(editor);
  assert.equal(await page.locator('dialog[open]').count(), 0);
  const escaped = await page.evaluate(() => window.__escapedToWindow);
  await page.keyboard.press('Escape');
  assert.equal(await page.evaluate(() => window.__escapedToWindow), escaped + 1,
    'removing the last modal must restore normal keyboard handling');
  assert.deepEqual(await page.evaluate(() => window.__test.writes), []);
  console.log('PASS: modal Escape prevents native defaults and outer shortcuts, closes only the top layer, preserves drafts, and handles focus loss/IME/repeat/cancel');
}
