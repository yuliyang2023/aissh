import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import path from "node:path";
const source = `import {defineWidget,MetricCard,StatusBadge,LineChart,DataTable} from '@wrd/monitor-ui';
function Cards({data}){return <div style={{display:'grid',gridTemplateColumns:'repeat(3, minmax(0, 1fr))',gap:12}}><MetricCard label="活跃连接" value={data?.connections??null} unit="个"/><MetricCard label="请求速率" value={data?.rps??null} unit="req/s"/><MetricCard label="平均响应" value={data?.latency??null} unit="ms"/></div>}
export default defineWidget({initialState:{details:false},render({data,history,state,setState}){return <section style={{display:'grid',gap:12}}><div style={{display:'flex',justifyContent:'space-between',alignItems:'center'}}><h3 style={{margin:0}}>Nginx 服务概览</h3><StatusBadge label={data?'运行中':'暂无有效数据'} tone={data?'success':'warning'}/></div><Cards data={data}/><LineChart label="连接数 · 最近 10 分钟" series={history.series('connections')} unit="个"/><button onClick={()=>setState({details:!state.details})}>{state.details?'收起明细':'展开明细'}</button>{state.details&&<DataTable columns={['service','state']} rows={[{service:'nginx',state:data?'active':'unknown'}]}/>}</section>}})`;
const compile = (source) =>
  JSON.parse(
    execFileSync(
      new URL("../.cache/widget-test-aissh", import.meta.url).pathname,
      ["--widget-compile"],
      {
        input: JSON.stringify({
          kind: "custom",
          sdkVersion: 1,
          stateVersion: 1,
          source,
        }),
        maxBuffer: 3 * 1024 * 1024,
      },
    ),
  );
export async function testWidgets(page, out) {
  const artifact = compile(source),
    bad = compile(
      `import {defineWidget} from '@wrd/monitor-ui';export default defineWidget({render(){while(true){}}})`,
    );
  await page.locator(".session-select").first().waitFor();
  await page.evaluate(() => (window.__test.trusted = true));
  await page.getByRole("button", { name: "连接并继续", exact: true }).click();
  await page.locator(".native-workbench:visible").waitFor();
  await page.evaluate(
    ({ artifact, bad }) => {
      const id = window.__test.sessions[0].id;
      window.__widgetSession = id;
      const now = Date.now();
      const data = {
        bindingId: "widget-binding",
        sourceRevision: 1,
        sequence: 1,
        data: { connections: 128, rps: 45, latency: 12 },
        lastGood: { connections: 128, rps: 45, latency: 12 },
        updatedAt: new Date(now).toISOString(),
        lastGoodAt: new Date(now).toISOString(),
        error: "",
        issues: [],
        truncated: false,
        intervalMs: 2000,
        history: Array.from({ length: 50 }, (_, i) => ({
          time: new Date(now - (49 - i) * 2000).toISOString(),
          values: {
            connections:
              i === 25 ? null : 90 + Math.round(Math.sin(i / 5) * 20) + i,
          },
        })),
      };
      window.__widgetData = data;
      window.__widgetArtifacts = { [artifact.hash]: artifact, [bad.hash]: bad };
      window.__widgetGoodHash = artifact.hash;
      window.__widgetBadHash = bad.hash;
      Object.assign(window.go.desktop.Service, {
        GetWidgetArtifact: async (_, hash) =>
          structuredClone(window.__widgetArtifacts[hash]),
        GetWidgetData: async () => structuredClone(window.__widgetData),
        GetSavedWidgets: async () => [],
        RetryWidgetSave: async () => {},
      });
      const original = window.go.desktop.Service.WorkbenchAction;
      window.go.desktop.Service.WorkbenchAction = async (id, command) => {
        await original(id, command);
        if (
          command.kind === "widget-ready" ||
          command.kind === "widget-failed"
        ) {
          const w = window.__test.workbenches[id];
          w.widgetCandidates = [];
          if (
            command.kind === "widget-ready" &&
            !w.panels.some((p) => p.id === "widget-demo")
          )
            w.panels.push({
              id: "widget-demo",
              title: "Nginx 看板",
              kind: "monitor",
              content: "connections=128\nrps=45\nlatency=12",
              status: "实时",
              error: "",
              intervalMs: 2000,
              updatedAt: new Date().toISOString(),
              pinned: false,
              widget: {
                hash: artifact.hash,
                revision: 1,
                bindingId: "widget-binding",
                sequence: 1,
              },
            });
          w.updatedAt = new Date().toISOString();
          window.__test.emit("workbench:" + id, structuredClone(w));
        }
      };
      const w = window.__test.workbenches[id];
      w.widgetCandidates = [
        {
          id: "candidate-good",
          panelId: "widget-demo",
          title: "Nginx 看板",
          hash: artifact.hash,
          data: {
            format: "kv",
            fields: ["connections", "rps", "latency"].map((name) => ({
              name,
              type: "number",
              unit: "",
              nullable: true,
              history: true,
            })),
          },
          expiresAt: new Date(now + 30000).toISOString(),
        },
      ];
      w.updatedAt = new Date().toISOString();
      window.__test.emit("workbench:" + id, structuredClone(w));
    },
    { artifact, bad },
  );
  await page.waitForFunction(
    () =>
      window.__test.actions.some(
        (a) => a.kind === "widget-ready" && a.candidateId === "candidate-good",
      ),
    { timeout: 20000 },
  );
  await page.getByRole("button", { name: "Nginx 看板", exact: true }).click();
  const host = page.getByLabel("自定义实时组件");
  await host.getByText("活跃连接", { exact: true }).waitFor();
  await host.getByText("128", { exact: false }).waitFor();
  await host.getByRole("button", { name: "展开明细" }).click();
  await host.getByRole("cell", { name: "nginx", exact: true }).waitFor();
  await page
    .locator(".native-workbench:visible")
    .getByRole("button", { name: "放大监控", exact: true })
    .click();
  await host.getByRole("button", { name: "收起明细" }).click();
  await page
    .locator(".native-workbench:visible")
    .getByRole("button", { name: "最大化监控", exact: true })
    .click();
  await page.locator(".monitor-details").evaluate((el) => (el.scrollTop = 0));
  await page.screenshot({ path: path.join(out, "custom-widget-live.png") });
  await host.getByRole("button", { name: "展开明细" }).click();
  await page.evaluate(() => {
    const id = window.__widgetSession,
      d = window.__widgetData,
      w = window.__test.workbenches[id];
    d.sequence++;
    d.data.connections = 155;
    const p = w.panels.find((p) => p.id === "widget-demo");
    p.widget.sequence = d.sequence;
    w.updatedAt = new Date().toISOString();
    window.__test.emit("workbench:" + id, structuredClone(w));
  });
  await host.getByText("155", { exact: false }).waitFor();
  await host.getByRole("cell", { name: "nginx", exact: true }).waitFor();
  await page.evaluate(() => {
    const id = window.__widgetSession,
      w = window.__test.workbenches[id];
    w.widgetCandidates = [
      {
        id: "candidate-bad",
        panelId: "widget-demo",
        title: "错误组件",
        hash: window.__widgetBadHash,
        expiresAt: new Date(Date.now() + 30000).toISOString(),
      },
    ];
    w.updatedAt = new Date().toISOString();
    window.__test.emit("workbench:" + id, structuredClone(w));
  });
  await page.waitForFunction(() =>
    window.__test.actions.some(
      (a) => a.kind === "widget-failed" && a.candidateId === "candidate-bad",
    ),
  );
  assert.equal(await host.getByText("155", { exact: false }).count(), 1);
  await page.evaluate(() => {
    const id = window.__widgetSession,
      w = window.__test.workbenches[id],
      d = window.__widgetData;
    d.sequence++;
    d.data = null;
    d.error = "字段 connections 无效";
    w.panels.find((p) => p.id === "widget-demo").widget.sequence = d.sequence;
    w.updatedAt = new Date().toISOString();
    window.__test.emit("workbench:" + id, structuredClone(w));
  });
  await host
    .getByText("数据无法使用：字段 connections 无效", { exact: true })
    .waitFor();
  assert.equal(
    await host.locator(".widget-metric strong").first().textContent(),
    "—个",
  );
  await page.emulateMedia({ colorScheme: "dark" });
  await page.setViewportSize({ width: 940, height: 650 });
  await page.screenshot({
    path: path.join(out, "custom-widget-error-dark.png"),
  });
  assert.equal(
    await host.evaluate((el) => el.scrollWidth <= el.clientWidth),
    true,
  );
  await page.getByText("查看原始输出", { exact: true }).click();
  await page.getByText("connections=128", { exact: false }).waitFor();
  console.log(
    "PASS: offline WASM, candidate preview, custom TSX, interactions, live data, retained state, failed update, missing-data UI and compact dark layout",
  );
}
