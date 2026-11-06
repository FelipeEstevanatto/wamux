import { translations, type Lang } from './translations';

export type Vars = Record<string, string | number>;

export const LANG_STORAGE_KEY = 'wamux-lang';

export function detectLang(): Lang {
  if (typeof window === 'undefined') return 'pt';
  const saved = localStorage.getItem(LANG_STORAGE_KEY);
  if (saved === 'pt' || saved === 'en') return saved;
  return navigator.language?.toLowerCase().startsWith('en') ? 'en' : 'pt';
}

export const localeOf = (lang: Lang): string => (lang === 'pt' ? 'pt-BR' : 'en-US');

export function translateTo(lang: Lang, key: string, vars?: Vars): string {
  const raw = translations[lang][key] ?? translations.pt[key] ?? key;
  if (!vars) return raw;
  return raw.replace(/\{(\w+)\}/g, (_, name: string) => String(vars[name] ?? `{${name}}`));
}

/** For code outside React, such as the API client. */
export const translate = (key: string, vars?: Vars): string =>
  translateTo(detectLang(), key, vars);
