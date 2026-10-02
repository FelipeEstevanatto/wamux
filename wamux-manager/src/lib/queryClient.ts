import { QueryClient } from '@tanstack/react-query';

/**
 * Shared server-state cache.
 *
 * Defaults are tuned for a live operational UI: data is considered fresh for a
 * short window (so several components mounting at once share one request), it is
 * kept for five minutes after unmount, a single retry smooths transient blips,
 * and we do not refetch on window focus (the dashboard polls explicitly).
 */
const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000,
      gcTime: 5 * 60_000,
      retry: 1,
      refetchOnWindowFocus: false,
    },
  },
});

export default queryClient;
