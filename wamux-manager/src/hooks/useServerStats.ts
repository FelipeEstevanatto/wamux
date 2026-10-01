import { useEffect, useState } from 'react';
import { fetchServerStats, type ServerStats } from '@/services/api/server';

/**
 * System-wide metrics from GET /server/stats. Polls every `pollMs` when > 0
 * (0 = fetch once). Used by the sidebar (version) and the dashboard.
 */
export function useServerStats(pollMs = 0) {
  const [stats, setStats] = useState<ServerStats | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let alive = true;

    const load = async () => {
      try {
        const data = await fetchServerStats();
        if (!alive) return;
        setStats(data);
        setError(null);
      } catch (e) {
        if (!alive) return;
        setError(e instanceof Error ? e.message : 'Erro ao buscar métricas');
      } finally {
        if (alive) setLoading(false);
      }
    };

    load();
    const timer = pollMs > 0 ? setInterval(load, pollMs) : undefined;
    return () => {
      alive = false;
      if (timer) clearInterval(timer);
    };
  }, [pollMs]);

  return { stats, error, loading };
}

export default useServerStats;
