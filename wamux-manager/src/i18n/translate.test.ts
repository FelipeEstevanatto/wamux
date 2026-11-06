import { afterEach, describe, expect, it, vi } from 'vitest';

import { detectLang, localeOf, translate, translateTo } from './translate';
import { translations } from './translations';

describe('translate', () => {
  afterEach(() => {
    localStorage.clear();
    vi.unstubAllGlobals();
  });

  it('follows the saved language outside React', () => {
    localStorage.setItem('wamux-lang', 'en');
    expect(translate('common.unknownError')).toBe('Unknown error');

    localStorage.setItem('wamux-lang', 'pt');
    expect(translate('common.unknownError')).toBe('Erro desconhecido');
  });

  it('falls back to the browser language', () => {
    vi.stubGlobal('navigator', { language: 'en-GB' });
    expect(detectLang()).toBe('en');

    vi.stubGlobal('navigator', { language: 'fr-FR' });
    expect(detectLang()).toBe('pt');
  });

  it('fills placeholders, and leaves unknown ones visible', () => {
    expect(translateTo('en', 'login.urlInvalid', { example: 'http://x' })).toContain('http://x');
    expect(translateTo('en', 'login.urlInvalid')).toContain('{example}');
  });

  it('returns an unknown key as it is', () => {
    expect(translateTo('en', 'no.such.key')).toBe('no.such.key');
  });

  it('maps languages to locales', () => {
    expect(localeOf('pt')).toBe('pt-BR');
    expect(localeOf('en')).toBe('en-US');
  });

  it('has every key in both languages', () => {
    const missing = (from: 'pt' | 'en', to: 'pt' | 'en') =>
      Object.keys(translations[from]).filter((key) => !(key in translations[to]));

    expect(missing('pt', 'en')).toEqual([]);
    expect(missing('en', 'pt')).toEqual([]);
  });
});
