import { describe, expect, it } from 'vitest';

import { translateTo } from '@/i18n/translate';
import { chatHeading } from './chatTitle';

const t = (key: string, vars?: Record<string, string | number>) =>
  translateTo('en', key, vars);

describe('chatHeading', () => {
  it('puts the contact name above the number', () => {
    expect(chatHeading('5511999999999@s.whatsapp.net', 'Ana', t)).toEqual({
      title: 'Ana',
      detail: '5511999999999',
    });
  });

  it('does not use a raw JID as the title when the name is unknown', () => {
    expect(chatHeading('5511999999999@s.whatsapp.net', '', t)).toEqual({
      title: 'Contact',
      detail: '5511999999999',
    });
    expect(chatHeading('abc@lid', 'abc@lid', t)).toEqual({
      title: 'Contact',
      detail: 'abc',
    });
    expect(chatHeading('120363@g.us', undefined, t)).toEqual({
      title: 'Group',
      detail: '120363',
    });
  });

  it('keeps a group subject without the raw id underneath', () => {
    expect(chatHeading('120363@g.us', 'Family', t)).toEqual({
      title: 'Family',
      detail: '',
    });
  });
});
