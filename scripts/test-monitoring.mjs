import assert from 'node:assert/strict';
import path from 'node:path';

export function installMonitoringMock(pet) {
  const monitoring = {
    policy: { enabled: true, petEnabled: false, confirmSeconds: 10, recoverySeconds: 10, repeatMinutes: 5, staleSeconds: 30, notifyRecovery: true, rules: [
      { id: 'collection', name: '监控采集失败', metric: 'collection', enabled: true, threshold: 0, severity: 'warning', panelId: '', keyword: '' },
      { id: 'stale', name: '监控数据过期', metric: 'stale', enabled: true, threshold: 0, severity: 'warning', panelId: '', keyword: '' },
      { id: 'offline', name: 'SSH 连接中断', metric: 'offline', enabled: true, threshold: 0, severity: 'critical', panelId: '', keyword: '' },
    ] }, monitors: [], incidents: [], notices: [], snoozedUntil: '', liveSessions: 1, petRunning: pet, error: '',
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
        Action: async (kind, id) => { actions.push({ kind, id }); if (kind === 'snooze') monitoring.snoozedUntil = new Date(Date.now() + 900000).toISOString(); if (kind === 'resume') monitoring.snoozedUntil = ''; },
      };
    }
    if (!window.go?.desktop.Service) { setTimeout(install, 0); return; }
    Object.assign(window.go.desktop.Service, {
      GetMonitoring: async () => structuredClone(monitoring),
      SaveAlertPolicy: async (p) => { monitoring.policy = structuredClone(p); },
      AcknowledgeAlert: async (id) => { monitoring.incidents.find(i => i.id === id).acknowledged = true; },
      SetPetEnabled: async (enabled) => { monitoring.petRunning = enabled; monitoring.policy.petEnabled = enabled; },
      SnoozeAlerts: async (minutes) => { monitoring.snoozedUntil = minutes ? new Date(Date.now() + minutes * 60000).toISOString() : ''; },
      OpenMonitorSession: async (id) => { window.__test.emit('monitor-session', id); },
    });
  };
  install();
}

const incident = (sessionId) => ({ id: 'alert-1', sessionId, sessionName: '生产应用服务器', ruleId: 'disk', title: '根目录磁盘使用率', detail: '当前 96.0%，提醒阈值 90.0%。', severity: 'critical', status: 'firing', openedAt: new Date().toISOString(), updatedAt: new Date().toISOString(), acknowledged: false });

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
  await page.evaluate((item) => {
    window.__monitoring.incidents = [item];
    window.__monitoring.notices = [{ sequence: 1, kind: 'firing', incident: item, time: new Date().toISOString() }];
  }, incident('session-1'));
  await page.getByText('根目录磁盘使用率', { exact: true }).waitFor();
  await page.locator('.desktop-pet.critical').waitFor();
  await page.screenshot({ path: path.join(out, 'desktop-pet-alert.png'), omitBackground: true });
  await page.locator('.pet-bubble-open').click();
  assert.deepEqual(await page.evaluate(() => window.__petActions.at(-1)), { kind: 'open', id: 'session-1' });
  await page.getByRole('button', { name: '静音 15 分钟' }).click();
  await page.getByText('安静陪伴中').waitFor();
  await page.locator('.desktop-pet.resting').waitFor();
  assert.equal(await page.locator('.pet-bubble').count(), 0);
  await page.getByRole('button', { name: '恢复提醒' }).click();
  await page.evaluate(() => {
    const i = window.__monitoring.incidents[0]; i.status = 'recovered';
    window.__monitoring.notices.push({ sequence: 2, kind: 'recovered', incident: i, time: new Date().toISOString() });
  });
  await page.getByText('已经恢复啦').waitFor();
  await page.emulateMedia({ colorScheme: 'dark', reducedMotion: 'reduce' });
  await page.screenshot({ path: path.join(out, 'desktop-pet-recovered-dark.png') });
  assert.equal(await page.locator('.pet-tail').evaluate(el => getComputedStyle(el).animationName), 'none');
  assert.equal(await page.evaluate(() => document.documentElement.scrollHeight <= innerHeight), true);
  await page.evaluate(() => { window.__monitoring.liveSessions = 0; });
  await page.getByText('到告警监控添加任务').waitFor();
  await page.locator('.desktop-pet.resting').waitFor();
  await page.getByRole('button', { name: '收起气泡' }).click();
  await page.screenshot({ path: path.join(out, 'desktop-pet-resting.png'), omitBackground: true });
  await page.locator('.pet-status').focus();
  await page.keyboard.press('Tab');
  assert.equal(await page.getByRole('button', { name: '打开告警监控' }).evaluate(el => el === document.activeElement), true);
  assert.equal(await page.locator('.pet-tools').evaluate(el => getComputedStyle(el).opacity), '1');
  await page.getByRole('button', { name: '隐藏桌宠' }).click();
  assert.equal(await page.evaluate(() => window.__petActions.at(-1).kind), 'hide');
  console.log('PASS: pixel pet, hover/keyboard tools, alert/resting moods, reduced motion, alert/recovery bubbles, navigation, snooze and hide');
}
