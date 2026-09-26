import assert from 'node:assert/strict';

export async function testIMEComposer(input, submissionCount) {
  const draft = '中文选词';
  await input.fill(draft);
  const before = await submissionCount();
  const composition = type => input.dispatchEvent(type, { data: '词' });
  async function confirm(options, label) {
    const prevented = await input.evaluate((el, options) => {
      const event = new KeyboardEvent('keydown', {
        key: 'Enter', code: 'Enter', keyCode: 13, bubbles: true, cancelable: true,
        ...options,
      });
      el.dispatchEvent(event);
      return event.defaultPrevented;
    }, options);
    assert.equal(await submissionCount(), before, label + ': must not send');
    assert.equal(await input.inputValue(), draft, label + ': must preserve draft');
    assert.equal(prevented, false, label + ': must allow native IME confirmation');
    await input.dispatchEvent('keyup', { key: 'Enter', code: 'Enter', keyCode: 13 });
  }

  await confirm({ isComposing: true }, 'Native composition flag');
  await composition('compositionstart');
  await confirm({ isComposing: false }, 'Active composition without native flag');
  await composition('compositionend');

  // WebKit can end composition before dispatching the confirmation keydown.
  await composition('compositionstart');
  await composition('compositionend');
  await confirm({ isComposing: false, keyCode: 229 }, 'Composition ended before Enter');
  await confirm({ isComposing: false, keyCode: 229 }, 'IME key code without composition events');

  await input.press('End');
  await input.press('Shift+Enter');
  assert.equal(await submissionCount(), before, 'Shift+Enter must not send');
  assert.equal(await input.inputValue(), draft + '\n', 'Shift+Enter must insert a newline');

  // Losing focus may cancel composition without compositionend.
  await composition('compositionstart');
  await input.evaluate(el => el.blur());
  await input.focus();
}

export async function testWorkbenchIME(page) {
  const workbench = page.locator('.native-workbench:visible');
  const input = workbench.getByLabel('向 AI 提问');
  const submissionCount = () => page.evaluate(() => window.__test.actions.filter(action => action.kind === 'prompt').length);
  const before = await submissionCount();
  const beforeWrites = await page.evaluate(() => window.__test.writes.length);
  await testIMEComposer(input, submissionCount);
  await input.fill('确认选词后发送');
  await input.press('Enter');
  await workbench.getByText('检查完成，服务运行正常。', { exact: true }).waitFor();
  assert.equal(await submissionCount(), before + 1, 'Enter sends exactly once after composition');
  assert.equal(await input.inputValue(), '', 'Successful send clears the draft');
  assert.equal(await page.evaluate(() => window.__test.actions.at(-1).text), '确认选词后发送');

  await input.fill('点击发送');
  await workbench.getByRole('button', { name: '发送', exact: true }).click();
  assert.equal(await submissionCount(), before + 2, 'Send button still works');
  assert.equal(await page.evaluate(() => window.__test.actions.at(-1).text), '点击发送');
  assert.equal(await page.evaluate(() => window.__test.writes.length), beforeWrites, 'Chat input stays out of the SSH terminal');
}
