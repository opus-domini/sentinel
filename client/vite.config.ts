import { defineConfig } from 'vite'
import { tanstackRouter } from '@tanstack/router-plugin/vite'
import viteReact from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import viteTsConfigPaths from 'vite-tsconfig-paths'

export default defineConfig({
  base: '/assets/',
  plugins: [
    tanstackRouter({ target: 'react', autoCodeSplitting: true }),
    viteTsConfigPaths({
      projects: ['./tsconfig.json'],
    }),
    tailwindcss(),
    viteReact(),
  ],
  define: {
    'process.env.NODE_ENV': JSON.stringify(
      process.env.NODE_ENV ?? 'production',
    ),
  },
  publicDir: 'public',
  server: {
    proxy: {
      '/api': 'http://127.0.0.1:4040',
      '/ws': {
        target: 'ws://127.0.0.1:4040',
        ws: true,
      },
    },
  },
  build: {
    outDir: 'dist/assets',
    emptyOutDir: true,
    sourcemap: false,
    cssCodeSplit: false,
    rollupOptions: {
      input: 'src/main.tsx',
      output: {
        entryFileNames: 'app.js',
        chunkFileNames: 'chunks/[name]-[hash].js',
        assetFileNames: (assetInfo) => {
          if (assetInfo.name?.endsWith('.css')) {
            return 'app.css'
          }
          return '[name][extname]'
        },
      },
    },
  },
})
