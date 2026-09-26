// External operation management through a mock Wails bridge. No live systems.
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFile, mkdir } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const dist = path.resolve(root, process.env.UI_TEST_DIST || "frontend/dist");
const { chromium } = await import(
  process.env.PLAYWRIGHT_MODULE || "playwright"
);
test(
  "discover, review, grant, preview, execute, revoke and audit an external interface",
  { timeout: 90000 },
  async () => {
    const browser = await chromium.launch({
      executablePath: process.env.UI_BROWSER_PATH,
    });
    try {
      await mkdir(path.join(root, ".cache/external-systems-ui"), {
        recursive: true,
      });
      const page = await browser.newPage({
        viewport: { width: 1280, height: 900 },
      });
      page.setDefaultTimeout(12000);
      const errors = [];
      page.on("pageerror", (e) => errors.push(e.message));
      await page.route("http://external.test/**", async (route) => {
        const pathname = new URL(route.request().url()).pathname;
        const file = path.join(
          dist,
          pathname === "/" ? "index.html" : pathname,
        );
        await route.fulfill({
          body: await readFile(file),
          contentType: file.endsWith(".js")
            ? "text/javascript"
            : file.endsWith(".css")
              ? "text/css"
              : "text/html",
        });
      });
      await page.addInitScript(() => {
        const data = {
          version: 2,
          revision: 1,
          credentials: [],
          hosts: [
            {
              id: "host-a",
              name: "测试主机",
              address: "example.test",
              port: 22,
              user: "test",
              credentialId: "",
              jumpHostId: "",
              aiProfileId: "",
              workspace: "",
              group: "",
            },
          ],
          aiProfiles: [],
          resources: [],
          defaultAIProfileId: "",
        };
        const effect = {
          access: "read",
          destructive: false,
          serviceImpact: false,
          permissionChange: false,
          sensitive: false,
        };
        const report = {
          status: "ready",
          exploredAt: new Date().toISOString(),
          summary: "读取文档完成，候选接口等待审查。",
          notes: ["GET /openapi.json · HTTP 200"],
          operations: [],
        };
        const state = {
          version: 1,
          connectionId: "cmdb",
          revision: 1,
          definitionBinding: "binding",
          operations: [],
          candidates: [
            {
              id: "list-assets",
              revision: 1,
              name: "资产列表",
              kind: "http",
              description: "列出资产",
              inputSchema: {
                type: "object",
                properties: {},
                additionalProperties: false,
              },
              request: { method: "GET", path: "/assets", parameters: [] },
              assessment: {
                effects: effect,
                risk: "low",
                assurance: "inferred",
                evidence: "OpenAPI 文档",
              },
              lifecycle: "draft",
              source: "discovery",
              updatedAt: "",
            },
          ],
          history: [],
          grants: [],
          permissions: {
            enabled: true,
            ceiling: ["read"],
            deniedOperations: [],
          },
          risk: {
            denySensitive: true,
            denyDestructive: true,
            allowHigh: false,
            denyCritical: true,
            confirmRead: false,
            productionFloor: "medium",
            maxQuantity: 100,
            maxResponseBytes: 131072,
            maxRows: 100,
            timeoutSeconds: 30,
          },
          changes: [],
        };
        const invocations = [];
        const clone = (v) => structuredClone(v);
        window.externalTest = {
          saved: 0,
          explored: 0,
          executed: 0,
          permissions: 0,
          risk: 0,
          request: null,
        };
        const pending = new Map();
        const preview = (op) => {
          window.externalTest.request = clone(op);
          if (Object.keys(op.arguments).length)
            throw Error("额外参数未在接口中定义");
          const grant = state.grants.find(
            (g) =>
              !g.revoked &&
              g.operations[op.operationId] === op.operationRevision,
          );
          const decision = {
            permission: grant ? "allow" : "deny",
            risk: "low",
            action: grant ? (state.risk.confirmRead ? "ask" : "allow") : "deny",
            required: ["read"],
            reasons: [grant ? "权限通过：本机只读" : "当前主体没有有效授权"],
          };
          const p = {
            token: "preview-" + Math.random().toString(36).slice(2),
            connectionId: "cmdb",
            operation: state.operations.find((d) => d.id === op.operationId),
            stateRevision: state.revision,
            arguments: op.arguments,
            request: {
              kind: "http",
              method: "GET",
              path: "/assets",
              readOnly: true,
            },
            requestDigest: "hash-of-exact-input",
            decision,
            caller: { local: true, hostId: "", sessionId: "desktop" },
            expiresAt: new Date(Date.now() + 300000).toISOString(),
          };
          pending.set(p.token, p);
          return clone(p);
        };
        window.go = {
          desktop: {
            Service: {
              Snapshot: async () => clone(data),
              ListSessions: async () => [],
              ListConversations: async () => [],
              SaveResource: async (r) => {
                window.externalTest.saved++;
                const saved = {
                  ...r,
                  id: "cmdb",
                  systemId: "asset-system",
                  systemName: "资产管理",
                  managed: true,
                  discovery: report,
                };
                data.resources = [saved];
                return clone(saved);
              },
              GetExternalState: async () =>
                clone({
                  state,
                  job: { id: "scan", status: "ready", connectionId: "cmdb" },
                  discovery: report,
                  invocations,
                }),
              StartResourceDiscovery: async () => {
                window.externalTest.explored++;
                if (window.externalTest.explored === 1)
                  throw Error("模拟网络中断，可重新探索");
                return { id: "scan", status: "ready", connectionId: "cmdb" };
              },
              SaveExternalOperation: async (id, rev, op, reason) => {
                assertRevision(rev);
                if (!reason) throw Error("需要原因");
                op.assessment.assurance = "reviewed";
                op.revision = 1;
                state.operations = [clone(op)];
                state.candidates = [];
                state.changes.push({
                  time: new Date().toISOString(),
                  actor: "local-owner",
                  revision: ++state.revision,
                  reason,
                });
                return clone(state);
              },
              SaveExternalPermissions: async (
                id,
                rev,
                policy,
                grants,
                reason,
              ) => {
                assertRevision(rev);
                window.externalTest.permissions++;
                state.permissions = clone(policy);
                state.grants = clone(grants);
                state.changes.push({
                  time: new Date().toISOString(),
                  actor: "local-owner",
                  revision: ++state.revision,
                  reason,
                });
                return clone(state);
              },
              SaveExternalRisk: async (id, rev, risk, reason) => {
                assertRevision(rev);
                window.externalTest.risk++;
                state.risk = clone(risk);
                state.changes.push({
                  time: new Date().toISOString(),
                  actor: "local-owner",
                  revision: ++state.revision,
                  reason,
                });
                return clone(state);
              },
              PreviewExternalCall: async (op) => preview(op),
              SimulateExternalCall: async (op, caller) => ({
                ...preview(op),
                token: "",
                caller,
              }),
              InvokeExternalCall: async (token) => {
                const p = pending.get(token);
                if (!p || p.stateRevision !== state.revision)
                  throw Error("确认失效");
                pending.delete(token);
                window.externalTest.executed++;
                invocations.unshift({
                  id: "call-1",
                  time: new Date().toISOString(),
                  operationId: p.operation.id,
                  operationRevision: 1,
                  caller: "local",
                  stateRevision: state.revision,
                  requestDigest: p.requestDigest,
                  decision: p.decision,
                  approved: true,
                  outcome: "succeeded",
                  summary: "",
                });
                return JSON.stringify({
                  invocationId: "call-1",
                  outcome: "succeeded",
                  output: "资产列表已返回",
                });
              },
            },
          },
        };
        function assertRevision(rev) {
          if (rev !== state.revision) throw Error("目录已更新");
        }
      });
      await page.goto("http://external.test");
      await page.getByRole("button", { name: "外联系统", exact: true }).click();
      await page.getByRole("button", { name: "添加系统", exact: true }).click();
      await page.getByLabel("系统名称", { exact: true }).fill("CMDB");
      await page.getByLabel(/^服务地址/).fill("https://cmdb.example.com");
      await page.getByLabel(/^连接环境/).selectOption("test");
      await page
        .getByRole("button", { name: "保存并只读探索", exact: true })
        .click();
      await page.getByRole("button", { name: "操作目录", exact: true }).click();
      await page
        .getByRole("button", { name: "审查并编辑", exact: true })
        .waitFor();
      await page
        .getByRole("button", { name: "审查并编辑", exact: true })
        .click();
      await page.getByLabel(/^启用状态/).selectOption("active");
      await page
        .getByLabel("审查依据 / 变更原因", { exact: true })
        .fill("已核对资产查询接口，无业务写入");
      await page
        .getByRole("button", { name: "保存接口版本", exact: true })
        .click();
      await page.getByText("已启用", { exact: true }).waitFor();
      await page.getByRole("button", { name: "权限管理", exact: true }).click();
      await page.getByRole("button", { name: "新增授权", exact: true }).click();
      await page.getByLabel("授权名称", { exact: true }).fill("本机只读");
      await page.getByLabel(/^资产列表 v1/).check();
      await page
        .getByLabel("允许已授权的低风险只读请求自动执行", { exact: true })
        .check();
      await page
        .getByRole("button", { name: "应用到待保存规则", exact: true })
        .click();
      await page
        .getByLabel("权限变更原因", { exact: true })
        .fill("授权本机查询资产");
      await page
        .getByRole("button", { name: "保存权限规则", exact: true })
        .click();
      await page.getByRole("button", { name: "撤销", exact: true }).waitFor();
      await page.screenshot({
        path: path.join(root, ".cache/external-systems-ui/permissions.png"),
      });
      await page.getByRole("button", { name: "风险规则", exact: true }).click();
      await page.getByLabel("只读操作也逐次确认", { exact: true }).check();
      await page
        .getByLabel("风险规则变更原因", { exact: true })
        .fill("本次环境所有调用须确认");
      await page
        .getByRole("button", { name: "保存风险规则", exact: true })
        .click();
      await page
        .getByRole("button", { name: "预览与调用", exact: true })
        .click();
      await page
        .getByLabel(/^调用参数（JSON）/)
        .fill('{"extra":1}');
      await page
        .getByRole("button", { name: "预览权限与风险", exact: true })
        .click();
      await page.getByText("额外参数未在接口中定义", { exact: true }).waitFor();
      await page.getByLabel(/^调用参数（JSON）/).fill("{}");
      await page
        .getByRole("button", { name: "预览权限与风险", exact: true })
        .click();
      await page
        .getByText("需要确认本次请求 · 风险低", { exact: true })
        .waitFor();
      assert.equal(await page.evaluate(() => window.externalTest.executed), 0);
      await page.screenshot({
        path: path.join(root, ".cache/external-systems-ui/preview.png"),
      });
      await page
        .getByRole("button", { name: "确认并执行此请求", exact: true })
        .click();
      await page.getByText("执行结果", { exact: true }).waitFor();
      assert.equal(await page.evaluate(() => window.externalTest.executed), 1);
      assert.equal(
        await page
          .getByRole("button", { name: "确认并执行此请求", exact: true })
          .count(),
        0,
      );
      await page.getByLabel(/^预览主体/).selectOption("host-a");
      await page
        .getByRole("button", { name: "预览权限与风险", exact: true })
        .click();
      await page
        .getByText("需要确认本次请求 · 风险低", { exact: true })
        .waitFor();
      assert.equal(
        await page
          .getByRole("button", { name: "确认并执行此请求", exact: true })
          .count(),
        0,
      );
      await page.getByRole("button", { name: "权限管理", exact: true }).click();
      await page.getByRole("button", { name: "撤销", exact: true }).click();
      await page
        .getByLabel("权限变更原因", { exact: true })
        .fill("撤销临时读取");
      await page
        .getByRole("button", { name: "保存权限规则", exact: true })
        .click();
      await page.getByRole("button", { name: "恢复", exact: true }).waitFor();
      await page
        .getByRole("button", { name: "预览与调用", exact: true })
        .click();
      await page
        .getByRole("button", { name: "预览权限与风险", exact: true })
        .click();
      await page.getByText("拒绝执行 · 风险低", { exact: true }).waitFor();
      assert.equal(
        await page
          .getByRole("button", { name: "确认并执行此请求", exact: true })
          .count(),
        0,
      );
      await page.getByRole("button", { name: "审计记录", exact: true }).click();
      await page.getByText("成功 · 资产列表 v1", { exact: true }).waitFor();
      await page.getByText(/^v5 · 撤销临时读取/).waitFor();
      await page.getByRole("button", { name: "只读探索", exact: true }).click();
      await page
        .getByText("模拟网络中断，可重新探索", { exact: true })
        .waitFor();
      await page.getByRole("button", { name: "只读探索", exact: true }).click();
      await page.setViewportSize({ width: 940, height: 680 });
      await page.emulateMedia({ colorScheme: "dark" });
      await page.screenshot({
        path: path.join(root, ".cache/external-systems-ui/audit-dark.png"),
      });
      const modal = await page.getByRole("dialog").boundingBox();
      assert.ok(modal && modal.width <= 940 && modal.height <= 680);
      assert.deepEqual(errors, []);
      const result = await page.evaluate(() => window.externalTest);
      assert.equal(result.permissions, 2);
      assert.equal(result.risk, 1);
      assert.equal(result.explored, 2);
      assert.equal(result.request.action, "system.invoke");
    } finally {
      await browser.close();
    }
  },
);
