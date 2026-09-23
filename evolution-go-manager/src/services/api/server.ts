/**
 * Server API Service
 * System-wide metrics from GET /server/stats (AuthAdmin).
 */

import apiClient from './client';

export interface StatKV {
  key: string;
  count: number;
  name?: string;
  phone?: string;
}

export interface ServerSystemStats {
  version?: string;
  goVersion?: string;
  goroutines?: number;
  numCpu?: number;
  uptimeSeconds?: number;
  memAllocMB?: number;
  memSysMB?: number;
  heapInuseMB?: number;
  numGC?: number;
  loadAvg1?: number;
  loadAvg5?: number;
  loadAvg15?: number;
  hostMemTotalMB?: number;
  hostMemAvailableMB?: number;
  hostMemUsedPct?: number;
}

export interface ServerMessageStats {
  total?: number;
  byStatus?: StatKV[];
  byDay?: StatKV[];
  topSources?: StatKV[];
}

export interface ServerStorageStats {
  diskPath?: string;
  diskTotalMB?: number;
  diskUsedMB?: number;
  diskAvailableMB?: number;
  diskUsedPct?: number;
  dataDir?: string;
  dataUsedMB?: number;
  dataFiles?: number;
  dbTotalMB?: number;
  dbMessagesMB?: number;
  mediaEnabled?: boolean;
  mediaBackend?: string;
}

export interface ServerStats {
  system: ServerSystemStats;
  messages: ServerMessageStats;
  storage?: ServerStorageStats;
}

export const fetchServerStats = async (): Promise<ServerStats> => {
  const response = await apiClient.get<ServerStats>('/server/stats');
  return response.data;
};

export default { fetchServerStats };
