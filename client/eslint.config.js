//  @ts-check

import { tanstackConfig } from '@tanstack/eslint-config'

export default [
  {
    ignores: [
      '.output/**',
      'dist/**',
      'public/sw.js',
      'eslint.config.js',
      'prettier.config.js',
      'tailwind.config.js',
    ],
  },
  ...tanstackConfig,
]
