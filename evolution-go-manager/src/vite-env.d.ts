/// <reference types="vite/client" />

// The design system exposes its stylesheet as a subpath export (0.0.6+) with no
// type declaration; a bare declaration keeps the side-effect import happy.
declare module '@evoapi/design-system/styles';

interface ImportMetaEnv {
  readonly VITE_DEFAULT_API_URL: string
  readonly VITE_DEFAULT_WS_URL: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}
