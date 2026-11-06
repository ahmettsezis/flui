import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';

const backend = { target: 'https://127.0.0.1:8443', secure: false };

export default defineConfig({
  plugins: [svelte()],
  build: {
    outDir: '../internal/webui/dist',
    emptyOutDir: true,
    sourcemap: false,
  },
  server: {
    proxy: { '/api': backend, '/healthz': backend, '/readyz': backend },
  },
});
