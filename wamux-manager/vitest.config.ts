import { defineConfig } from 'vitest/config';
import path from 'path';

// Minimal Vitest setup: reuse the app's `@/*` aliases so a test imports the
// same specifiers the source does. A jsdom environment gives the few DOM globals
// (localStorage, window) a module like the auth store touches at import time.
export default defineConfig({
  resolve: {
    alias: {
      '@': path.resolve(import.meta.dirname, './src'),
      '@components': path.resolve(import.meta.dirname, './src/components'),
      '@pages': path.resolve(import.meta.dirname, './src/pages'),
      '@services': path.resolve(import.meta.dirname, './src/services'),
      '@hooks': path.resolve(import.meta.dirname, './src/hooks'),
      '@store': path.resolve(import.meta.dirname, './src/store'),
      '@types': path.resolve(import.meta.dirname, './src/types'),
      '@utils': path.resolve(import.meta.dirname, './src/utils'),
      '@styles': path.resolve(import.meta.dirname, './src/styles'),
    },
  },
  test: {
    environment: 'jsdom',
    include: ['src/**/*.test.{ts,tsx}'],
  },
});