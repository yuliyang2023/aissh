import assert from 'node:assert/strict';
import path from 'node:path';

// Runs after the pet task tests, which install the page clock.
export async function testPetSeverity(page, out) {
  const poll = () => page.clock.runFor(1100);
  const badge = page.locator('.pet-bubble-open .pet-notice-badge');
  const color = () => badge.evaluate(el => getComputedStyle(el).color);
  const capture = async name => {
    // Flush a frame after React changes the SVG under the mocked clock.
    await page.clock.runFor(100);
    await page.screenshot({ path: path.join(out, name), omitBackground: true, animations: 'disabled' });
  };
  for (const scheme of ['light', 'dark']) {
    for (const style of ['classic', 'hacker', 'space']) {
      await page.emulateMedia({ colorScheme: scheme, reducedMotion: 'no-preference' });
      await page.evaluate(style => {
        const state = window.__monitoring;
        state.policy.petStyle = style;
        state.liveSessions = 1;
        state.read = {};
        const warning = {
          id: 'severity-warning', sessionId: 'warning-session', sessionName: '生产应用服务器',
          ruleId: 'disk', title: '磁盘空间不足', detail: '当前使用率 85%，请关注剩余空间。',
          severity: 'warning', status: 'firing', acknowledged: false,
          openedAt: new Date().toISOString(), updatedAt: new Date().toISOString(),
        };
        state.incidents = [warning];
        state.notices.push({ sequence: state.notices.at(-1).sequence + 1, kind: 'firing', incident: warning, time: new Date().toISOString() });
      }, style);
      await poll();
      await page.locator(`.desktop-pet.pet-style-${style}.warning`).waitFor();
      assert.equal(await badge.textContent(), '警告');
      assert.equal(await page.locator('.pet-emote path').count(), 1);
      const warningColor = await color();
      const warningBackground = await page.locator('.pet-bubble').evaluate(el => getComputedStyle(el).backgroundColor);
      const warningMotion = await page.locator('.pet-emote').evaluate(el => getComputedStyle(el).animationName);
      await capture(`pet-severity-${style}-${scheme}-warning.png`);

      await page.evaluate(() => {
        const state = window.__monitoring;
        const critical = { ...state.incidents[0], id: 'severity-critical', sessionId: 'critical-session', severity: 'critical', title: '数据库连接中断', detail: '连接连续失败，请立即检查服务状态。' };
        const recovered = { ...critical, id: 'severity-recovered', sessionId: 'recovered-session', status: 'recovered', title: '接口响应已恢复', detail: '响应时间已回到正常范围。' };
        state.incidents.push(critical, recovered);
        for (const item of [critical, recovered]) state.notices.push({ sequence: state.notices.at(-1).sequence + 1, kind: item.status === 'recovered' ? 'recovered' : 'firing', incident: item, time: new Date().toISOString() });
      });
      await poll();
      await page.locator('.desktop-pet.critical').waitFor();
      assert.equal(await badge.textContent(), '警告', 'arriving severe alerts must not steal the selected notice');
      assert.equal(await color(), warningColor, 'a warning must not inherit the cat’s critical color');
      assert.equal(await page.locator('.pet-bubble').evaluate(el => getComputedStyle(el).backgroundColor), warningBackground);
      assert.equal(await page.locator('.pet-emote path').count(), 2, 'critical cats have double exclamation marks');
      assert.notEqual(await page.locator('.pet-emote').evaluate(el => getComputedStyle(el).animationName), warningMotion);
      await page.getByRole('button', { name: '上一条提醒' }).click();
      assert.equal(await badge.textContent(), '严重告警');
      const criticalColor = await color();
      assert.notEqual(criticalColor, warningColor);
      assert.equal(await page.locator('.pet-bubble').evaluate(el => getComputedStyle(el).borderTopColor), criticalColor);
      await capture(`pet-severity-${style}-${scheme}-critical.png`);
      await page.getByRole('button', { name: '下一条提醒' }).click();
      await page.getByRole('button', { name: '下一条提醒' }).click();
      assert.equal(await badge.textContent(), '已恢复', 'recovery overrides the original critical severity');
      const recoveredColor = await color();
      assert.notEqual(recoveredColor, criticalColor);
      assert.notEqual(recoveredColor, warningColor);
      await capture(`pet-severity-${style}-${scheme}-recovered.png`);
      await page.getByRole('button', { name: '全部提醒', exact: true }).click();
      const rows = page.locator('.pet-inbox-item');
      assert.deepEqual(await rows.locator('.pet-notice-badge').allTextContents(), ['严重告警', '警告', '已恢复']);
      assert.deepEqual(await rows.locator('.pet-notice-badge').evaluateAll(els => els.map(el => getComputedStyle(el).color)), [criticalColor, warningColor, recoveredColor]);
      assert.equal(await rows.evaluateAll(els => els.every(el => el.scrollWidth <= el.clientWidth)), true, 'severity badges must fit beside long titles');
      const box = await page.locator('.pet-inbox').boundingBox();
      assert.ok(box.y >= 0 && box.y + box.height <= 330, 'mixed severity inbox fits the native window');
      assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true);
      await capture(`pet-severity-${style}-${scheme}-mixed.png`);
      await page.emulateMedia({ reducedMotion: 'reduce' });
      assert.equal(await page.locator('.pet-emote').evaluate(el => getComputedStyle(el).animationName), 'none');
      assert.equal(await page.locator('.pet-cat-body, .pet-space-float').evaluate(el => getComputedStyle(el).animationName), 'none');
      await page.getByRole('button', { name: '全部已读', exact: true }).click();
    }
  }
  console.log('PASS: independent warning/critical/recovery styles, mixed inbox, severity priority, all three skins, light/dark colors, critical expression and reduced motion');
}
