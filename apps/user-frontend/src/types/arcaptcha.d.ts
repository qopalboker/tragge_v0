declare global {
  interface Window {
    arcaptcha: {
      render: (
        container: HTMLElement | null,
        options: {
          'site-key': string;
          size?: 'invisible' | 'normal';
          callback?: (token: string) => void;
          error_callback?: () => void;
          theme?: 'light' | 'dark';
          lang?: 'en' | 'fa';
        }
      ) => string;
      execute: (widgetId?: string) => void;
      reset: (widgetId?: string) => void;
    };
  }
}

export {};
