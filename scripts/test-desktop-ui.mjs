// UI integration test using an in-memory Wails bridge. No real credentials or
// hosts are touched. Run after frontend/build; set PLAYWRIGHT_MODULE if needed.
import assert from 'node:assert/strict';
import http from 'node:http';
import { readFile, mkdir } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const { chromium } = await import(process.env.PLAYWRIGHT_MODULE || 'playwright');
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
const browser = await chromium.launch({
  headless: true,
  executablePath:
    process.env.CHROME_PATH || '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
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
    recentConnections: JSON.parse(sessionStorage.getItem('recent-test-connections') || '[]'),
  };
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
  const updateSession = (id, status) => {
    const session = sessions.find(s => s.id === id);
    if (!session) return;
    session.status = status;
    if (status === 'ended' || status === 'failed') session.endedAt = new Date().toISOString();
    emit('session-updated', { ...session });
    if (session.endedAt) emit('terminal-exit:' + id, '会话已结束');
  };
  window.__test = { data, emit, sessions, updateSession, writes: [], starts: [], trusted: false, resolutions: [], failStart: false };

  window.go = {
    desktop: {
      Service: {
        Snapshot: async () => JSON.parse(JSON.stringify(data)),
        SaveCredential: async ({ credential, secret }) =>
          save(data.credentials, { ...credential, hasSecret: !!secret || credential.hasSecret }),
        SaveHost: async (host) => save(data.hosts, host),
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
        CreateSession: async (hostId) => {
          const host = data.hosts.find(h => h.id === hostId);
          const session = { id: 'session-' + sequence++, hostId, name: host.name,
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
  await page.getByLabel('主机名称', { exact: true }).fill('新建测试主机');
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
  await page.getByRole('button', { name: '管理会话', exact: true }).click();
  await page.getByRole('heading', { name: '会话管理', exact: true }).waitFor();
  await page.locator('.session-state.running').waitFor();
  await page.getByRole('button', { name: '查看终端', exact: true }).click();
  assert.equal(await page.evaluate(() => window.__test.starts.length), 1);
  await page.getByRole('button', { name: '管理会话', exact: true }).click();
  await page.getByRole('button', { name: '断开会话 生产应用服务器', exact: true }).click();
  await page.getByRole('button', { name: '保持连接', exact: true }).click();
  assert.equal(await page.evaluate(() => window.__test.sessions[0].status), 'running');
  await page.getByRole('button', { name: '新建连接', exact: true }).click();
  await page.locator('tbody tr').filter({ hasText: '开发环境' }).getByRole('button', { name: '连接', exact: true }).click();
  await page.waitForFunction(() => window.__test.starts.length === 2);
  await page.getByRole('button', { name: '管理会话', exact: true }).click();
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
  await page.getByRole('button', { name: '管理会话', exact: true }).click();
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
  await page.getByRole('button', { name: '管理会话', exact: true }).click();
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
  await page.locator('.nav-item').filter({ hasText: '会话管理' }).click();
  await page.getByRole('button', { name: '最近连接', exact: true }).click();
  await page.locator('.sessions-table tbody tr').first().waitFor();
  assert.equal(await page.locator('.sessions-table tbody tr').count(), 2);
  await page.getByRole('button', { name: '清空最近记录', exact: true }).click();
  await page.getByText('还没有最近连接', { exact: true }).waitFor();
  await page.setViewportSize({ width: 960, height: 700 });
  await page.locator('.nav-item').filter({ hasText: /^主机/ }).click();
  await page.screenshot({ path: path.join(out, 'compact.png') });
  // Native menu actions use this same event bridge. Do not discard open sheets.
  await page.evaluate(() => window.__test.emit('desktop-command', 'new-host'));
  await page.getByLabel('主机名称', { exact: true }).waitFor();
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
  await page.locator('.identity strong').filter({ hasText: '生产应用服务器' }).waitFor();
  assert.equal(await page.locator('html').getAttribute('data-platform'), 'other');
  assert.equal(
    await page.getByRole('button', { name: '刷新配置' }).getAttribute('title'),
    '刷新配置 · Ctrl+R',
  );
  await page.setViewportSize({ width: 940, height: 650 });
  assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true);
  await page.screenshot({ path: path.join(out, 'hosts-other-platform.png') });
  assert.deepEqual(errors, []);
  console.log(
    'PASS: host/credential nested creation, password masking, references, combined search/route filters, resources, AI profiles, host trust, terminal input, credential repair, session switching/filtering/reconnect/close/startup failure/history, native menu actions, sidebar, light/dark appearance, non-macOS layout/shortcuts',
  );
} finally {
  await browser.close();
  await new Promise((resolve) => server.close(resolve));
}
