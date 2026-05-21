import { defineConfig } from 'vitest/config'

export default defineConfig({
  resolve: {
    tsconfigPaths: true,
  },
  test: {
    environment: 'node',
    include: ['src/**/*.e2e.test.ts', 'src/**/*.e2e.test.tsx'],
  },
})
