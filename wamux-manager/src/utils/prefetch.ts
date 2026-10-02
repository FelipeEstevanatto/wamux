/**
 * Route prefetching.
 *
 * Pages behind the login are lazy-loaded, so the first navigation to one pays a
 * network round-trip. Prefetching the chunk on hover/focus (and once on idle)
 * makes navigation feel instant without changing what is in the initial bundle.
 */
const loaders: Record<string, () => Promise<unknown>> = {
  '/manager': () => import('@/pages/Dashboard'),
  '/manager/instances': () => import('@/pages/Instances'),
  '/manager/messages': () => import('@/pages/Messages'),
  '/manager/events': () => import('@/pages/Events'),
  '/manager/settings': () => import('@/pages/Settings'),
  '/manager/api-tester': () => import('@/pages/ApiTester'),
  '/manager/about': () => import('@/pages/About'),
  '/manager/terms': () => import('@/pages/Terms'),
  '/manager/privacy': () => import('@/pages/Privacy'),
};

const inFlight = new Set<string>();

export function prefetchRoute(path: string): void {
  const load = loaders[path];
  if (!load || inFlight.has(path)) return;
  inFlight.add(path);
  void load().catch(() => inFlight.delete(path));
}

/** Prefetch the most likely next routes when the browser is idle. */
export function prefetchLikelyRoutes(): void {
  const run = () => {
    prefetchRoute('/manager/instances');
    prefetchRoute('/manager/messages');
  };
  const ric = (
    window as Window & { requestIdleCallback?: (cb: () => void) => number }
  ).requestIdleCallback;
  if (typeof ric === 'function') {
    ric(run);
  } else {
    window.setTimeout(run, 1500);
  }
}
