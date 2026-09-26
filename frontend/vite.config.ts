import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
export default defineConfig({
  plugins: [react()],
  worker: {
    format: 'es',
    plugins: () => [
      {
        name: 'retain-shell-formatter-runtime',
        transform(code, id) {
          // sh-syntax 0.7 omits this CJS runtime from its sideEffects list.
          // It initializes Go for the formatter's WASM module.
          if (id.endsWith('/sh-syntax/vendors/wasm_exec.cjs'))
            return { code, map: null, moduleSideEffects: true };
        },
      },
    ],
  },
  base: './',
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    rollupOptions: { output: { manualChunks: { terminal: ['@xterm/xterm', '@xterm/addon-fit'] } } },
  },
  server: { strictPort: true, port: 3415 },
});
