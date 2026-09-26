import assert from 'node:assert/strict';
import path from 'node:path';

export function installMonitoringMock(pet) {
  const monitoring = {
    policy: { enabled: true, petEnabled: false, petStyle: 'classic', confirmSeconds: 10, recoverySeconds: 10, repeatMinutes: 5, staleSeconds: 30, notifyRecovery: true, rules: [
      { id: 'collection', name: '监控采集失败', metric: 'collection', enabled: true, threshold: 0, severity: 'warning', panelId: '', keyword: '' },
      { id: 'stale', name: '监控数据过期', metric: 'stale', enabled: true, threshold: 0, severity: 'warning', panelId: '', keyword: '' },
      { id: 'offline', name: 'SSH 连接中断', metric: 'offline', enabled: true, threshold: 0, severity: 'critical', panelId: '', keyword: '' },
    ] }, monitors: [], incidents: [], notices: [], taskNotices: [], read: {}, snoozedUntil: '', liveSessions: 1, petRunning: pet, error: '',
  };
  const actions = [];
  // addInitScript order is not guaranteed; wait until the fixture bridge exists.
  const install = () => {
    window.__monitoring = monitoring;
    window.__petActions = actions;
    if (pet) {
      window.go ??= { desktop: {} };
      window.go.desktop.PetBridge = {
        Snapshot: async () => structuredClone(monitoring),
        Action: async (kind, id) => { actions.push({ kind, id }); if (kind === 'snooze') monitoring.snoozedUntil = new Date(Date.now() + 900000).toISOString(); if (kind === 'resume') monitoring.snoozedUntil = ''; if (kind === 'style') monitoring.policy.petStyle = id; },
      };
    }
    if (!window.go?.desktop.Service) { setTimeout(install, 0); return; }
    Object.assign(window.go.desktop.Service, {
      GetMonitoring: async () => structuredClone(monitoring),
      SaveAlertPolicy: async (p) => { monitoring.policy = structuredClone(p); },
      AcknowledgeAlert: async (id) => { monitoring.incidents.find(i => i.id === id).acknowledged = true; },
      SetPetEnabled: async (enabled) => { monitoring.petRunning = enabled; monitoring.policy.petEnabled = enabled; },
      SetPetStyle: async (style) => { monitoring.policy.petStyle = style; },
      SnoozeAlerts: async (minutes) => { monitoring.snoozedUntil = minutes ? new Date(Date.now() + minutes * 60000).toISOString() : ''; },
      OpenMonitorSession: async (id) => { window.__test.emit('monitor-session', id); },
      ReadPetSession: async (id, updatedAt) => {
        (window.__petReads ??= []).push({ id, updatedAt });
        monitoring.read[id] = { alert: monitoring.notices.at(-1)?.sequence || 0, task: monitoring.taskNotices.at(-1)?.sequence || 0 };
      },
    });
  };
  install();
}

const incident = (sessionId) => ({ id: 'alert-1', sessionId, sessionName: '生产应用服务器', ruleId: 'disk', title: '根目录磁盘使用率', detail: '当前 96.0%，提醒阈值 90.0%。', severity: 'critical', status: 'firing', openedAt: new Date().toISOString(), updatedAt: new Date().toISOString(), acknowledged: false });

export async function testPetSessionReads(page) {
  await page.locator('.session-select').first().waitFor();
  await page.evaluate(() => {
    window.__test.trusted = true;
    window.__readFocused = true;
    window.__readHidden = false;
    Object.defineProperty(document, 'hasFocus', { configurable: true, value: () => window.__readFocused });
    Object.defineProperty(document, 'hidden', { configurable: true, get: () => window.__readHidden });
  });
  await page.getByRole('button', { name: '连接并继续', exact: true }).click();
  await page.locator('.native-workbench:visible').waitFor();
  const first = await page.evaluate(() => window.__test.sessions[0].id);
  await page.waitForFunction(id => window.__petReads?.some(r => r.id === id), first);
  await page.locator('.session-select').filter({ hasText: '检查磁盘空间' }).click();
  await page.getByRole('button', { name: '连接并继续', exact: true }).click();
  await page.waitForFunction(() => window.__test.sessions.length === 2);
  const second = await page.evaluate(() => window.__test.sessions[1].id);
  await page.waitForFunction(id => window.__petReads?.some(r => r.id === id), second);
  const reset = () => page.evaluate(() => { window.__petReads = []; });
  const poll = () => page.waitForTimeout(1100);
  const readSessions = () => page.evaluate(() => [...new Set(window.__petReads.map(r => r.id))]);
  await reset();
  await poll();
  assert.deepEqual(await readSessions(), [second], 'background workbench must remain unread');
  await page.locator('.session-select').filter({ hasText: '排查 nginx 超时' }).click();
  await reset();
  await poll();
  assert.deepEqual(await readSessions(), [first], 'manual session navigation must read only its own notices');
  assert.equal(await page.evaluate(() => window.__petReads.at(-1).updatedAt === window.__test.workbenches[window.__petReads.at(-1).id].updatedAt), true);

  await page.getByRole('button', { name: '设置', exact: true }).click();
  await reset();
  await poll();
  assert.deepEqual(await readSessions(), [], 'settings must not consume selected session notices');
  await page.getByRole('button', { name: '返回工作区', exact: true }).click();
  await poll();
  assert.deepEqual(await readSessions(), [first]);
  await page.evaluate(() => { window.__readFocused = false; window.__petReads = []; });
  await poll();
  assert.deepEqual(await readSessions(), [], 'unfocused window must leave notices unread');
  await page.evaluate(() => { window.__readFocused = true; window.dispatchEvent(new Event('focus')); });
  assert.deepEqual(await readSessions(), [first], 'refocusing the same session reads its displayed notices');
  await page.evaluate(() => { window.__readHidden = true; window.__petReads = []; });
  await poll();
  assert.deepEqual(await readSessions(), [], 'hidden window must leave notices unread');
  await page.evaluate(() => { window.__readHidden = false; document.dispatchEvent(new Event('visibilitychange')); });
  assert.deepEqual(await readSessions(), [first]);
  const workbench = page.locator('.native-workbench:visible');
  await workbench.getByRole('button', { name: '放大监控', exact: true }).click();
  await workbench.getByRole('button', { name: '最大化监控', exact: true }).click();
  await reset();
  await poll();
  assert.deepEqual(await readSessions(), [], 'hidden AI conversation must not consume task notices');
  await workbench.getByRole('button', { name: '还原监控', exact: true }).click();
  await poll();
  assert.deepEqual(await readSessions(), [first]);
  console.log('PASS: per-session pet reads on manual navigation, foreground focus and visibility; background sessions/settings/hidden chat stay unread');
}

export async function testMonitoring(page, out) {
  await page.locator('.session-select').first().waitFor();
  await page.evaluate(() => { window.__test.trusted = true; });
  await page.getByRole('button', { name: '连接并继续', exact: true }).click();
  await page.locator('.native-workbench:visible').waitFor();
  await page.locator('.native-workbench:visible').getByRole('button', { name: 'AI 添加面板', exact: true }).click();
  await page.getByLabel('AI 实时面板需求').fill('展示 nginx 的进程数，每 5 秒刷新');
  await page.getByRole('button', { name: '让 AI 创建', exact: true }).click();
  await page.waitForFunction(() => window.__test.actions.some(a => a.kind === 'prompt' && a.text.includes('展示 nginx')));
  assert.equal(await page.evaluate(() => window.__test.actions.at(-1).text.includes('purpose=realtime、alert=null')), true);
  await page.waitForFunction(() => Object.values(window.__test.workbenches).every(w => !w.busy));
  const id = await page.evaluate(() => window.__test.sessions[0].id);
  await page.evaluate((id) => {
    const updatedAt = new Date().toISOString();
    window.__monitoring.monitors = [
      { sessionId: id, sessionName: '生产应用服务器', panelId: 'nginx', title: 'Nginx 服务', command: 'systemctl is-active nginx 2>/dev/null || true', origin: 'ai', intervalMs: 5000, updatedAt, error: '', status: 'collecting', alert: { enabled: true, kind: 'not_match', pattern: '(?m)^active\\s*$', threshold: 0, recoveryThreshold: 0, severity: 'critical', forSeconds: 10, recoverySeconds: 10 } },
      { sessionId: 'ended-fixture', sessionName: '开发环境', panelId: 'alert-availability', title: '服务可用性', command: 'systemctl is-active nginx', origin: 'ai', intervalMs: 2000, updatedAt, error: '', status: 'stopped' },
    ];
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText: async (text) => { window.__copiedMonitorCommand = text; } } });
  }, id);
  await page.locator('.sidebar').getByRole('button', { name: '告警监控', exact: true }).click();
  const inventory = page.getByLabel('告警任务清单', { exact: true });
  await inventory.getByText('2 个告警任务', { exact: true }).waitFor();
  const nginx = inventory.getByRole('article', { name: '生产应用服务器 · Nginx 服务' });
  await nginx.getByText('每 5 秒', { exact: true }).waitFor();
  await nginx.getByText('正在采集', { exact: true }).waitFor();
  assert.equal(await nginx.getByLabel('完整命令').textContent(), 'systemctl is-active nginx 2>/dev/null || true');
  await nginx.getByRole('button', { name: '复制命令' }).click();
  assert.equal(await page.evaluate(() => window.__copiedMonitorCommand), 'systemctl is-active nginx 2>/dev/null || true');
  await inventory.getByLabel('搜索告警任务').fill('2>/dev/null');
  assert.equal(await inventory.getByRole('article').count(), 1);
  await inventory.getByLabel('搜索告警任务').fill('');
  await inventory.getByLabel('筛选监控会话').selectOption('ended-fixture');
  await inventory.getByText('连接已结束', { exact: true }).waitFor();
  assert.equal(await inventory.getByRole('article').count(), 1);
  await inventory.getByLabel('筛选监控会话').selectOption('');
  await nginx.getByRole('button', { name: '立即检查' }).click();
  assert.equal(await page.evaluate(() => window.__test.actions.at(-1).kind), 'alert-refresh');
  assert.equal(await inventory.getByRole('article', { name: '生产应用服务器 · 系统' }).count(), 0);
  await page.screenshot({ path: path.join(out, 'monitor-inventory.png') });
  await page.getByRole('dialog').getByRole('button', { name: '关闭窗口', exact: true }).click();
  await page.locator('.monitoring-launch').click();
  const modal = page.getByRole('dialog', { name: '告警监控' });
  await modal.getByText('暂时没有问题记录', { exact: true }).waitFor();
  assert.equal(await modal.getByLabel('桌宠样式').inputValue(), 'classic');
  await modal.getByLabel('桌宠样式').selectOption('hacker');
  await modal.getByRole('img', { name: '黑客猫猫' }).waitFor();
  assert.equal(await page.evaluate(() => window.__monitoring.policy.petStyle), 'hacker');
  await modal.getByLabel('桌宠样式').selectOption('space');
  await modal.getByRole('img', { name: '太空猫猫' }).waitFor();
  assert.equal(await page.evaluate(() => window.__monitoring.policy.petStyle), 'space');
  await modal.getByRole('button', { name: '提醒策略', exact: true }).click();
  await modal.getByLabel('采集/断线异常持续（秒）').fill('20');
  await modal.getByRole('button', { name: '保存提醒策略' }).click();
  await modal.getByText('策略已保存', { exact: true }).waitFor();
  assert.equal(await page.evaluate(() => window.__monitoring.policy.confirmSeconds), 20);
  await modal.getByRole('button', { name: '开启桌宠' }).click();
  await modal.getByRole('button', { name: '关闭桌宠' }).waitFor();
  await page.evaluate((item) => { window.__monitoring.incidents = [item]; }, incident(id));
  await modal.getByText('根目录磁盘使用率', { exact: true }).waitFor();
  await page.screenshot({ path: path.join(out, 'monitoring-alert.png') });
  await modal.getByRole('button', { name: '我知道了' }).click();
  await modal.getByText('已知晓 · 仍异常').waitFor();
  await modal.getByRole('button', { name: '静音 15 分钟' }).click();
  await modal.getByText('提醒已暂时静音').waitFor();
  await modal.getByRole('button', { name: '恢复提醒' }).click();
  await modal.getByRole('button', { name: '查看会话' }).click();
  await modal.waitFor({ state: 'hidden' });
  await page.locator('.native-workbench:visible').waitFor();
  await page.getByRole('button', { name: /告警监控，1 个问题/ }).click();
  await modal.getByRole('button', { name: 'AI 添加告警任务', exact: true }).click();
  await modal.getByLabel('希望 AI 在什么情况下提醒').fill('监控接口响应时间，高于 500 毫秒持续 30 秒提醒');
  await modal.getByRole('button', { name: '让 AI 创建', exact: true }).click();
  await modal.waitFor({ state: 'hidden' });
  assert.equal(await page.evaluate(() => window.__test.actions.at(-1).text.includes('500 毫秒') && window.__test.actions.at(-1).text.includes('purpose=alert')), true);
  await page.getByRole('button', { name: /告警监控，1 个问题/ }).click();
  await modal.getByRole('button', { name: '提醒策略', exact: true }).click();
  await page.setViewportSize({ width: 940, height: 650 });
  await page.emulateMedia({ colorScheme: 'dark' });
  await modal.getByRole('button', { name: '保存提醒策略' }).scrollIntoViewIfNeeded();
  assert.equal(await modal.evaluate(el => el.scrollWidth <= el.clientWidth), true);
  await page.screenshot({ path: path.join(out, 'monitoring-policy-dark.png') });
  console.log('PASS: alert policy, pet toggle, incident acknowledgement, snooze, session navigation and compact dark layout');
}

export async function testPet(page, out) {
  await page.setViewportSize({ width: 320, height: 330 });
  await page.locator('.desktop-pet').waitFor();
  await page.locator('.desktop-pet.watching').waitFor();
  assert.equal(await page.evaluate(() => getComputedStyle(document.body).backgroundColor), 'rgba(0, 0, 0, 0)');
  await page.mouse.move(0, 0);
  await page.screenshot({ path: path.join(out, 'desktop-pet-idle.png'), omitBackground: true });
  await page.locator('.pet-character').hover();
  await page.waitForFunction(() => getComputedStyle(document.querySelector('.pet-tools')).opacity === '1');
  await page.getByRole('button', { name: '切换为黑客猫猫' }).click();
  await page.locator('.desktop-pet.pet-style-hacker.watching').waitFor();
  await page.getByRole('img', { name: '黑客猫猫' }).waitFor();
  assert.deepEqual(await page.evaluate(() => window.__petActions.at(-1)), { kind: 'style', id: 'hacker' });
  await page.screenshot({ path: path.join(out, 'desktop-pet-hacker-idle.png'), omitBackground: true });
  await page.getByRole('button', { name: '切换为太空猫猫' }).click();
  await page.locator('.desktop-pet.pet-style-space.watching').waitFor();
  await page.getByRole('img', { name: '太空猫猫' }).waitFor();
  assert.deepEqual(await page.evaluate(() => window.__petActions.at(-1)), { kind: 'style', id: 'space' });
  await page.screenshot({ path: path.join(out, 'desktop-pet-space-idle.png'), omitBackground: true });
  await page.getByRole('button', { name: '切换为监控橘猫' }).click();
  await page.getByRole('img', { name: '像素监控小猫' }).waitFor();
  await page.getByRole('button', { name: '切换为黑客猫猫' }).click();
  await page.locator('.desktop-pet.pet-style-hacker.watching').waitFor();
  await page.evaluate((item) => {
    window.__monitoring.incidents = [item];
    window.__monitoring.notices = [{ sequence: 1, kind: 'firing', incident: item, time: new Date().toISOString() }];
  }, incident('session-1'));
  await page.getByText('根目录磁盘使用率', { exact: true }).waitFor();
  await page.locator('.desktop-pet.critical').waitFor();
  await page.screenshot({ path: path.join(out, 'desktop-pet-alert.png'), omitBackground: true });
  await page.getByRole('button', { name: '切换为太空猫猫' }).click();
  await page.locator('.desktop-pet.pet-style-space.critical').waitFor();
  assert.equal(await page.locator('.pet-bubble-open').count(), 1, 'space appearance must preserve alerts');
  await page.screenshot({ path: path.join(out, 'desktop-pet-space-alert.png'), omitBackground: true });
  await page.getByRole('button', { name: '切换为监控橘猫' }).click();
  await page.getByRole('img', { name: '像素监控小猫' }).waitFor();
  assert.equal(await page.locator('.pet-bubble-open').count(), 1, 'changing appearance must preserve alerts');
  await page.getByRole('button', { name: '切换为黑客猫猫' }).click();
  await page.locator('.desktop-pet.pet-style-hacker.critical').waitFor();
  assert.equal(await page.locator('.pet-bubble-open').count(), 1);
  await page.locator('.pet-bubble-open').click();
  assert.deepEqual(await page.evaluate(() => window.__petActions.at(-1)), { kind: 'open', id: 'session-1' });
  await page.getByRole('button', { name: '静音 15 分钟' }).click();
  await page.getByText('安静陪伴中').waitFor();
  await page.locator('.desktop-pet.resting').waitFor();
  assert.equal(await page.locator('.pet-bubble').count(), 0);
  await page.screenshot({ path: path.join(out, 'desktop-pet-hacker-resting.png'), omitBackground: true });
  await page.getByRole('button', { name: '切换为太空猫猫' }).click();
  await page.locator('.desktop-pet.pet-style-space.resting').waitFor();
  assert.equal(await page.locator('.pet-thruster').count(), 0, 'thrusters turn off while resting');
  await page.screenshot({ path: path.join(out, 'desktop-pet-space-resting.png'), omitBackground: true });
  await page.getByRole('button', { name: '切换为监控橘猫' }).click();
  await page.getByRole('button', { name: '切换为黑客猫猫' }).click();
  await page.evaluate(() => { window.__monitoring.incidents[0].severity = 'warning'; });
  await page.getByRole('button', { name: '恢复提醒' }).click();
  await page.locator('.desktop-pet.pet-style-hacker.warning').waitFor();
  await page.screenshot({ path: path.join(out, 'desktop-pet-hacker-warning.png'), omitBackground: true });
  await page.getByRole('button', { name: '切换为太空猫猫' }).click();
  await page.locator('.desktop-pet.pet-style-space.warning').waitFor();
  assert.equal(await page.locator('.pet-thruster').count(), 1);
  await page.screenshot({ path: path.join(out, 'desktop-pet-space-warning.png'), omitBackground: true });
  await page.evaluate(() => {
    const i = window.__monitoring.incidents[0]; i.status = 'recovered';
    window.__monitoring.notices.push({ sequence: 2, kind: 'recovered', incident: i, time: new Date().toISOString() });
  });
  await page.getByText('已经恢复啦').waitFor();
  await page.emulateMedia({ colorScheme: 'dark', reducedMotion: 'reduce' });
  await page.screenshot({ path: path.join(out, 'desktop-pet-space-recovered-dark.png') });
  assert.equal(await page.locator('.pet-tail').evaluate(el => getComputedStyle(el).animationName), 'none');
  assert.equal(await page.locator('.pet-space-float').evaluate(el => getComputedStyle(el).animationName), 'none');
  assert.equal(await page.locator('.pet-thruster').evaluate(el => getComputedStyle(el).animationName), 'none');
  assert.equal(await page.evaluate(() => document.documentElement.scrollHeight <= innerHeight), true);
  await page.evaluate(() => { window.__monitoring.liveSessions = 0; });
  await page.getByText('1 个会话 · 1 条未读', { exact: true }).waitFor();
  await page.locator('.desktop-pet.resting').waitFor();
  await page.getByRole('button', { name: '收起气泡' }).click();
  await page.screenshot({ path: path.join(out, 'desktop-pet-space-resting-dark.png'), omitBackground: true });
  await page.evaluate(() => { window.__monitoring.policy.petStyle = 'classic'; });
  await page.locator('.desktop-pet.pet-style-classic.resting').waitFor();
  await page.getByRole('img', { name: '像素监控小猫' }).waitFor();
  await page.locator('.pet-status').click();
  await page.getByRole('button', { name: '全部已读', exact: true }).click();
  await testPetTasks(page, out);
  await page.locator('.pet-status').focus();
  await page.keyboard.press('Tab');
  assert.equal(await page.getByRole('button', { name: '打开告警监控' }).evaluate(el => el === document.activeElement), true);
  assert.equal(await page.locator('.pet-tools').evaluate(el => getComputedStyle(el).opacity), '1');
  await page.getByRole('button', { name: '隐藏桌宠' }).click();
  assert.equal(await page.evaluate(() => window.__petActions.at(-1).kind), 'hide');
  console.log('PASS: classic/hacker/space pet cycling, live appearance updates, all moods, resting thrusters, reduced motion, preserved alert/recovery bubbles, keyboard tools, navigation, snooze and hide');
}

async function testPetTasks(page, out) {
  await page.clock.install();
  const poll = () => page.clock.runFor(1100);
  const add = async (kinds, sessionId = 'task-session') => {
    await page.evaluate(({ kinds, sessionId }) => {
      const notices = window.__monitoring.taskNotices;
      for (const kind of kinds) notices.push({ sequence: (notices.at(-1)?.sequence || 0) + 1, sessionId, sessionName: '生产应用服务器', kind, time: new Date().toISOString() });
    }, { kinds, sessionId });
    await poll();
  };
  await add(['completed']);
  await page.getByText('任务完成啦', { exact: true }).waitFor();
  await page.locator('.desktop-pet.watching').waitFor();
  assert.equal(await page.locator('.pet-bubble.recovered').count(), 1);
  assert.match(await page.locator('.pet-bubble-open > span').textContent(), /#task-s.*生产应用服务器/);
  assert.equal(await page.evaluate(() => document.documentElement.scrollHeight <= innerHeight), true);
  await page.screenshot({ path: path.join(out, 'desktop-pet-task-completed.png'), omitBackground: true });
  await page.locator('.pet-bubble-open').click();
  assert.deepEqual(await page.evaluate(() => window.__petActions.at(-1)), { kind: 'open', id: 'task-session' });
  await poll();
  assert.equal(await page.locator('.pet-bubble').count(), 0, 'a read task must not replay on each snapshot');

  await add(['approval', 'completed']);
  await page.getByText('任务完成啦', { exact: true }).waitFor();
  assert.equal(await page.getByText('需要你确认一下', { exact: true }).count(), 0, 'completed task replaces obsolete approval');
  await page.locator('.pet-status').focus();
  await page.mouse.move(0, 0);
  await page.clock.runFor(21000);
  assert.equal(await page.locator('.pet-bubble').count(), 0, 'completion bubble collapses after 20 seconds');
  await page.getByText('1 个会话 · 1 条未读', { exact: true }).waitFor();
  await poll();
  assert.equal(await page.locator('.pet-bubble').count(), 0, 'unread entries do not reopen on polling');
  await page.locator('.pet-status').click();
  await page.getByRole('region', { name: '未读提醒列表' }).waitFor();
  assert.equal(await page.locator('.pet-inbox-item').count(), 1, 'auto-collapse preserves unread entry');
  await page.getByRole('button', { name: '收起提醒列表' }).click();

  // Same host name, three distinct sessions. Browsing must not be stolen by arrivals.
  await add(['failed'], 'failed-session');
  await page.getByText('任务遇到问题了', { exact: true }).waitFor();
  await page.locator('.desktop-pet.warning').waitFor();
  await add(['reply'], 'reply-session');
  await page.getByText('任务遇到问题了', { exact: true }).waitFor();
  await page.getByText('3 个会话 · 3 条未读', { exact: true }).waitFor();
  await page.getByRole('button', { name: '上一条提醒' }).click();
  await page.getByText('等你回复哦', { exact: true }).waitFor();
  await page.getByRole('button', { name: '下一条提醒' }).click();
  await page.getByText('任务遇到问题了', { exact: true }).waitFor();
  await page.getByRole('button', { name: '全部提醒', exact: true }).click();
  await page.clock.runFor(21000);
  assert.equal(await page.locator('.pet-inbox-item').count(), 3, 'list must stay open while browsing');
  const labels = await page.locator('.pet-inbox-item small').allTextContents();
  assert.equal(new Set(labels).size, 3, 'same-host sessions need distinct labels');
  assert.match(await page.locator('.pet-inbox-item').first().textContent(), /需要补充信息/);
  await page.screenshot({ path: path.join(out, 'desktop-pet-multiple-sessions.png'), omitBackground: true });
  assert.equal(await page.evaluate(() => document.documentElement.scrollHeight <= innerHeight), true);
  const box = await page.locator('.pet-inbox').boundingBox();
  assert.ok(box.y >= 0 && box.y + box.height <= 330, 'inbox fits the native window');
  await page.locator('.pet-inbox-item').filter({ hasText: '#failed' }).click();
  assert.deepEqual(await page.evaluate(() => window.__petActions.at(-1)), { kind: 'open', id: 'failed-session' });
  await page.getByText('2 个会话 · 2 条未读', { exact: true }).waitFor();

  await add(['completed'], 'reply-session');
  assert.equal(await page.locator('.pet-inbox-item').count(), 2, 'same-session outcome updates its unread entry');
  assert.equal(await page.getByText('AI 需要补充信息', { exact: true }).count(), 0);
  await page.getByRole('button', { name: '全部已读', exact: true }).click();
  await poll();
  assert.equal(await page.locator('.pet-bubble').count(), 0, 'clear-all must not replay on polling');
  await page.getByText('任务完成后会提醒你', { exact: true }).waitFor();

  // Visiting one workbench only consumes that session's notices, including
  // collapsed entries. A newer outcome must survive a previous read receipt.
  await add(['completed'], 'visited-session');
  await add(['reply'], 'untouched-session');
  await page.getByRole('button', { name: '收起气泡' }).click();
  await page.evaluate(() => {
    window.__monitoring.read['visited-session'] = { alert: 0, task: window.__monitoring.taskNotices.at(-1).sequence };
  });
  await poll();
  await page.getByText('1 个会话 · 1 条未读', { exact: true }).waitFor();
  assert.equal(await page.locator('.pet-bubble').count(), 0, 'reading elsewhere must not reopen a collapsed inbox');
  await page.locator('.pet-status').click();
  assert.match(await page.locator('.pet-inbox-item').textContent(), /#untouc/);
  await poll();
  assert.equal(await page.locator('.pet-inbox-item').count(), 1, 'visited notices must not replay');
  await add(['failed'], 'visited-session');
  assert.equal(await page.locator('.pet-inbox-item').count(), 2, 'new notice survives an older read receipt');
  await page.evaluate(() => {
    window.__monitoring.read['visited-session'].task = window.__monitoring.taskNotices.at(-1).sequence;
  });
  await poll();
  assert.equal(await page.locator('.pet-inbox-item').count(), 1);
  await page.getByRole('button', { name: '全部已读', exact: true }).click();

  // Concurrent outcomes in one poll must all be retained, with actual alerts distinct.
  await page.evaluate(() => {
    const state = window.__monitoring;
    state.taskNotices.push(...Array.from({ length: 8 }, (_, i) => ({ sequence: state.taskNotices.at(-1).sequence + i + 1, kind: 'completed', sessionId: `concurrent-${i}`, sessionName: '同名服务器', time: new Date().toISOString() })));
    const item = { ...state.incidents[0], status: 'firing', severity: 'critical', acknowledged: false };
    state.incidents = [item];
    state.notices.push({ sequence: 3, kind: 'firing', incident: item, time: new Date().toISOString() });
  });
  await poll();
  await page.getByText('有个问题需要你看看', { exact: true }).waitFor();
  await page.getByText('9 个会话 · 9 条未读', { exact: true }).waitFor();
  await page.getByRole('button', { name: '全部提醒', exact: true }).click();
  assert.equal(await page.locator('.pet-inbox-item').count(), 9);
  assert.match(await page.locator('.pet-inbox-item').first().textContent(), /根目录磁盘使用率/);
  assert.equal(new Set(await page.locator('.pet-inbox-item small').allTextContents()).size, 9, 'prefix collisions still get unique session labels');
  await page.evaluate(() => {
    const state = window.__monitoring;
    state.read[state.incidents[0].sessionId] = { alert: 3, task: 0 };
  });
  await poll();
  assert.equal(await page.locator('.pet-inbox-item').count(), 8, 'reading an alert session preserves all other tasks');
  assert.equal(await page.getByText('根目录磁盘使用率', { exact: true }).count(), 0);
  assert.equal(await page.evaluate(() => window.__monitoring.incidents[0].acknowledged), false, 'reading must not acknowledge the underlying incident');
  await page.locator('.pet-inbox-item').last().click();
  assert.equal(await page.locator('.pet-inbox-item').count(), 7);
  await page.getByRole('button', { name: '全部已读', exact: true }).click();
  await page.evaluate(() => { window.__monitoring.incidents = []; });
  await poll();

  await add(['approval']);
  await page.getByText('需要你确认一下', { exact: true }).waitFor();
  await page.getByRole('button', { name: '已读', exact: true }).click();
  assert.equal(await page.locator('.pet-bubble').count(), 0);
  await add(['reply']);
  await page.getByRole('button', { name: '静音 15 分钟' }).click();
  await add(['completed']);
  assert.equal(await page.locator('.pet-bubble').count(), 0, 'snoozed completion must stay quiet');
  await page.getByRole('button', { name: '恢复提醒' }).click();
  await poll();
  assert.equal(await page.locator('.pet-bubble').count(), 0, 'resuming must not replay snoozed completion');

  await page.evaluate(() => { window.__monitoring.policy.enabled = false; });
  await add(['completed']);
  await page.getByText('提醒已关闭', { exact: true }).waitFor();
  assert.equal(await page.locator('.pet-bubble').count(), 0);
  await page.evaluate(() => { window.__monitoring.policy.enabled = true; });
  await poll();
  assert.equal(await page.locator('.pet-bubble').count(), 0);
  await page.evaluate(() => {
    const notices = window.__monitoring.taskNotices;
    notices.push({ sequence: notices.at(-1).sequence + 1, kind: 'completed', sessionId: 'old', sessionName: '旧会话', time: new Date(Date.now() - 60000).toISOString() });
  });
  await poll();
  assert.equal(await page.locator('.pet-bubble').count(), 0, 'stale task must not be announced');
  console.log('PASS: multi-session unread inbox, retained collapsed notices, unique same-host labels, paging, priority, concurrent events, exact-session navigation, read/clear actions, no replay, snooze and stale notices');
}
