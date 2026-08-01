export type Locale = 'en' | 'fa';
export type Direction = 'ltr' | 'rtl';

// Messages are arbitrarily nested records of strings. Each app passes
// its own tree; the shared layer deep-merges over the `common` tree.
export type LocaleMessages = Record<string, unknown>;

export type LocaleMessageTree = Record<Locale, LocaleMessages>;
