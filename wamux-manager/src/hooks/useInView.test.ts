import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, createElement } from 'react';
import { createRoot, type Root } from 'react-dom/client';

import { useInView } from './useInView';

// React 19 requires this flag for act() outside the test runner's own setup.
(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT =
  true;

// A component that renders the hook's current value into a data attribute so
// the test can read it without a testing-library dependency.
function Probe() {
  const [ref, inView] = useInView<HTMLDivElement>();
  return createElement('div', { ref, 'data-in-view': inView ? '1' : '0' });
}

function renderProbe(): { container: HTMLDivElement; root: Root } {
  const container = document.createElement('div');
  document.body.appendChild(container);
  const root = createRoot(container);
  act(() => {
    root.render(createElement(Probe));
  });
  return { container, root };
}

function inViewFlag(container: HTMLElement): string | undefined {
  return container.querySelector('div')?.dataset.inView;
}

afterEach(() => {
  document.body.innerHTML = '';
  vi.unstubAllGlobals();
});

describe('useInView', () => {
  it('reports true immediately when IntersectionObserver is unavailable', () => {
    // jsdom has no IntersectionObserver: the hook must fail open (load eagerly)
    // rather than never reporting the element as visible.
    expect(typeof IntersectionObserver).toBe('undefined');
    const { container } = renderProbe();
    expect(inViewFlag(container)).toBe('1');
  });

  it('stays false until the observer reports intersection, then latches true', () => {
    const callbacks: IntersectionObserverCallback[] = [];
    const observed = new Set<Element>();
    const unobserve = vi.fn((el: Element) => observed.delete(el));

    class MockIO {
      constructor(cb: IntersectionObserverCallback) {
        callbacks.push(cb);
      }
      observe(el: Element) {
        observed.add(el);
      }
      unobserve = unobserve;
      disconnect() {}
      takeRecords() {
        return [];
      }
    }
    vi.stubGlobal('IntersectionObserver', MockIO as unknown as typeof IntersectionObserver);

    const { container } = renderProbe();
    // Observer path: not yet intersecting.
    expect(inViewFlag(container)).toBe('0');
    expect(observed.size).toBe(1);

    // Fire the intersection the way the browser would.
    const el = container.querySelector('div')!;
    act(() => {
      callbacks[0](
        [{ target: el, isIntersecting: true } as unknown as IntersectionObserverEntry],
        {} as IntersectionObserver
      );
    });
    expect(inViewFlag(container)).toBe('1');
  });
});