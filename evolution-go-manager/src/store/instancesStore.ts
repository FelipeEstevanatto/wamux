/**
 * Instances Store
 * Manages instances state using Zustand
 */

import { create } from 'zustand';
import type { Instance, InstanceOverview } from '@/types/instance';
import * as instancesApi from '@/services/api/instances';

// Per-instance overviews (profile picture + counts) change slowly, so they are
// fetched at most once per minute instead of on every 5s instance poll.
const OVERVIEW_TTL_MS = 60_000;

interface InstancesStore {
  instances: Instance[];
  isLoading: boolean;
  error: string | null;

  // instanceId -> overview
  overviews: Record<string, InstanceOverview>;
  overviewFetchedAt: Record<string, number>;

  // Actions
  fetchInstances: () => Promise<void>;
  fetchOverviews: (instances: Instance[]) => Promise<void>;
  addInstance: (instance: Instance) => void;
  updateInstance: (instanceName: string, updates: Partial<Instance>) => void;
  removeInstance: (instanceName: string) => void;
  setLoading: (isLoading: boolean) => void;
  setError: (error: string | null) => void;
  clearError: () => void;
}

const useInstancesStore = create<InstancesStore>()((set, get) => ({
  instances: [],
  isLoading: false,
  error: null,
  overviews: {},
  overviewFetchedAt: {},

  // Fetch all instances from API
  fetchInstances: async () => {
    set({ isLoading: true, error: null });
    try {
      const instances = await instancesApi.fetchInstances();
      set({ instances, isLoading: false });
    } catch (error) {
      console.error('Failed to fetch instances:', error);
      set({
        error:
          error instanceof Error
            ? error.message
            : 'Erro ao buscar instâncias',
        isLoading: false,
      });
    }
  },

  // Fetch the per-instance overviews (avatar, contacts, messages) for connected
  // instances, skipping ones fetched within the TTL. Best-effort: a failure
  // just leaves the previous value in place.
  fetchOverviews: async (instances: Instance[]) => {
    const now = Date.now();
    const { overviewFetchedAt } = get();
    const stale = instances.filter(
      (i) => i.connected && now - (overviewFetchedAt[i.id] ?? 0) > OVERVIEW_TTL_MS
    );
    if (stale.length === 0) return;

    const results = await Promise.all(
      stale.map(async (instance) => {
        try {
          const overview = await instancesApi.fetchInstanceOverview(instance.id);
          return [instance.id, overview] as const;
        } catch (error) {
          console.error(`Failed to fetch overview for ${instance.id}:`, error);
          return null;
        }
      })
    );

    const overviews = { ...get().overviews };
    const fetchedAt = { ...get().overviewFetchedAt };
    for (const result of results) {
      if (result) {
        overviews[result[0]] = result[1];
        fetchedAt[result[0]] = now;
      }
    }
    set({ overviews, overviewFetchedAt: fetchedAt });
  },

  // Add a new instance to the store
  addInstance: (instance: Instance) => {
    set((state) => ({
      instances: [...state.instances, instance],
    }));
  },

  // Update an existing instance
  updateInstance: (instanceName: string, updates: Partial<Instance>) => {
    set((state) => ({
      instances: state.instances.map((instance) =>
        instance.instanceName === instanceName
          ? { ...instance, ...updates }
          : instance
      ),
    }));
  },

  // Remove an instance from the store
  removeInstance: (instanceName: string) => {
    set((state) => ({
      instances: state.instances.filter(
        (instance) => instance.instanceName !== instanceName
      ),
    }));
  },

  // Set loading state
  setLoading: (isLoading: boolean) => {
    set({ isLoading });
  },

  // Set error message
  setError: (error: string | null) => {
    set({ error });
  },

  // Clear error
  clearError: () => {
    set({ error: null });
  },
}));

export default useInstancesStore;
