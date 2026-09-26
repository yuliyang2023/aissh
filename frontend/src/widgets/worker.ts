/// <reference lib="webworker" />
import { newQuickJSWASMModuleFromVariant, newVariant } from 'quickjs-emscripten-core';
import RELEASE_SYNC from '@jitl/quickjs-wasmfile-release-sync';
import wasmURL from '@jitl/quickjs-wasmfile-release-sync/wasm?url';
import { WidgetVM } from './vm';
let vm: WidgetVM | null = null;
let busy = false;
self.onmessage = async (event: MessageEvent) => {
  if (busy) return;
  busy = true;
  const m = event.data;
  try {
    if (m.kind === 'init') {
      if (vm) throw Error('重复初始化');
      const module = await newQuickJSWASMModuleFromVariant(
        newVariant(RELEASE_SYNC, { wasmLocation: wasmURL }),
      );
      vm = new WidgetVM(module, m.bundle);
      self.postMessage({ sequence: m.sequence, ready: true });
    } else if (vm) {
      const tree = m.kind === 'render' ? vm.render(m.data) : vm.event(m.id, m.payload);
      self.postMessage({ sequence: m.sequence, tree });
    } else throw Error('组件尚未就绪');
  } catch (e) {
    self.postMessage({
      sequence: m.sequence,
      error: e instanceof Error ? e.message : '组件运行失败',
    });
  } finally {
    busy = false;
  }
};
