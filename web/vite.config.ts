/// <reference types="vitest" />
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';

const __dirname = path.dirname(fileURLToPath(import.meta.url));

export default defineConfig({
  plugins: [react()],
  build: {
    outDir: path.resolve(__dirname, '../internal/delivery/web/dist'),
    emptyOutDir: false,
    assetsDir: 'assets',
  },
  server: {
    host: '127.0.0.1',
    port: 5421,
    strictPort: true,
    proxy: {
      '/api': {
        target: 'http://127.0.0.1:7421',
        changeOrigin: false,
      },
    },
  },
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: 'src/test/setup.ts',
    include: ['src/**/*.test.{ts,tsx}'],
  },
});
