import { create } from 'zustand';

interface ConnectionState {
  /** True while requests are failing with a network error and we have not seen a success yet. */
  reconnecting: boolean;
  setReconnecting: (value: boolean) => void;
}

/**
 * Process-wide connectivity flag, driven by the API client (see
 * services/api/client.ts). The banner reads it to tell the operator that live
 * data may be stale.
 */
const useConnectionStore = create<ConnectionState>()((set) => ({
  reconnecting: false,
  setReconnecting: (value) => set({ reconnecting: value }),
}));

export default useConnectionStore;
