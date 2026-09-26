// UI integration test using an in-memory Wails bridge. No real credentials or
// hosts are touched. Run after frontend/build; set PLAYWRIGHT_MODULE if needed.
import assert from 'node:assert/strict';
import http from 'node:http';
import { readFile, mkdir } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const browsers = await import(process.env.PLAYWRIGHT_MODULE || 'playwright');
const engine = process.env.UI_BROWSER || 'chromium';
const server = http.createServer(async (req, res) => {
  try {
    const requested = decodeURIComponent(new URL(req.url, 'http://local').pathname);
    const file = path.join(root, 'frontend/dist', requested === '/' ? 'index.html' : requested);
    if (!file.startsWith(path.join(root, 'frontend/dist') + path.sep)) throw new Error('path');
    const body = await readFile(file);
    res.setHeader(
      'Content-Type',
      file.endsWith('.js') ? 'text/javascript' : file.endsWith('.css') ? 'text/css' : 'text/html',
    );
    res.end(body);
  } catch {
    res.statusCode = 404;
    res.end();
  }
});
await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve));
const browser = await browsers[engine].launch({
  headless: true,
  timeout: 20000,
  executablePath:
    process.env.UI_BROWSER_PATH || (engine === 'chromium' ? process.env.CHROME_PATH || '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome' : undefined),
});
const context = await browser.newContext({
  viewport: { width: 1280, height: 840 },
  deviceScaleFactor: 1,
});
const page = await context.newPage();
const errors = [];
page.on('pageerror', (e) => errors.push(e.message));
await page.addInitScript(() => {
  const data = {
    version: 1,
    revision: 1,
    hosts: [
      {
        id: 'host-one',
        name: '生产应用服务器',
        address: '192.0.2.10',
        port: 22,
        user: 'ops',
        group: '生产环境',
        credentialId: 'cred-one',
        jumpHostId: '',
        aiProfileId: '',
        workspace: '',
      },
      {
        id: 'host-two',
        name: '开发环境',
        address: '192.0.2.20',
        port: 2222,
        user: 'developer',
        group: '开发环境',
        credentialId: 'cred-one',
        jumpHostId: 'host-one',
        aiProfileId: '',
        workspace: '',
      },
    ],
    credentials: [
      {
        id: 'cred-one',
        name: '运维 SSH 密钥',
        kind: 'key',
        username: 'ops',
        identityFile: '~/.ssh/id_ed25519',
        hasSecret: true,
        updatedAt: '',
      },
    ],
    resources: [],
    aiProfiles: [],
    defaultAIProfileId: '',
    conversationHosts: JSON.parse(sessionStorage.getItem('test-conversation-hosts') || '[]'),
    recentConnections: JSON.parse(sessionStorage.getItem('recent-test-connections') || '[]'),
  };
  data.hosts.push(...JSON.parse(sessionStorage.getItem('test-direct-saved-hosts') || '[]'));
  const listeners = {};
  let sequence = 1;
  const emit = (name, value) => (listeners[name] || []).forEach((fn) => fn(value));
  window.runtime = {
    EventsOn: (name, fn) => {
      (listeners[name] ??= []).push(fn);
      return () => {
        listeners[name] = listeners[name].filter((f) => f !== fn);
      };
    },
  };
  const save = (collection, item) => {
    const value = { ...item, id: item.id || 'test-' + sequence++ };
    const i = collection.findIndex((c) => c.id === value.id);
    if (i < 0) collection.push(value);
    else collection[i] = value;
    return value;
  };
  const sessions = [];
  const conversations = JSON.parse(sessionStorage.getItem('test-ai-conversations') || 'null') || [
    { id: 'shared-id', hostId: 'host-one', title: '排查 nginx 超时', preview: '发现上游响应变慢', messageCount: 2, startedAt: '2026-09-25T08:00:00Z', updatedAt: '2026-09-25T08:10:00Z' },
    { id: 'disk', hostId: 'host-one', title: '检查磁盘空间', preview: '日志占用了较多空间', messageCount: 2, startedAt: '2026-09-24T08:00:00Z', updatedAt: '2026-09-24T08:10:00Z' },
    { id: 'shared-id', hostId: 'host-two', title: '开发服务启动失败', preview: '开发环境缺少配置', messageCount: 2, startedAt: '2026-09-23T08:00:00Z', updatedAt: '2026-09-23T08:10:00Z' },
  ];
  const persistConversations = () => sessionStorage.setItem('test-ai-conversations', JSON.stringify(conversations));
  const updateSession = (id, status) => {
    const session = sessions.find(s => s.id === id);
    if (!session) return;
    session.status = status;
    if (status === 'ended' || status === 'failed') session.endedAt = new Date().toISOString();
    emit('session-updated', { ...session });
    if (session.endedAt) emit('terminal-exit:' + id, '会话已结束');
  };
  window.__test = { data, emit, sessions, conversations, updateSession, writes: [], starts: [], trusted: false, resolutions: [], failStart: false };

  const normalizeHost = (input) => {
    const host = { ...input, address: input.address.trim(), user: input.user.trim(), port: input.port || 22 };
    const user = host.user || data.credentials.find(c => c.id === host.credentialId)?.username.trim() || '';
    const address = host.address.includes(':') ? `[${host.address}]` : host.address;
    host.name = input.name.trim() || `${user ? user + '@' : ''}${address}:${host.port}`;
    return host;
  };

  window.go = {
    desktop: {
      Service: {
        Snapshot: async () => JSON.parse(JSON.stringify(data)),
        SaveCredential: async ({ credential, secret }) =>
          save(data.credentials, { ...credential, hasSecret: !!secret || credential.hasSecret }),
        SaveHost: async (host) => {
          window.__test.lastHostInput = { ...host };
          return save(data.hosts, normalizeHost(host));
        },
        SaveResource: async (resource) => save(data.resources, resource),
        SaveAIProfile: async (profile, defaultProfile) => {
          const p = save(data.aiProfiles, profile);
          if (defaultProfile) data.defaultAIProfileId = p.id;
          return p;
        },
        Delete: async (kind, id) => {
          if (kind === 'credential' && data.hosts.some((h) => h.credentialId === id))
            throw new Error('凭据仍被主机使用');
          const k = {
            host: 'hosts',
            credential: 'credentials',
            resource: 'resources',
            ai: 'aiProfiles',
          }[kind];
          data[k] = data[k].filter((v) => v.id !== id);
        },
        PickKeyFile: async () => '/Users/example/.ssh/id_ed25519',
        PickWorkspace: async () => '/Users/example/project',
        CleanupSecrets: async () => {},
        CheckHost: async (id) => ({
          hostId: id,
          name: '生产应用服务器',
          address: '192.0.2.10',
          known: window.__test.trusted,
          token: 'trust-token',
          keys: [{ algorithm: 'ssh-ed25519', fingerprint: 'SHA256:example-test-fingerprint' }],
        }),
        TrustHost: async () => {
          window.__test.trusted = true;
        },
        TestHost: async () => {},
        ListSessions: async () => {
          const snapshot = structuredClone(sessions);
          if (window.__test.holdList) {
            window.__test.holdList = false;
            return new Promise(resolve => { window.__test.releaseList = () => resolve(snapshot); });
          }
          return snapshot;
        },
        ClearRecentConnections: async () => {
          data.recentConnections = [];
          sessionStorage.removeItem('recent-test-connections');
        },
        ListConversations: async () => structuredClone(conversations),
        GetConversation: async (hostId, id) => {
          const c = conversations.find(c => c.hostId === hostId && c.id === id);
          if (!c) throw new Error('会话不存在');
          return c.messageCount ? [{ role: 'user', content: c.title, time: c.startedAt }, { role: 'assistant', content: c.preview, time: c.updatedAt }] : [];
        },
        CreateConversation: async (hostId) => {
          if (window.__test.failConversation) throw new Error('测试：无法写入历史');
          const c = { id: 'conversation-' + sequence++, hostId, title: '新对话', preview: '', messageCount: 0, startedAt: new Date().toISOString(), updatedAt: new Date().toISOString() };
          conversations.unshift(c);
          persistConversations();
          return { ...c };
        },
        CreateDirectConversation: async (input, saveHost) => {
          const host = { ...normalizeHost(input), id: 'direct-' + Date.now() + '-' + sequence++ };
          if (saveHost) {
            data.hosts.push(host);
            sessionStorage.setItem('test-direct-saved-hosts', JSON.stringify(data.hosts.filter(h => h.id.startsWith('direct-'))));
          } else {
            data.conversationHosts.push(host);
            sessionStorage.setItem('test-conversation-hosts', JSON.stringify(data.conversationHosts));
          }
          return { host, conversation: await window.go.desktop.Service.CreateConversation(host.id) };
        },
        OpenConversationSession: async (hostId, id) => {
          const existing = sessions.find(s => s.hostId === hostId && s.conversationId === id && (s.status === 'starting' || s.status === 'running'));
          if (existing) return { ...existing };
          const created = await window.go.desktop.Service.CreateSession(hostId);
          const s = sessions.find(s => s.id === created.id);
          s.conversationId = id;
          return { ...s };
        },
        ReconnectSession: async (id) => {
          const s = sessions.find(s => s.id === id);
          return s.conversationId ? window.go.desktop.Service.OpenConversationSession(s.hostId, s.conversationId) : window.go.desktop.Service.CreateSession(s.hostId);
        },
        CreateSession: async (hostId) => {
          const host = [...data.hosts, ...data.conversationHosts].find(h => h.id === hostId);
          const session = { id: 'session-' + sequence++, hostId, conversationId: '', name: host.name,
            address: host.address, port: host.port, user: host.user, createdAt: new Date().toISOString(), endedAt: '', status: 'starting' };
          sessions.push(session);
          data.recentConnections = [{ hostId, openedAt: session.createdAt }, ...data.recentConnections.filter(r => r.hostId !== hostId)].slice(0, 50);
          sessionStorage.setItem('recent-test-connections', JSON.stringify(data.recentConnections));
          return { ...session };
        },
        StartSession: async (id) => {
          window.__test.starts.push(id);
          if (window.__test.failStart) {
            window.__test.failStart = false;
            updateSession(id, 'failed');
            throw new Error('测试：无法启动终端');
          }
          updateSession(id, 'running');
          setTimeout(() => emit('terminal:' + id, btoa('Connected to test host\r\nops@server:~$ ')), 20);
        },
        WriteTerminal: async (id, text) => {
          window.__test.writes.push(atob(text));
          emit('terminal:' + id, text);
        },
        ResizeTerminal: async () => {},
        CloseSession: async (id) => {
          updateSession(id, 'ended');
          const index = sessions.findIndex(s => s.id === id);
          if (index >= 0) sessions.splice(index, 1);
        },
        ImportSSH: async () => [],
        TestResource: async () => '{"status":"ok"}',
        RunResource: async () => '{"rows":[[1]]}',
        ResolveCredentialRequest: async (id, accepted) => {
          window.__test.resolutions.push({ id, accepted });
          emit('credential-request-ended', id);
        },
      },
    },
  };
});
const out = path.join(root, '.cache/desktop-ui');
await mkdir(out, { recursive: true });
try {
  await page.goto(`http://127.0.0.1:${server.address().port}`);
  if (process.env.DIALOG_ONLY) {
    for (const viewport of [{ width: 1180, height: 760 }, { width: 940, height: 650 }]) {
      await page.setViewportSize(viewport);
      await page.getByRole('heading', { name: 'AI 会话', exact: true }).waitFor();
      await page.locator('.topbar').getByRole('button', { name: '新建对话', exact: true }).click();
      const dialog = page.getByRole('dialog');
      await dialog.waitFor();
      await page.screenshot({ path: path.join(out, `new-dialog-${engine}-${viewport.width}-existing.png`) });
      const visibleBounds = async (locator) => locator.evaluate(el => {
        const r = el.getBoundingClientRect();
        const d = el.closest('dialog').getBoundingClientRect();
        let top = d.top, bottom = d.bottom;
        for (let parent = el.parentElement; parent && parent.tagName !== 'DIALOG'; parent = parent.parentElement) {
          if (['hidden', 'auto', 'scroll'].includes(getComputedStyle(parent).overflowY)) {
            const p = parent.getBoundingClientRect();
            top = Math.max(top, p.top); bottom = Math.min(bottom, p.bottom);
          }
        }
        return { height: r.height, inView: r.height > 0 && r.top >= top && r.bottom <= bottom };
      });
      for (const control of [dialog.getByRole('button', { name: '已有连接', exact: true }), dialog.getByRole('button', { name: '直接连接', exact: true }), dialog.getByLabel('服务器', { exact: true }), dialog.getByRole('button', { name: '创建对话', exact: true })]) {
        const bounds = await visibleBounds(control);
        assert.equal(bounds.inView, true, `${engine}: new-dialog control clipped: ${JSON.stringify(bounds)}`);
      }
      await dialog.getByRole('button', { name: '直接连接', exact: true }).click();
      await dialog.getByLabel('主机地址', { exact: true }).fill('192.0.2.88');
      await dialog.getByLabel('同时加入主机列表').check();
      await dialog.getByLabel('主机名称').fill('布局验证');
      await page.screenshot({ path: path.join(out, `new-dialog-${engine}-${viewport.width}-direct.png`) });
      assert.equal((await visibleBounds(dialog.getByRole('button', { name: '连接并新建对话', exact: true }))).inView, true);
      await dialog.getByRole('button', { name: '取消', exact: true }).click();
    }
    console.log(`PASS: ${engine} top-right new conversation controls visible and usable in both modes and window sizes`);
  } else {
  await page.getByRole('heading', { name: 'AI 会话', exact: true }).waitFor();
  await page.locator('.conversation-item').first().waitFor();
  assert.equal(await page.locator('.conversation-group').count(), 2);
  assert.equal(await page.locator('.conversation-item').count(), 3);
  assert.equal(await page.locator('.nav-item').first().innerText(), 'AI 会话\n3');
  await page.getByText('发现上游响应变慢', { exact: true }).last().waitFor();
  assert.equal(await page.evaluate(() => window.__test.starts.length), 0);
  await page.screenshot({ path: path.join(out, 'ai-conversations.png') });
  await page.getByLabel('按服务器筛选 AI 会话').selectOption('host-two');
  assert.equal(await page.locator('.conversation-item').count(), 1);
  await page.locator('.chat-content').filter({ hasText: '开发环境缺少配置' }).waitFor();
  assert.equal(await page.locator('.chat-content').filter({ hasText: '上游响应变慢' }).count(), 0);
  await page.getByLabel('按服务器筛选 AI 会话').selectOption('');
  await page.getByRole('textbox', { name: '搜索', exact: true }).fill('磁盘');
  assert.equal(await page.locator('.conversation-item').count(), 1);
  await page.getByRole('textbox', { name: '搜索', exact: true }).fill('没有这个话题');
  await page.getByRole('heading', { name: '没有匹配的对话', exact: true }).waitFor();
  await page.getByRole('button', { name: '清除搜索', exact: true }).click();
  await page.setViewportSize({ width: 940, height: 650 });
  await page.emulateMedia({ colorScheme: 'dark' });
  await page.screenshot({ path: path.join(out, 'ai-conversations-compact-dark.png') });
  assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true);
  await page.emulateMedia({ colorScheme: 'light' });
  await page.setViewportSize({ width: 1280, height: 840 });
  await page.locator('.nav-item').filter({ hasText: /^主机/ }).click();
  await page.locator('.identity strong').filter({ hasText: '生产应用服务器' }).waitFor();
  await page.screenshot({ path: path.join(out, 'hosts.png') });
  await page.getByRole('button', { name: '通过跳板', exact: true }).click();
  await page.locator('tbody tr').filter({ hasText: '开发环境' }).waitFor();
  assert.equal(await page.locator('tbody tr').count(), 1);
  await page.getByRole('textbox', { name: '搜索', exact: true }).fill('192.0.2.10');
  await page.getByText('没有找到匹配的主机', { exact: true }).waitFor();
  await page.getByRole('button', { name: '清除搜索', exact: true }).click();
  await page.getByRole('button', { name: '直接连接', exact: true }).click();
  await page.locator('tbody tr').filter({ hasText: '生产应用服务器' }).waitFor();
  assert.equal(await page.locator('tbody tr').count(), 1);
  await page.getByRole('button', { name: '全部', exact: true }).click();
  await page.getByRole('button', { name: '添加主机', exact: true }).click();
  await page.getByLabel(/^主机名称/).fill('新建测试主机');
  await page.getByLabel('主机地址', { exact: true }).fill('192.0.2.30');
  await page.getByRole('button', { name: '新建', exact: true }).click();
  await page.getByLabel('凭据名称', { exact: true }).fill('测试密码');
  await page.getByLabel(/^用户名/).fill('tester');
  const password = page.locator('dialog').last().locator('input[type=password]');
  await password.fill('ui-test-only-secret');
  assert.equal(await password.getAttribute('type'), 'password');
  await page.screenshot({ path: path.join(out, 'credential-form.png') });
  await page.getByRole('button', { name: '保存', exact: true }).click();
  await page.getByRole('button', { name: '保存主机', exact: true }).click();
  await page.getByText('新建测试主机', { exact: true }).waitFor();
  assert.equal(await page.locator('input[type=password]').count(), 0);
  assert.equal(await page.evaluate(() => localStorage.length), 0);
  // The optional name previews the current connection and can be saved empty.
  await page.setViewportSize({ width: 940, height: 650 });
  await page.getByRole('button', { name: '添加主机', exact: true }).click();
  const hostDialog = page.getByRole('dialog');
  const hostName = hostDialog.getByLabel(/^主机名称/);
  assert.equal(await hostDialog.getByLabel('主机地址', { exact: true }).evaluate(el => el === document.activeElement), true);
  assert.equal(await hostName.evaluate(el => el.required), false);
  await hostDialog.getByLabel('主机地址', { exact: true }).fill('192.0.2.31');
  assert.equal(await hostName.getAttribute('placeholder'), '192.0.2.31:22');
  await hostDialog.getByLabel(/^登录凭据/).selectOption({ label: '测试密码' });
  assert.equal(await hostName.getAttribute('placeholder'), 'tester@192.0.2.31:22');
  await hostDialog.getByLabel('SSH 用户名', { exact: true }).fill('deploy');
  await hostDialog.getByLabel('端口', { exact: true }).fill('2222');
  assert.equal(await hostName.getAttribute('placeholder'), 'deploy@192.0.2.31:2222');
  await hostDialog.getByLabel('主机地址', { exact: true }).fill('2001:db8::31');
  assert.equal(await hostName.getAttribute('placeholder'), 'deploy@[2001:db8::31]:2222');
  await hostDialog.getByLabel('主机地址', { exact: true }).fill('192.0.2.31');
  await page.screenshot({ path: path.join(out, 'host-form-optional-name.png') });
  await hostDialog.locator('summary').click();
  await hostDialog.getByLabel(/^分组（可选）/).fill('测试分组');
  assert.equal(await hostDialog.getByRole('button', { name: '保存主机', exact: true }).evaluate(el => {
    const button = el.getBoundingClientRect();
    const modal = el.closest('dialog').getBoundingClientRect();
    return button.top >= modal.top && button.bottom <= modal.bottom && button.bottom <= innerHeight;
  }), true, 'Host save action must remain visible with advanced settings expanded');
  await page.screenshot({ path: path.join(out, 'host-form-advanced.png') });
  await hostDialog.getByRole('button', { name: '保存主机', exact: true }).click();
  await page.getByText('deploy@192.0.2.31:2222', { exact: true }).waitFor();
  assert.equal(await page.evaluate(() => window.__test.lastHostInput.name), '');
  await page.setViewportSize({ width: 1280, height: 840 });
  await page.getByRole('textbox', { name: '搜索', exact: true }).fill('192.0.2.30');
  assert.equal(await page.locator('tbody tr').count(), 1);
  await page.getByRole('textbox', { name: '搜索', exact: true }).fill('');
  await page.locator('.nav-item').filter({ hasText: '凭据' }).click();
  await page.getByRole('button', { name: '删除 运维 SSH 密钥', exact: true }).click();
  await page.getByRole('button', { name: '确认删除', exact: true }).click();
  await page.getByRole('alert').getByText('凭据仍被主机使用').waitFor();
  await page.getByRole('button', { name: '取消', exact: true }).click();
  await page.screenshot({ path: path.join(out, 'credentials.png') });
  await page.locator('.nav-item').filter({ hasText: '系统连接' }).click();
  await page.getByRole('button', { name: '添加系统', exact: true }).click();
  await page.getByLabel('系统名称', { exact: true }).fill('内部状态接口');
  await page.getByLabel('服务地址').fill('https://api.example.test');
  await page.getByRole('button', { name: '保存系统连接', exact: true }).click();
  await page.getByText('内部状态接口', { exact: true }).waitFor();
  await page.screenshot({ path: path.join(out, 'resources.png') });
  await page.getByRole('button', { name: '执行', exact: true }).click();
  await page.getByRole('button', { name: '批准并执行', exact: true }).click();
  await page.locator('.result-output').waitFor();
  await page.getByRole('button', { name: '完成', exact: true }).click();
  await page.locator('.nav-item').filter({ hasText: 'AI 配置' }).click();
  await page.getByRole('button', { name: '添加 AI 配置', exact: true }).first().click();
  await page.getByLabel('配置名称', { exact: true }).fill('本机助手');
  await page.getByRole('button', { name: '保存', exact: true }).click();
  await page.getByText('本机助手', { exact: true }).waitFor();
  await page.locator('.nav-item').filter({ hasText: /^主机/ }).click();
  await page
    .locator('tbody tr')
    .filter({ has: page.locator('.identity strong').filter({ hasText: '生产应用服务器' }) })
    .getByRole('button', { name: '连接', exact: true })
    .click();
  await page.getByText('核对 SSH 主机指纹', { exact: true }).waitFor();
  await page.screenshot({ path: path.join(out, 'host-trust.png') });
  await page.getByRole('button', { name: '已核对，信任并继续', exact: true }).click();
  await page.locator('.terminal-screen').waitFor();
  await page.waitForTimeout(300);
  await page.locator('.xterm-helper-textarea').focus();
  await page.keyboard.type('ls');
  await page.keyboard.press('Enter');
  await page.waitForTimeout(100);
  assert.ok((await page.evaluate(() => window.__test.writes.join(''))).includes('ls'));
  await page.screenshot({ path: path.join(out, 'terminal.png') });
  await page.evaluate(() =>
    window.__test.emit('credential-needed', {
      requestId: 'request-1',
      credentialId: 'cred-one',
      sessionId: 'session',
    }),
  );
  await page.getByText('更新连接凭据', { exact: true }).waitFor();
  await page.locator('dialog input[type=password]').fill('updated-ui-secret');
  await page.getByRole('button', { name: '保存并继续', exact: true }).click();
  await page.waitForTimeout(100);
  assert.deepEqual(await page.evaluate(() => window.__test.resolutions), [
    { id: 'request-1', accepted: true },
  ]);
  // Session management keeps terminal instances alive across navigation and
  // recovers from both remote exit and local startup failure.
  const firstSession = await page.evaluate(() => window.__test.sessions[0].id);
  await page.locator('.nav-item').filter({ hasText: 'SSH 会话' }).click();
  await page.getByRole('heading', { name: 'SSH 会话', exact: true }).waitFor();
  await page.locator('.session-state.running').waitFor();
  await page.getByRole('button', { name: '查看终端', exact: true }).click();
  assert.equal(await page.evaluate(() => window.__test.starts.length), 1);
  await page.locator('.nav-item').filter({ hasText: 'SSH 会话' }).click();
  await page.getByRole('button', { name: '断开会话 生产应用服务器', exact: true }).click();
  await page.getByRole('button', { name: '保持连接', exact: true }).click();
  assert.equal(await page.evaluate(() => window.__test.sessions[0].status), 'running');
  await page.getByRole('button', { name: '新建连接', exact: true }).click();
  await page.locator('tbody tr').filter({ hasText: '开发环境' }).getByRole('button', { name: '连接', exact: true }).click();
  await page.waitForFunction(() => window.__test.starts.length === 2);
  await page.locator('.nav-item').filter({ hasText: 'SSH 会话' }).click();
  await page.evaluate(() => { window.__test.holdList = true; });
  await page.getByRole('button', { name: '刷新配置', exact: true }).click();
  await page.waitForFunction(() => !!window.__test.releaseList);
  await page.evaluate(id => window.__test.updateSession(id, 'ended'), firstSession);
  await page.evaluate(() => window.__test.releaseList());

  await page.locator('.session-state.ended').waitFor();
  await page.getByRole('textbox', { name: '搜索', exact: true }).fill('192.0.2.20');
  assert.equal(await page.locator('.sessions-table tbody tr').count(), 1);
  await page.getByRole('button', { name: '清除搜索', exact: true }).click();
  await page.getByLabel('会话状态', { exact: true }).selectOption('ended');
  assert.equal(await page.locator('.sessions-table tbody tr').count(), 1);
  await page.getByRole('button', { name: '重连', exact: true }).click();
  await page.waitForFunction(() => window.__test.starts.length === 3);
  assert.equal(await page.evaluate(id => window.__test.sessions.some(s => s.id === id), firstSession), false);
  await page.locator('.nav-item').filter({ hasText: 'SSH 会话' }).click();
  await page.getByLabel('会话状态', { exact: true }).selectOption('all');
  assert.equal(await page.locator('.session-state.running').count(), 2);
  await page.screenshot({ path: path.join(out, 'sessions.png') });
  await page.setViewportSize({ width: 940, height: 650 });
  await page.emulateMedia({ colorScheme: 'dark' });
  await page.screenshot({ path: path.join(out, 'sessions-compact-dark.png') });
  assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true);
  await page.emulateMedia({ colorScheme: 'light' });
  await page.setViewportSize({ width: 1280, height: 840 });
  await page.evaluate(() => window.__test.updateSession(window.__test.sessions[0].id, 'failed'));
  await page.locator('.session-state.failed').waitFor();
  await page.evaluate(() => { window.__test.failStart = true; });
  await page.getByRole('button', { name: '重连', exact: true }).click();
  await page.getByText('测试：无法启动终端', { exact: true }).waitFor();
  await page.locator('.terminal-ended').filter({ hasText: '异常结束' }).waitFor();
  const writesBefore = await page.evaluate(() => window.__test.writes.length);
  await page.locator('.terminal-pane:visible .xterm-helper-textarea').focus();
  await page.keyboard.type('should-not-send');
  assert.equal(await page.evaluate(() => window.__test.writes.length), writesBefore);
  await page.getByRole('button', { name: '重新连接', exact: true }).click();
  await page.waitForFunction(() => window.__test.starts.length === 5);
  await page.locator('.nav-item').filter({ hasText: 'SSH 会话' }).click();
  await page.getByRole('button', { name: '断开会话 开发环境', exact: true }).click();
  await page.getByRole('button', { name: '断开连接', exact: true }).click();
  await page.waitForFunction(() => window.__test.sessions.length === 1);
  await page.evaluate(() => window.__test.updateSession(window.__test.sessions[0].id, 'ended'));
  await page.getByRole('button', { name: '清理已结束', exact: true }).click();
  await page.getByText('还没有打开的会话', { exact: true }).waitFor();
  assert.equal(await page.evaluate(() => window.__test.sessions.length), 0);
  await page.getByRole('button', { name: '最近连接', exact: true }).click();
  assert.equal(await page.locator('.sessions-table tbody tr').count(), 2);
  await page.screenshot({ path: path.join(out, 'recent-connections.png') });
  await page.reload();
  await page.locator('.nav-item').filter({ hasText: 'SSH 会话' }).click();
  await page.getByRole('button', { name: '最近连接', exact: true }).click();
  await page.locator('.sessions-table tbody tr').first().waitFor();
  assert.equal(await page.locator('.sessions-table tbody tr').count(), 2);
  await page.getByRole('button', { name: '清空最近记录', exact: true }).click();
  await page.getByText('还没有最近连接', { exact: true }).waitFor();
  await page.locator('.nav-item').filter({ hasText: 'AI 会话' }).click();
  await page.getByRole('button', { name: '为 生产应用服务器 新建对话', exact: true }).click();
  await page.getByRole('dialog').waitFor();
  assert.equal(await page.getByLabel('服务器', { exact: true }).inputValue(), 'host-one');
  await page.evaluate(() => { window.__test.failConversation = true; });
  await page.getByRole('button', { name: '创建对话', exact: true }).click();
  await page.getByRole('alert').getByText('测试：无法写入历史').waitFor();
  await page.evaluate(() => { window.__test.failConversation = false; });
  await page.getByRole('button', { name: '创建对话', exact: true }).click();
  await page.locator('dialog[open]').waitFor({ state: 'detached' });
  assert.equal(await page.evaluate(() => window.__test.sessions.length), 0);
  await page.getByRole('button', { name: '开始对话', exact: true }).waitFor();
  const conversationID = await page.evaluate(() => window.__test.conversations[0].id);
  await page.getByRole('button', { name: '开始对话', exact: true }).click();
  await page.getByRole('button', { name: '已核对，信任并继续', exact: true }).click();
  await page.waitForFunction(() => window.__test.starts.length === 1);
  assert.equal(await page.evaluate(() => window.__test.sessions[0].conversationId), conversationID);
  await page.locator('.nav-item').filter({ hasText: 'AI 会话' }).click();
  await page.getByRole('button', { name: '返回对话', exact: true }).click();
  assert.equal(await page.evaluate(() => window.__test.starts.length), 1);
  await page.evaluate(() => window.__test.updateSession(window.__test.sessions[0].id, 'ended'));
  await page.getByRole('button', { name: '重新连接', exact: true }).click();
  await page.waitForFunction(() => window.__test.starts.length === 2);
  assert.equal(await page.evaluate(() => window.__test.sessions[0].conversationId), conversationID);
  // A CLI conversation switch must change the desktop association as well.
  await page.evaluate(() => {
    const s = window.__test.sessions[0];
    s.conversationId = 'disk';
    window.__test.emit('session-updated', { ...s });
  });
  await page.locator('.nav-item').filter({ hasText: 'AI 会话' }).click();
  await page.locator('.conversation-item').filter({ hasText: '检查磁盘空间' }).click();
  await page.getByRole('button', { name: '返回对话', exact: true }).click();
  assert.equal(await page.evaluate(() => window.__test.starts.length), 2);
  await page.reload();
  await page.getByRole('heading', { name: 'AI 会话', exact: true }).waitFor();
  await page.locator('.conversation-item').filter({ hasText: '新对话' }).waitFor();
  assert.equal(await page.evaluate(() => window.__test.sessions.length), 0);
  await page.evaluate(() => window.__test.emit('desktop-command', 'new-conversation'));
  await page.getByRole('heading', { name: '新建 AI 对话', exact: true }).waitFor();
  await page.getByRole('button', { name: '取消', exact: true }).click();
  await page.setViewportSize({ width: 960, height: 700 });
  await page.locator('.nav-item').filter({ hasText: /^主机/ }).click();
  await page.screenshot({ path: path.join(out, 'compact.png') });
  // Native menu actions use this same event bridge. Do not discard open sheets.
  await page.evaluate(() => window.__test.emit('desktop-command', 'new-host'));
  await page.getByLabel(/^主机名称/).waitFor();
  await page.evaluate(() => window.__test.emit('desktop-command', 'page:credentials'));
  assert.equal(await page.locator('dialog[open]').count(), 1);
  await page.getByRole('button', { name: '取消', exact: true }).click();
  await page.evaluate(() => window.__test.emit('desktop-command', 'page:credentials'));
  await page.getByRole('heading', { name: '凭据', exact: true }).waitFor();
  await page.evaluate(() => window.__test.emit('desktop-command', 'sidebar'));
  assert.equal(await page.locator('.sidebar').isVisible(), false);
  await page.getByRole('button', { name: '显示侧栏', exact: true }).click();
  assert.equal(await page.locator('.sidebar').isVisible(), true);
  await page.evaluate(() => window.__test.emit('desktop-command', 'search'));
  await page.waitForFunction(() => document.activeElement?.getAttribute('aria-label') === '搜索');
  await page.emulateMedia({ colorScheme: 'dark' });
  await page.setViewportSize({ width: 1180, height: 760 });
  await page.screenshot({ path: path.join(out, 'credentials-dark.png') });
  await page.getByRole('button', { name: '添加凭据', exact: true }).first().click();
  await page.screenshot({ path: path.join(out, 'sheet-dark.png') });
  await page.getByRole('button', { name: '取消', exact: true }).click();
  await page.locator('dialog[open]').waitFor({ state: 'detached' });
  await page.evaluate(() => window.__test.emit('desktop-command', 'page:hosts'));
  await page.getByRole('heading', { name: '主机', exact: true }).waitFor();
  await page.screenshot({ path: path.join(out, 'hosts-dark.png') });
  await page.emulateMedia({ colorScheme: 'light' });
  await page.screenshot({ path: path.join(out, 'hosts-macos.png') });
  // Exercise the shared UI without macOS titlebar insets or command-key labels.
  await page.addInitScript(() => {
    Object.defineProperty(navigator, 'platform', { value: 'Win32', configurable: true });
  });
  await page.reload();
  await page.locator('.nav-item').filter({ hasText: /^主机/ }).click();
  await page.locator('.identity strong').filter({ hasText: '生产应用服务器' }).waitFor();
  assert.equal(await page.locator('html').getAttribute('data-platform'), 'other');
  assert.equal(
    await page.getByRole('button', { name: '刷新配置' }).getAttribute('title'),
    '刷新配置 · Ctrl+R',
  );
  await page.setViewportSize({ width: 940, height: 650 });
  assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true);
  await page.screenshot({ path: path.join(out, 'hosts-other-platform.png') });
  // Direct connection can retain a conversation without adding a saved host.
  await page.locator('.nav-item').filter({ hasText: 'AI 会话' }).click();
  await page.getByRole('button', { name: '新建对话', exact: true }).first().click();
  await page.getByRole('dialog').getByRole('button', { name: '直接连接', exact: true }).click();
  await page.getByLabel('主机地址', { exact: true }).fill('192.0.2.44');
  await page.getByLabel('SSH 用户名', { exact: true }).fill('deploy');
  await page.getByLabel('端口', { exact: true }).fill('2222');
  assert.equal(await page.getByLabel('同时加入主机列表').isChecked(), false);
  await page.screenshot({ path: path.join(out, 'direct-conversation.png') });
  await page.getByRole('button', { name: '连接并新建对话', exact: true }).click();
  await page.getByText('核对 SSH 主机指纹', { exact: true }).waitFor();
  assert.equal(await page.evaluate(() => window.__test.sessions.length), 0);
  await page.getByRole('button', { name: '已核对，信任并继续', exact: true }).click();
  await page.waitForFunction(() => window.__test.sessions.some(s => s.address === '192.0.2.44' && s.status === 'running'));
  const directHost = await page.evaluate(() => window.__test.data.conversationHosts[0].id);
  assert.equal(await page.evaluate(() => window.__test.data.hosts.some(h => h.address === '192.0.2.44')), false);
  assert.equal(await page.evaluate(() => window.__test.sessions[0].port), 2222);
  assert.equal(await page.evaluate(() => window.__test.data.conversationHosts[0].name), 'deploy@192.0.2.44:2222');
  await page.evaluate(() => window.__test.updateSession(window.__test.sessions[0].id, 'ended'));
  await page.getByRole('button', { name: '重新连接', exact: true }).click();
  await page.waitForFunction(() => window.__test.starts.length === 2);
  assert.equal(await page.evaluate(() => window.__test.sessions[0].hostId), directHost);
  await page.reload();
  await page.getByLabel('按服务器筛选 AI 会话').selectOption(directHost);
  await page.getByRole('button', { name: '开始对话', exact: true }).waitFor();
  await page.locator('.nav-item').filter({ hasText: /^主机/ }).click();
  assert.equal(await page.locator('tbody tr').filter({ hasText: '192.0.2.44' }).count(), 0);
  await page.locator('.nav-item').filter({ hasText: 'AI 会话' }).click();
  await page.getByRole('button', { name: '新建对话', exact: true }).first().click();
  await page.getByRole('dialog').getByRole('button', { name: '直接连接', exact: true }).click();
  await page.getByLabel('主机地址', { exact: true }).fill('192.0.2.55');
  await page.getByLabel('同时加入主机列表').check();
  await page.getByLabel('主机名称').fill('直连保存的主机');
  assert.equal(await page.getByRole('button', { name: '连接并新建对话', exact: true }).evaluate(el => {
    const button = el.getBoundingClientRect();
    const modal = el.closest('dialog').getBoundingClientRect();
    return button.top >= modal.top && button.bottom <= modal.bottom && button.bottom <= innerHeight;
  }), true, 'Direct connection action must remain visible in a narrow window');
  await page.screenshot({ path: path.join(out, 'direct-conversation-save.png') });
  await page.getByRole('button', { name: '连接并新建对话', exact: true }).click();
  await page.getByRole('button', { name: '已核对，信任并继续', exact: true }).click();
  await page.waitForFunction(() => window.__test.sessions.some(s => s.address === '192.0.2.55' && s.status === 'running'));
  await page.locator('.nav-item').filter({ hasText: /^主机/ }).click();
  await page.locator('tbody tr').filter({ hasText: '直连保存的主机' }).waitFor();
  // First-time users can start directly with no preconfigured host.
  await page.evaluate(() => { window.__test.data.hosts = []; window.__test.data.conversationHosts = []; });
  await page.getByRole('button', { name: '刷新配置', exact: true }).click();
  await page.locator('.nav-item').filter({ hasText: 'AI 会话' }).click();
  await page.getByRole('heading', { name: '从一台服务器开始对话', exact: true }).waitFor();
  await page.getByRole('button', { name: '新建对话', exact: true }).last().click();
  await page.getByLabel('主机地址', { exact: true }).waitFor();
  assert.equal(await page.getByRole('dialog').getByRole('button', { name: '已有连接', exact: true }).isDisabled(), true);
  await page.getByRole('button', { name: '取消', exact: true }).click();
  assert.deepEqual(errors, []);
  console.log(
    'PASS: host/credential nested creation, password masking, references, combined search/route filters, resources, AI profiles, host trust, terminal input, credential repair, session switching/filtering/reconnect/close/startup failure/history, native menu actions, sidebar, light/dark appearance, non-macOS layout/shortcuts',
  );
  }
} finally {
  await browser.close();
  await new Promise((resolve) => server.close(resolve));
}
