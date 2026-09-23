/**
 * Server API Service
 * System-wide metrics from GET /server/stats (AuthAdmin).
 */

import apiClient from './client';

export interface StatKV {
  key: string;
  count: number;
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

export interface ServerStats {
  system: ServerSystemStats;
  messages: ServerMessageStats;
}

export const fetchServerStats = async (): Promise<ServerStats> => {
  const response = await apiClient.get<ServerStats>('/server/stats');
  return response.data;
};

export default { fetchServerStats };
