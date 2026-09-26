import {
  type QuickJSContext,
  type QuickJSHandle,
  type QuickJSRuntime,
  type QuickJSWASMModule,
} from 'quickjs-emscripten-core';
import { validateTree, type ViewNode, type WidgetData } from './protocol';

export class WidgetVM {
  private runtime: QuickJSRuntime;
  private vm: QuickJSContext;
  private deadline = 0;
  private interrupted = false;
  private diagnostic?: QuickJSHandle;
  constructor(module: QuickJSWASMModule, bundle: string) {
    if (bundle.length > 524288) throw Error('组件过大');
    this.runtime = module.newRuntime();
    this.runtime.setMemoryLimit(32 * 1024 * 1024);
    this.runtime.setMaxStackSize(256 * 1024);
    this.runtime.setInterruptHandler(() => {
      if (performance.now() <= this.deadline) return false;
      this.interrupted = true;
      return true;
    });
    this.vm = this.runtime.newContext();
    try {
      // Capture intrinsics before author code runs. Never stringify arbitrary
      // thrown objects or invoke their message/stack getters in the host realm.
      this.deadline = performance.now() + 200;
      this.diagnostic = this.vm.unwrapResult(
        this.vm.evalCode(
          `(() => {
        const own=Object.getOwnPropertyDescriptor, proto=Object.getPrototypeOf;
        const slice=Function.prototype.call.bind(String.prototype.slice);
        const read=(value,key)=>{for(let i=0;value&&i<3;i++,value=proto(value)){
          const d=own(value,key);if(d)return typeof d.value==='string'?slice(d.value,0,180):'';
        }return ''};
        return error=>{
          if(typeof error==='string')return slice(error,0,180);
          if(!error||(typeof error!=='object'&&typeof error!=='function'))return '';
          const name=read(error,'name'), message=read(error,'message');
          return slice((name?name+': ':'')+message,0,220);
        };
      })()`,
          'widget-diagnostic.js',
        ),
      );
      this.evaluate(bundle, 200, false, '初始化');
    } catch (e) {
      this.dispose();
      throw e;
    }
  }
  private evaluate(source: string, ms: number, output = true, phase = '渲染'): string {
    this.interrupted = false;
    this.deadline = performance.now() + ms;
    const result = this.vm.evalCode(source, 'widget.js');
    if (result.error) {
      let detail = '';
      const interrupted = this.interrupted;
      try {
        if (!interrupted && this.diagnostic) {
          this.deadline = performance.now() + 10;
          const message = this.vm.callFunction(this.diagnostic, this.vm.undefined, result.error);
          if (message.error) message.error.dispose();
          else {
            try {
              if (this.vm.typeof(message.value) === 'string')
                detail = this.vm.getString(message.value);
            } finally {
              message.value.dispose();
            }
          }
        }
      } finally {
        result.error.dispose();
      }
      if (interrupted) throw Error(`组件${phase}超时（预算 ${ms} ms），已停止执行`);
      if (/out of memory/i.test(detail)) throw Error(`组件${phase}内存不足（上限 32 MiB）`);
      detail = detail.replace(/[\u0000-\u001f\u007f]/g, ' ').slice(0, 220);
      throw Error(`组件${phase}失败：${detail || '代码抛出了无法读取详情的异常'}`);
    }
    try {
      if (!output) return '';
      if (this.vm.typeof(result.value) !== 'string') throw Error('组件必须返回有效界面');
      const value = this.vm.getString(result.value);
      if (value.length > 262144) throw Error('组件输出过大');
      return value;
    } finally {
      result.value.dispose();
    }
  }
  render(data: WidgetData): ViewNode[] {
    const raw = JSON.stringify(data);
    if (raw.length > 524288) throw Error('组件数据过大');
    return validateTree(
      JSON.parse(this.evaluate(`__widgetModule.__render(${JSON.stringify(raw)})`, 50)),
    );
  }
  event(id: string, payload: unknown): ViewNode[] {
    const raw = JSON.stringify(payload);
    if (!/^\d{1,12}$/.test(id) || raw.length > 2048) throw Error('交互无效');
    return validateTree(
      JSON.parse(
        this.evaluate(
          `__widgetModule.__event(${JSON.stringify(id)},${JSON.stringify(raw)})`,
          50,
          true,
          '交互',
        ),
      ),
    );
  }
  dispose() {
    this.diagnostic?.dispose();
    this.vm.dispose();
    this.runtime.dispose();
  }
}
