export function formatCollectorCommand(command: string): Promise<string> {
  return new Promise((resolve, reject) => {
    const worker = new Worker(new URL('./collectorFormatter.worker.ts', import.meta.url), {
      type: 'module',
    });
    const finish = (code?: string) => {
      clearTimeout(timer);
      worker.terminate();
      if (typeof code === 'string') resolve(code);
      else reject(new Error('采集命令无法格式化'));
    };
    const timer = setTimeout(() => finish(), 5000);
    worker.onmessage = (event: MessageEvent<{ code?: string }>) => finish(event.data.code);
    worker.onerror = () => finish();
    worker.postMessage(command);
  });
}
