import { useCallback, useMemo } from 'react';
import { useQueries, useQuery, useQueryClient } from '@tanstack/react-query';
import * as instancesApi from '@/services/api/instances';
import type { Instance, InstanceOverview } from '@/types/instance';
import queryClient from '@/lib/queryClient';

export const instanceKeys = {
  list: ['instances'] as const,
  overview: (id: string) => ['instance-overview', id] as const,
};

// Instances change often (connect/disconnect, pairing) — poll centrally here so
// no page needs its own interval.
const POLL_MS = 30_000;
const EMPTY: Instance[] = [];

/**
 * The instances list and each connected instance's overview, from the shared query
 * cache. Pass `pollMs` to change how often the list refreshes, or 0 to stop.
 */
export function useInstances(pollMs = POLL_MS) {
  const qc = useQueryClient();

  const list = useQuery<Instance[], Error>({
    queryKey: instanceKeys.list,
    queryFn: instancesApi.fetchInstances,
    refetchInterval: pollMs > 0 ? pollMs : false,
  });
  const instances = list.data ?? EMPTY;

  const connected = useMemo(
    () => instances.filter((i) => i.connected),
    [instances]
  );

  const overviewResults = useQueries({
    queries: connected.map((i) => ({
      queryKey: instanceKeys.overview(i.id),
      queryFn: () => instancesApi.fetchInstanceOverview(i.id),
      staleTime: 60_000,
      retry: 0,
    })),
  });

  const overviews = useMemo(() => {
    const out: Record<string, InstanceOverview> = {};
    connected.forEach((instance, index) => {
      const data = overviewResults[index]?.data;
      if (data) out[instance.id] = data;
    });
    return out;
  }, [connected, overviewResults]);

  const fetchInstances = useCallback(
    async () => {
      await qc.refetchQueries({ queryKey: instanceKeys.list, type: 'active' });
    },
    [qc]
  );

  // Manual refresh: keep the list on screen and spin the button long enough to
  // be perceptible (matches the previous behaviour).
  const refreshInstances = useCallback(async () => {
    const startedAt = Date.now();
    await qc.refetchQueries({ queryKey: instanceKeys.list, type: 'active' });
    const elapsed = Date.now() - startedAt;
    if (elapsed < 500) {
      await new Promise((resolve) => setTimeout(resolve, 500 - elapsed));
    }
  }, [qc]);

  const addInstance = useCallback(
    (instance: Instance) => {
      qc.setQueryData<Instance[]>(instanceKeys.list, (old) =>
        old ? [...old, instance] : [instance]
      );
    },
    [qc]
  );

  const updateInstance = useCallback(
    (instanceName: string, updates: Partial<Instance>) => {
      qc.setQueryData<Instance[]>(instanceKeys.list, (old) =>
        old?.map((i) =>
          i.instanceName === instanceName ? { ...i, ...updates } : i
        )
      );
    },
    [qc]
  );

  const removeInstance = useCallback(
    (instanceName: string) => {
      qc.setQueryData<Instance[]>(instanceKeys.list, (old) =>
        old?.filter((i) => i.instanceName !== instanceName)
      );
    },
    [qc]
  );

  return {
    instances,
    isLoading: list.isLoading,
    isRefreshing: list.isFetching && !list.isLoading,
    error: list.error ? list.error.message : null,
    overviews,
    fetchInstances,
    refreshInstances,
    addInstance,
    updateInstance,
    removeInstance,
  };
}

/** Optimistically patch one instance in the cache (usable outside components). */
export function updateInstanceInCache(
  instanceName: string,
  updates: Partial<Instance>
): void {
  queryClient.setQueryData<Instance[]>(instanceKeys.list, (old) =>
    old?.map((i) => (i.instanceName === instanceName ? { ...i, ...updates } : i))
  );
}

/** Force a refetch of the instances list from anywhere. */
export function invalidateInstances(): Promise<unknown> {
  return queryClient.invalidateQueries({ queryKey: instanceKeys.list });
}

export default useInstances;
