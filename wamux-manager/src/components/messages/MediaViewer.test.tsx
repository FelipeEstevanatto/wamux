import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { I18nProvider } from '@/i18n/I18nContext';
import type { HistoryMessage } from '@/types/messages';
import MediaViewer from './MediaViewer';

(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT =
  true;

const download = vi.hoisted(() => vi.fn());

vi.mock('@/hooks/useMediaSrc', () => ({
  useMediaSrc: (url: string) => ({ src: url ? `blob:${url}` : '', loading: false, failed: !url }),
}));
vi.mock('@/utils/downloadMessageFile', () => ({ downloadMessageFile: download }));
vi.mock('sonner', () => ({ toast: { error: vi.fn() } }));

const image = (id: string, overrides: Partial<HistoryMessage> = {}) =>
  ({
    message_id: id,
    media_url: `/chat/media/${id}`,
    media_mimetype: 'image/png',
    timestamp: '2026-01-02 10:00:00',
    text_content: '',
    file_size: 2 * 1024 * 1024,
    ...overrides,
  }) as HistoryMessage;

const label = (name: string) => document.querySelector<HTMLElement>(`[aria-label="${name}"]`);
const shownImage = () => document.querySelector('img');
const transform = () => shownImage()?.style.transform ?? '';

describe('MediaViewer', () => {
  let root: Root;
  const onClose = vi.fn();

  const render = async (items: HistoryMessage[], index = 0) => {
    await act(async () => {
      root.render(
        <I18nProvider>
          <MediaViewer open items={items} initialIndex={index} token="t" onClose={onClose} />
        </I18nProvider>
      );
    });
  };

  const press = async (key: string) => {
    await act(async () => {
      document.querySelector('[role="dialog"]')?.dispatchEvent(
        new KeyboardEvent('keydown', { key, bubbles: true })
      );
    });
  };

  beforeEach(() => {
    localStorage.setItem('wamux-lang', 'pt');
    onClose.mockClear();
    download.mockReset().mockResolvedValue(undefined);
    root = createRoot(document.body.appendChild(document.createElement('div')));
  });

  afterEach(() => {
    act(() => root.unmount());
    document.body.innerHTML = '';
  });

  it('fills the screen', async () => {
    await render([image('a')]);

    const classes = document.querySelector('[role="dialog"]')?.className ?? '';
    expect(classes).toContain('w-screen');
    expect(classes).toContain('h-dvh');
    expect(classes).toContain('sm:max-w-none');
  });

  it('shows what it knows without downloading anything else', async () => {
    await render([image('a', { file_name: 'photo.png', text_content: 'At the beach' })]);

    const dialog = document.querySelector('[role="dialog"]');
    expect(dialog?.textContent).toContain('photo.png');
    expect(dialog?.textContent).toContain('Imagem');
    expect(dialog?.textContent).toContain('2.0 MB');
    expect(dialog?.textContent).toContain('At the beach');
  });

  it('adds the dimensions once the image is on screen', async () => {
    await render([image('a')]);
    const img = shownImage() as HTMLImageElement;
    Object.defineProperty(img, 'naturalWidth', { value: 1920 });
    Object.defineProperty(img, 'naturalHeight', { value: 1080 });

    await act(async () => img.dispatchEvent(new Event('load')));

    expect(document.body.textContent).toContain('1920 × 1080');
  });

  it('zooms from the toolbar and resets', async () => {
    await render([image('a')]);
    expect(document.body.textContent).toContain('100%');

    await act(async () => label('Aumentar zoom')?.click());
    expect(document.body.textContent).toContain('150%');
    expect(transform()).toContain('scale(1.5)');

    await act(async () => label('Ajustar à tela')?.click());
    expect(transform()).toContain('scale(1)');
  });

  it('zooms from the keyboard', async () => {
    await render([image('a')]);

    await press('+');
    expect(transform()).toContain('scale(1.5)');
    await press('-');
    expect(transform()).toContain('scale(1)');
    await press('+');
    await press('0');
    expect(transform()).toContain('scale(1)');
  });

  it('cannot zoom out past the fitted size', async () => {
    await render([image('a')]);

    expect((label('Diminuir zoom') as HTMLButtonElement).disabled).toBe(true);
    await press('-');
    expect(transform()).toContain('scale(1)');
  });

  it('rotates in quarter turns', async () => {
    await render([image('a')]);

    await act(async () => label('Girar')?.click());
    expect(transform()).toContain('rotate(90deg)');
    await press('r');
    expect(transform()).toContain('rotate(180deg)');
  });

  it('zooms on the wheel without scrolling the page', async () => {
    await render([image('a')]);
    const wheel = new WheelEvent('wheel', { deltaY: -100, cancelable: true, bubbles: true });

    await act(async () => document.querySelector('[data-testid="zoom-stage"]')?.dispatchEvent(wheel));

    expect(wheel.defaultPrevented).toBe(true);
    expect(transform()).toContain('scale(1.2)');
  });

  it('toggles zoom on a double click', async () => {
    await render([image('a')]);
    const stage = document.querySelector('[data-testid="zoom-stage"]');

    await act(async () => stage?.dispatchEvent(new MouseEvent('dblclick', { bubbles: true })));
    expect(transform()).toContain('scale(2.5)');

    await act(async () => stage?.dispatchEvent(new MouseEvent('dblclick', { bubbles: true })));
    expect(transform()).toContain('scale(1)');
  });

  it('moves between items, starting each one fresh', async () => {
    await render([image('a'), image('b')]);
    expect(document.body.textContent).toContain('1 / 2');

    await press('+');
    await press('r');
    await press('ArrowRight');

    expect(document.body.textContent).toContain('2 / 2');
    expect(shownImage()?.getAttribute('src')).toBe('blob:/chat/media/b');
    expect(transform()).toContain('scale(1)');
    expect(transform()).toContain('rotate(0deg)');

    await press('ArrowRight');
    expect(document.body.textContent).toContain('1 / 2');
  });

  it('leaves zoom controls out for videos', async () => {
    await render([image('v', { media_mimetype: 'video/mp4' })]);

    expect(document.querySelector('video')).not.toBeNull();
    expect(document.body.textContent).toContain('Vídeo');
    expect(label('Aumentar zoom')).toBeNull();
  });

  it('downloads through the shared path', async () => {
    const item = image('a');
    await render([item]);

    await act(async () => label('Baixar')?.click());

    expect(download).toHaveBeenCalledWith('t', item);
  });

  it('closes from its button', async () => {
    await render([image('a')]);

    await act(async () => label('Fechar')?.click());

    expect(onClose).toHaveBeenCalled();
  });

  it('says so when the file cannot load', async () => {
    await render([image('a', { media_url: '' })]);

    expect(document.body.textContent).toContain('Não foi possível carregar a mídia');
  });
});
