import { test } from "node:test";
import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { build } from "../../frontend/node_modules/esbuild/lib/main.js";
import { getQuickJS } from "../../frontend/node_modules/quickjs-emscripten/dist/index.mjs";
import { mkdir } from "node:fs/promises";
import { readFileSync } from "node:fs";
await mkdir(new URL("../../.cache/widget-tests/", import.meta.url), {
  recursive: true,
});
await build({
  entryPoints: [
    new URL("../../frontend/src/widgets/vm.ts", import.meta.url).pathname,
  ],
  bundle: true,
  platform: "node",
  format: "esm",
  packages: "external",
  outfile: new URL("../../.cache/widget-tests/vm.mjs", import.meta.url)
    .pathname,
});
const { WidgetVM } = await import("../../.cache/widget-tests/vm.mjs");
const QuickJS = await getQuickJS();
const data = {
  bindingId: "a",
  sourceRevision: 1,
  sequence: 1,
  data: { n: 5 },
  lastGood: { n: 5 },
  updatedAt: new Date().toISOString(),
  lastGoodAt: "",
  error: "",
  issues: [],
  history: [],
  truncated: false,
  intervalMs: 2000,
};
const compile = (source) =>
  JSON.parse(
    execFileSync(
      new URL("../../.cache/widget-test-aissh", import.meta.url).pathname,
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
  ).bundle;
test("custom TSX, state events and actual samples", () => {
  const vm = new WidgetVM(
    QuickJS,
    compile(
      `import {defineWidget,MetricCard} from '@wrd/monitor-ui';function Info({n}){return <MetricCard label="采样" value={n}/>};export default defineWidget({initialState:{count:0},render({data,state,setState}){return <section><Info n={data?.n??null}/><button onClick={()=>setState({count:state.count+1})}>点击 {state.count}</button></section>}})`,
    ),
  );
  try {
    const tree = vm.render(data);
    assert.equal(tree[0].children[0].props.value, 5);
    const id = tree[0].children[1].events.onClick;
    const next = vm.event(id, {});
    assert.equal(next[0].children[1].children.at(-1).text, "1");
    assert.equal(
      vm.render({ ...data, data: null })[0].children[0].props.value,
      null,
    );
  } finally {
    vm.dispose();
  }
});
test("VM cannot reach browser, Node or native bridges", () => {
  const vm = new WidgetVM(
    QuickJS,
    compile(
      `import {defineWidget} from '@wrd/monitor-ui';export default defineWidget({render(){return <p>{[typeof fetch,typeof window,typeof document,typeof process,typeof globalThis.require,typeof WebSocket,typeof __host].join(',')}</p>}})`,
    ),
  );
  try {
    assert.equal(
      vm.render(data)[0].children[0].text,
      Array(7).fill("undefined").join(","),
    );
  } finally {
    vm.dispose();
  }
});
test("infinite loops and allocations are interrupted", () => {
  for (const body of [
    "while(true){}",
    'const a=[];while(true)a.push(new Array(100000).fill("x"))',
  ]) {
    const bundle = compile(
      `import {defineWidget} from '@wrd/monitor-ui';${body};export default defineWidget({render(){return <p/>}})`,
    );
    const start = performance.now();
    assert.throws(() => new WidgetVM(QuickJS, bundle), /超时|内存不足/);
    // Interrupts bound JS loops. Allocation/GC teardown is additionally bounded
    // by WidgetClient's outer Worker deadline, not this synchronous VM timer.
    if (body === "while(true){}") assert.ok(performance.now() - start < 3000);
  }
  const vm = new WidgetVM(
    QuickJS,
    compile(
      `import {defineWidget} from '@wrd/monitor-ui';export default defineWidget({render(){while(true){}}})`,
    ),
  );
  try {
    assert.throws(() => vm.render(data));
  } finally {
    vm.dispose();
  }
});
test("MySQL function entry accepts empty, error and table samples", () => {
  const source = readFileSync(
    new URL("./fixtures/mysql-function.tsx", import.meta.url),
    "utf8",
  );
  const vm = new WidgetVM(QuickJS, compile(source));
  try {
    assert.match(
      JSON.stringify(vm.render({ ...data, data: null })),
      /暂无有效采集数据/,
    );
    for (const diagnostic of ["", "认证受限"]) {
      const tree = vm.render({
        ...data,
        data: {
          response: "alive",
          diagnostic,
          uptime_seconds: 120,
          threads: 3,
          slow_queries: 0,
          avg_qps: 4.2,
          services: [
            {
              unit: "mysql.service",
              state: "active",
              substate: "running",
              pid: 123,
              restarts: 0,
            },
          ],
        },
      });
      assert.match(JSON.stringify(tree), /mysql.service/);
    }
  } finally {
    vm.dispose();
  }
});
test("diagnostics distinguish initialization, render errors and actual timeouts", () => {
  assert.throws(
    () => new WidgetVM(QuickJS, compile("export default {};")),
    /初始化失败.*组件入口无效/,
  );
  const vm = new WidgetVM(
    QuickJS,
    compile("export default function Widget(){return <StatusBadge/>}"),
  );
  try {
    assert.throws(
      () => vm.render(data),
      /渲染失败.*ReferenceError.*StatusBadge/,
    );
  } finally {
    vm.dispose();
  }
  const looping = new WidgetVM(
    QuickJS,
    compile("export default function Widget(){while(true){}}"),
  );
  try {
    assert.throws(() => looping.render(data), /渲染超时.*50 ms/);
  } finally {
    looping.dispose();
  }
});
test("hex colors are not mistaken for oversized layout dimensions", () => {
  const vm = new WidgetVM(
    QuickJS,
    compile(
      `export default function Widget(){return <div style={{color:'#15803d',background:'#12345678',border:'1px solid #453099',width:120}}>ok</div>}`,
    ),
  );
  try {
    assert.equal(vm.render(data)[0].props.style.color, "#15803d");
  } finally {
    vm.dispose();
  }
  const invalid = new WidgetVM(
    QuickJS,
    compile(
      `export default function Widget(){return <div style={{width:'5000px'}}>bad</div>}`,
    ),
  );
  try {
    assert.throws(() => invalid.render(data), /width.*超过 4000/);
  } finally {
    invalid.dispose();
  }
});
test("diagnostics do not call error getters or serialize thrown data", () => {
  const vm = new WidgetVM(
    QuickJS,
    compile(
      `export default function Widget(){throw {payload:'PRIVATE-DATA',get message(){while(true){}},toJSON(){while(true){}}}}`,
    ),
  );
  try {
    assert.throws(
      () => vm.render(data),
      (e) =>
        /无法读取详情/.test(e.message) && !e.message.includes("PRIVATE-DATA"),
    );
  } finally {
    vm.dispose();
  }
});
test("renderer rejects code, URLs, CSS, bridge attrs and forged events", () => {
  for (const source of [
    "<script/>",
    "<iframe/>",
    '<div dangerouslySetInnerHTML={{__html:"x"}}/>',
    '<div style={{background:"url(https://example.com)"}}/>',
    '<div data-wails-drag="true"/>',
    "<svg><foreignObject/></svg>",
    "<button onMouseOver={()=>{}}/>",
    "<div onClick={()=>{}}/>",
  ]) {
    const vm = new WidgetVM(
      QuickJS,
      compile(
        `import {defineWidget} from '@wrd/monitor-ui';export default defineWidget({render(){return ${source}}})`,
      ),
    );
    try {
      assert.throws(() => vm.render(data), source);
    } finally {
      vm.dispose();
    }
  }
});

test("v2 native SSH capture and declarative mapping render without remote Python", () => {
 const fixture=JSON.parse(readFileSync(new URL('../../.cache/widget-v2-fixture.json',import.meta.url),'utf8'));
 assert.equal(fixture.artifact.view.sdkVersion,2);
 const vm=new WidgetVM(QuickJS,fixture.artifact.bundle);
 try {
  const tree=vm.render(fixture.sample);
  assert.equal(tree[0].children[1].props.value,2);
  assert.equal(tree[0].children[2].props.value,30);
  assert.equal(tree[0].children[4].props.rows.length,2);
  assert.equal(tree[0].children[3].props.series[0].value,30);
  const absent=vm.render({...fixture.sample,data:null,error:'SSH disconnected'});
  assert.equal(absent[0].children[1].props.value,null);
  const failed=structuredClone(fixture.sample);
  failed.data.memory.value=null;failed.data.memory.quality='unknown';
  assert.equal(vm.render(failed)[0].children[2].props.value,null);
 } finally {vm.dispose()}
});
