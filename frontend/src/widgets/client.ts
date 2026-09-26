import { validateTree, type ViewNode, type WidgetArtifact, type WidgetData } from './protocol';
let slots = 0;
const queue: (() => void)[] = [];
function acquire(): Promise<() => void> {
  return new Promise((resolve, reject) => {
    const start = () => {
      slots++;
      let released = false;
      resolve(() => {
        if (released) return;
        released = true;
        slots--;
        queue.shift()?.();
      });
    };
    if (slots < 4) start();
    else if (queue.length < 32) queue.push(start);
    else reject(Error('等待运行的组件过多'));
  });
}
// Generated code is only ever sent to the fixed Worker, as data.
export class WidgetClient {
  private worker: Worker | null = null;
  private release: (() => void) | null = null;
  private sequence = 0;
  private waiting: {
    resolve: (v: any) => void;
    reject: (e: Error) => void;
    timer: ReturnType<typeof setTimeout>;
  } | null = null;
  private stopped = false;
  private request(data: Record<string, unknown>, timeout = 1000): Promise<any> {
    if (this.stopped || this.waiting || !this.worker)
      return Promise.reject(Error('组件不可用或正在更新'));
    return new Promise((resolve, reject) => {
      const timer = setTimeout(() => this.stop('组件无响应，已停止展示'), timeout);
      this.waiting = { resolve, reject, timer };
      this.worker!.postMessage({ ...data, sequence: ++this.sequence });
    });
  }
  async init(a: WidgetArtifact) {
    if (this.stopped) throw Error('组件已关闭');
    const release = await acquire();
    if (this.stopped) {
      release();
      throw Error('组件已关闭');
    }
    this.release = release;
    try {
      this.worker = new Worker(new URL('./worker.ts', import.meta.url), { type: 'module' });
      this.worker.onmessage = (event) => {
        if (!this.waiting || event.data.sequence !== this.sequence) return;
        const w = this.waiting;
        this.waiting = null;
        clearTimeout(w.timer);
        if (event.data.error) {
          w.reject(Error(String(event.data.error).slice(0, 1000)));
          this.stop();
        } else w.resolve(event.data);
      };
      this.worker.onerror = () => this.stop('组件运行环境不可用');
      await this.request({ kind: 'init', bundle: a.bundle }, 10000);
    } catch (e) {
      this.stop();
      throw e;
    }
  }
  async render(data: WidgetData): Promise<ViewNode[]> {
    return validateTree((await this.request({ kind: 'render', data })).tree);
  }
  async event(id: string, payload: unknown): Promise<ViewNode[]> {
    return validateTree((await this.request({ kind: 'event', id, payload })).tree);
  }
  stop(message = '组件已关闭') {
    if (this.stopped) return;
    this.stopped = true;
    this.worker?.terminate();
    this.release?.();
    this.release = null;
    if (this.waiting) {
      clearTimeout(this.waiting.timer);
      this.waiting.reject(Error(message));
      this.waiting = null;
    }
  }
}
