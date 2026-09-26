/// <reference types="vite/client" />
/// <reference lib="webworker" />
import { getProcessor } from 'sh-syntax';
import initWasm from 'sh-syntax/main.wasm?init';

const processor = getProcessor(initWasm);

self.onmessage = async (event: MessageEvent<string>) => {
  try {
    const code = await processor(event.data, { print: true, indent: 2, simplify: false });
    self.postMessage({ code });
  } catch {
    self.postMessage({ error: true });
  }
};
