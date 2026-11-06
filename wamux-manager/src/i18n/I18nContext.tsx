import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react';
import type { Lang } from './translations';
import { detectLang, LANG_STORAGE_KEY, localeOf, translateTo, type Vars } from './translate';

interface I18nValue {
  lang: Lang;
  /** The BCP 47 locale for dates and numbers. */
  locale: string;
  setLang: (lang: Lang) => void;
  /** Translate a dotted key, interpolating `{name}` placeholders. */
  t: (key: string, vars?: Vars) => string;
}

const I18nContext = createContext<I18nValue | undefined>(undefined);
export function I18nProvider({ children }: { children: ReactNode }) {
  const [lang, setLangState] = useState<Lang>(detectLang);

  useEffect(() => {
    document.documentElement.lang = localeOf(lang);
  }, [lang]);

  const setLang = useCallback((next: Lang) => {
    setLangState(next);
    try {
      localStorage.setItem(LANG_STORAGE_KEY, next);
    } catch {
      /* ignore */
    }
  }, []);

  const t = useCallback((key: string, vars?: Vars) => translateTo(lang, key, vars), [lang]);

  const value = useMemo(
    () => ({ lang, locale: localeOf(lang), setLang, t }),
    [lang, setLang, t]
  );
  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>;
}

// eslint-disable-next-line react-refresh/only-export-components
export function useI18n(): I18nValue {
  const ctx = useContext(I18nContext);
  if (!ctx) throw new Error('useI18n must be used within an I18nProvider');
  return ctx;
}
