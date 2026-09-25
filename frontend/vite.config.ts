import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
export default defineConfig({
  plugins: [react()],
  base: './',
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    rollupOptions: { output: { manualChunks: { terminal: ['@xterm/xterm', '@xterm/addon-fit'] } } },
  },
  server: { strictPort: true, port: 3415 },
});
