import { defineConfig } from 'vitest/config'
import viteTsConfigPaths from 'vite-tsconfig-paths'

export default defineConfig({
  plugins: [viteTsConfigPaths({ projects: ['./tsconfig.json'] })],
  test: {
    environment: 'node',
    include: ['src/**/*.e2e.test.ts', 'src/**/*.e2e.test.tsx'],
  },
})
