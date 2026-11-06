import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';

import PageSkeleton from './PageSkeleton';

(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT =
  true;

describe('PageSkeleton', () => {
  let root: Root;
  let host: HTMLDivElement;

  const blocks = async (path: string) => {
    await act(async () => {
      root.render(
        <MemoryRouter key={path} initialEntries={[path]}>
          <PageSkeleton />
        </MemoryRouter>
      );
    });
    return host.querySelectorAll('[data-slot="skeleton"]').length;
  };

  beforeEach(() => {
    localStorage.clear();
    host = document.body.appendChild(document.createElement('div'));
    root = createRoot(host);
  });

  afterEach(() => {
    act(() => root.unmount());
    host.remove();
  });

  // The path is written on the wrapper, so it is left out of the comparison.
  const shape = () => host.innerHTML.replace(/data-page-skeleton="[^"]*"/, '');

  it('gives every page its own shape', async () => {
    const paths = [
      '/manager',
      '/manager/instances',
      '/manager/instances/abc/settings',
      '/manager/messages',
      '/manager/events',
      '/manager/api-tester',
      '/manager/about',
    ];
    const shapes = new Set<string>();
    for (const path of paths) {
      await blocks(path);
      shapes.add(shape());
    }

    expect(shapes.size).toBe(paths.length);
  });

  it('does not take instance settings for the instance list', async () => {
    await blocks('/manager/instances/abc/settings');

    expect(host.querySelector('.max-w-4xl')).not.toBeNull();
  });

  it('falls back to the dashboard', async () => {
    const unknown = await blocks('/manager/anything');
    const dashboard = await blocks('/manager');

    expect(unknown).toBe(dashboard);
  });

  it('matches the conversation list width the user chose', async () => {
    localStorage.setItem('wamux.messages.listWidth', '500');
    await blocks('/manager/messages');

    expect(host.innerHTML).toContain('--list-width: 500px');
  });

  it('announces that the page is loading', async () => {
    await blocks('/manager');

    expect(host.querySelector('[role="status"]')?.getAttribute('aria-busy')).toBe('true');
  });
});
