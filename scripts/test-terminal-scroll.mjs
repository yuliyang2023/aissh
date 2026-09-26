// Browser regression checks using the desktop UI harness's in-memory SSH bridge.
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import path from 'node:path';

export async function testTerminalScroll(page, out, engine) {
  // Poll from the harness: older WebKit drivers implement waitForFunction with
  // eval, which the application's CSP correctly disallows.
  const waitFor = async (predicate, argument) => {
    const deadline = Date.now() + 10000;
    while (Date.now() < deadline) {
      if (await page.evaluate(predicate, argument)) return;
      await new Promise((resolve) => setTimeout(resolve, 50));
    }
    assert.fail(`Timed out waiting for ${predicate}`);
  };
  await page.evaluate(() => {
    window.__test.trusted = true;
  });
  await page.getByRole('button', { name: '连接并继续', exact: true }).click();
  const pane = page.locator('.terminal-pane:visible');
  const viewport = pane.locator('.xterm-viewport');
  const bottom = pane.getByRole('button', { name: '回到底部', exact: true });
  await viewport.waitFor();
  await waitFor(() => window.__test.starts.length === 1);
  const session = await page.evaluate(() => window.__test.sessions[0].id);
  const emit = (text) =>
    page.evaluate(
      ({ session, text }) => {
        window.__test.emit('terminal:' + session, btoa(text));
      },
      { session, text },
    );
  const metrics = () =>
    viewport.evaluate((el) => ({
      top: el.scrollTop,
      height: el.clientHeight,
      total: el.scrollHeight,
      // Native scrolling can stop between rows; xterm snaps to the nearest row
      // when it next renders. Compare the visible terminal row, not raw pixels.
      row: Math.round(
        el.scrollTop /
          el.parentElement.querySelector('.xterm-rows > div').getBoundingClientRect().height,
      ),
    }));
  const append = async (prefix, count) => {
    const before = await metrics();
    await emit(Array.from({ length: count }, (_, i) => `\r\n${prefix} ${i}`).join(''));
    await waitFor((total) => {
      const el = [...document.querySelectorAll('.xterm-viewport')].find((el) => el.clientHeight);
      return el?.scrollHeight > total;
    }, before.total);
  };
  const waitForBottom = async () => {
    await waitFor(() => {
      const el = [...document.querySelectorAll('.xterm-viewport')].find((el) => el.clientHeight);
      return el && Math.abs(el.scrollHeight - el.clientHeight - el.scrollTop) <= 2;
    });
    await waitFor(
      () =>
        document.querySelector('.terminal-pane:not([style*="none"]) .terminal-scroll-bottom')
          ?.disabled,
    );
  };
  const waitForHistory = async () => {
    await pane.getByText('浏览历史', { exact: true }).waitFor();
    // Browser scroll events and xterm's viewport synchronization finish on
    // separate animation frames after the last pointer/wheel event.
    await viewport.evaluate(
      (el) =>
        new Promise((resolve) => {
          let previous = el.scrollTop;
          let stableFrames = 0;
          const check = () => {
            stableFrames = el.scrollTop === previous ? stableFrames + 1 : 0;
            previous = el.scrollTop;
            if (stableFrames >= 3) resolve();
            else requestAnimationFrame(check);
          };
          requestAnimationFrame(check);
        }),
    );
  };

  if (process.env.TERMINAL_TRANSCRIPT) {
    // Include the backend's real startup/output/teardown, not just raw mock text.
    await emit(await readFile(process.env.TERMINAL_TRANSCRIPT, 'utf8'));
    await waitFor(() => {
      const el = [...document.querySelectorAll('.xterm-viewport')].find((el) => el.clientHeight);
      return el && el.scrollHeight > el.clientHeight * 3;
    });
  } else {
    await append('initial output', 400);
  }
  await waitForBottom();
  await append('following output', 80);
  await waitForBottom();

  // Real wheel input must pause following before the next output arrives.
  await pane.locator('.xterm-screen').hover();
  await page.mouse.wheel(0, -600);
  await waitForHistory();
  const reading = await metrics();
  await append('output while reading', 120);
  const afterOutput = await metrics();
  assert.equal(afterOutput.row, reading.row, 'new output moved the reading position');
  assert.equal(await bottom.isEnabled(), true);
  await page.screenshot({
    path: path.join(out, `terminal-scroll-${engine}-history.png`),
  });

  const writes = await page.evaluate(() => window.__test.writes.length);
  await bottom.click();
  await waitForBottom();
  assert.equal(
    await pane.locator('.xterm-helper-textarea').evaluate((el) => el === document.activeElement),
    true,
  );
  assert.equal(
    await page.evaluate(() => window.__test.writes.length),
    writes,
    'scroll button sent SSH input',
  );
  await append('resumed output', 60);
  await waitForBottom();

  // Drag the native scrollbar rather than relying only on wheel events.
  const box = await viewport.boundingBox();
  await page.mouse.move(box.x + box.width - 6, box.y + box.height - 12);
  await page.mouse.down();
  await page.mouse.move(box.x + box.width - 6, box.y + 70, { steps: 10 });
  await page.mouse.up();
  await waitForHistory();
  const dragged = await metrics();
  await append('output after dragging', 60);
  assert.equal((await metrics()).row, dragged.row, 'dragged viewport jumped on output');

  // A hidden session keeps its own position while the bridge continues writing.
  await page
    .getByLabel('SSH 与 AI 会话列表')
    .getByRole('button', { name: /检查磁盘空间/ })
    .click();
  await emit('\r\nbackground output one\r\nbackground output two');
  await page
    .getByLabel('SSH 与 AI 会话列表')
    .getByRole('button', { name: /^排查 nginx 超时/ })
    .click();
  await waitForHistory();
  assert.equal((await metrics()).row, dragged.row, 'switching sessions lost the reading position');

  // Resize and reflow must leave history visible and keep controls inside the pane.
  await page.setViewportSize({ width: 940, height: 650 });
  await waitForHistory();
  await waitFor(() => {
    const screen = document.querySelector('.terminal-pane:not([style*="none"]) .xterm-screen');
    const body = screen?.closest('.terminal-body');
    return (
      screen && body && screen.getBoundingClientRect().bottom <= body.getBoundingClientRect().bottom
    );
  });
  assert.equal(
    await bottom.evaluate((el) => {
      const parent = el.closest('.terminal-pane').getBoundingClientRect();
      const rect = el.getBoundingClientRect();
      return rect.left >= parent.left && rect.right <= parent.right;
    }),
    true,
    'scroll controls clipped after resize',
  );
  await page.screenshot({
    path: path.join(out, `terminal-scroll-${engine}-compact.png`),
  });
  await bottom.click();
  await waitForBottom();

  // Returning manually to the bottom also resumes follow mode.
  await pane.locator('.xterm-screen').hover();
  await page.mouse.wheel(0, -300);
  await waitForHistory();
  await viewport.evaluate((el) => {
    el.scrollTop = el.scrollHeight;
  });
  await waitForBottom();
  await append('manually resumed output', 30);
  await waitForBottom();

  // Full-screen programs have no scrollback; their mouse input stays with xterm.
  await emit('\x1b[?1049h\x1b[2J\x1b[Hfull screen application');
  await waitFor(() => {
    const el = [...document.querySelectorAll('.xterm-viewport')].find((el) => el.clientHeight);
    return el && el.scrollHeight <= el.clientHeight + 2;
  });
  assert.equal(await bottom.isDisabled(), true);
  const beforeMouse = await page.evaluate(() => window.__test.writes.length);
  await emit('\x1b[?1000h');
  await pane.locator('.xterm-screen').click({ position: { x: 30, y: 30 } });
  await waitFor(
    (start) => window.__test.writes.slice(start).some((data) => data.startsWith('\x1b[M')),
    beforeMouse,
  );
  await emit('\x1b[?1000l\x1b[?1049l');
  await waitForBottom();

  // Ended sessions retain readable history and local scroll controls.
  await page.evaluate((id) => window.__test.updateSession(id, 'ended'), session);
  await pane.getByText('会话已结束，终端内容保留至关闭标签页。', { exact: true }).waitFor();
  await pane.locator('.xterm-screen').hover();
  await page.mouse.wheel(0, -300);
  await waitForHistory();
  await bottom.click();
  await waitForBottom();
  assert.equal(await page.evaluate(() => window.__test.starts.length), 1);
  console.log(
    `PASS: ${engine} terminal follow, wheel/drag, history stability, return-to-bottom focus, session switching, resize, remote mouse, alternate screen and ended session`,
  );
}
