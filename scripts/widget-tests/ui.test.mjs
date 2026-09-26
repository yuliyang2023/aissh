import { test } from "node:test";
import { spawn } from "node:child_process";
import { existsSync } from "node:fs";
import path from "node:path";
const root = new URL("../../", import.meta.url).pathname;
const projectBrowsers = path.join(root, ".cache/widget-browsers");
if (!process.env.PLAYWRIGHT_BROWSERS_PATH && existsSync(projectBrowsers))
  process.env.PLAYWRIGHT_BROWSERS_PATH = projectBrowsers;
const runtimeModule = path.resolve(
  path.dirname(process.execPath),
  "../node_modules/playwright/index.mjs",
);
const playwrightModule =
  process.env.PLAYWRIGHT_MODULE ||
  (existsSync(runtimeModule) ? runtimeModule : "playwright");
const run = (overrides) =>
  new Promise((resolve, reject) => {
    const child = spawn(process.execPath, ["scripts/test-desktop-ui.mjs"], {
      cwd: root,
      env: {
        ...process.env,
        PLAYWRIGHT_MODULE: playwrightModule,
        ...overrides,
      },
      stdio: "inherit",
    });
    const timer = setTimeout(() => {
      child.kill("SIGTERM");
      reject(Error("browser test timed out"));
    }, 90000);
    child.on("error", (e) => {
      clearTimeout(timer);
      reject(e);
    });
    child.on("exit", (code) => {
      clearTimeout(timer);
      code === 0 ? resolve() : reject(Error(`browser test exit ${code}`));
    });
  });
await test("custom widgets in Chromium", () => run({ WIDGETS_ONLY: "1" }));
await test("existing monitoring remains independent", () =>
  run({ MONITORING_ONLY: "1", WIDGETS_ONLY: "" }));
const pw = await import(playwrightModule);
let webkit = pw.webkit.executablePath();
await test(
  "custom widgets in installed WebKit",
  { skip: !existsSync(webkit) },
  () =>
    run({
      WIDGETS_ONLY: "1",
      MONITORING_ONLY: "",
      UI_BROWSER: "webkit",
      UI_BROWSER_PATH: webkit,
    }),
);
