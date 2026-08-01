// Ambient shape for Vite's import.meta.env. The real types come from
// `vite/client` in each consuming app; this lets the shared package
// typecheck standalone without depending on vite.

interface ImportMetaEnv {
  readonly DEV: boolean;
  readonly PROD: boolean;
  readonly MODE: string;
  readonly VITE_LOG_LEVEL?: string;
  readonly [key: string]: string | boolean | undefined;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
