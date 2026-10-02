import { useQuery } from '@tanstack/react-query';
import { fetchServerStats, type ServerStats } from '@/services/api/server';

/**
 * System-wide metrics from GET /server/stats. Cached and shared through
 * TanStack Query, so the sidebar and the dashboard mounting together issue a
 * single request. Polls every `pollMs` when > 0 (0 = fetch once and rely on the
 * cache).
 */
export function useServerStats(pollMs = 0) {
  const query = useQuery<ServerStats, Error>({
    queryKey: ['server-stats'],
    queryFn: fetchServerStats,
    refetchInterval: pollMs > 0 ? pollMs : false,
    staleTime: pollMs > 0 ? Math.max(1000, Math.floor(pollMs / 2)) : 30_000,
  });

  return {
    stats: query.data ?? null,
    error: query.error ? query.error.message : null,
    loading: query.isLoading,
  };
}

export default useServerStats;
