/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_APP_TARGET: 'mobile' | 'tv'
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}
