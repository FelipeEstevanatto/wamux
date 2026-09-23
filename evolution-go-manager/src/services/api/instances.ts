/**
 * Instances API Service
 * Handles all Evolution GO instance-related API calls
 */

import apiClient from './client';
import type {
  Instance,
  InstanceOverview,
  ProxyConfig,
  ProxyTestResult,
  RawInstance,
  InstancesResponse,
  CreateInstancePayload,
  ConnectionState,
  InstanceStatus,
} from '@/types/instance';

/**
 * Normalize raw instance data from Evolution GO API
 */
const normalizeInstance = (raw: RawInstance): Instance => {
  // Determine status from connected field
  const status: InstanceStatus = raw.connected ? 'open' : 'close';

  // Parse QR code if available
  let qrcode: Instance['qrcode'];
  if (raw.qrcode) {
    const parts = raw.qrcode.split('|');
    qrcode = {
      base64: parts[0] || undefined,
      code: parts[1] || undefined,
    };
  }

  return {
    id: raw.id,
    instanceName: raw.name,
    status,
    apikey: raw.token,
    // jid is "5514991421911:5@s.whatsapp.net": drop the "@server" and the
    // ":device" agent suffix, so the UI shows the plain phone number.
    owner: raw.jid ? raw.jid.split('@')[0].split(':')[0] : '',
    profileName: raw.name,
    connected: raw.connected,
    qrcode,
    webhook: raw.webhook || undefined,
    rabbitmqEnable: raw.rabbitmqEnable || undefined,
    websocketEnable: raw.websocketEnable || undefined,
    natsEnable: raw.natsEnable || undefined,
    events: raw.events || undefined,
    disconnectReason: raw.disconnect_reason || undefined,
    createdAt: raw.createdAt,
    alwaysOnline: raw.alwaysOnline,
    rejectCall: raw.rejectCall,
    readMessages: raw.readMessages,
    ignoreGroups: raw.ignoreGroups,
    ignoreStatus: raw.ignoreStatus,
  };
};

/**
 * Fetch all instances
 * GET /instance/all
 */
export const fetchInstances = async (): Promise<Instance[]> => {
  const response = await apiClient.get<InstancesResponse>('/instance/all');
  // Normalize the instances from Evolution GO format to our format
  return response.data.data.map(normalizeInstance);
};

/**
 * Fetch a single instance by ID
 * GET /instance/info/:instanceId
 */
export const fetchInstance = async (instanceId: string): Promise<Instance> => {
  const response = await apiClient.get<{
    message: string;
    data: RawInstance;
  }>(`/instance/info/${instanceId}`);
  return normalizeInstance(response.data.data);
};

/**
 * Fetch the per-instance overview (own profile picture, push name, contact and
 * message counts)
 * GET /instance/overview/:instanceId
 * Uses the admin apikey (default header)
 */
export const fetchInstanceOverview = async (
  instanceId: string
): Promise<InstanceOverview> => {
  const response = await apiClient.get<{
    message: string;
    data: InstanceOverview;
  }>(`/instance/overview/${instanceId}`);
  return response.data.data;
};

/**
 * Create a new instance
 * POST /instance/create
 */
export const createInstance = async (
  payload: CreateInstancePayload
): Promise<Instance> => {
  const response = await apiClient.post<Instance>('/instance/create', payload);
  return response.data;
};

export interface ConnectConfig {
  webhookUrl?: string;
  subscribe?: string[];
  phone?: string;
  rabbitmqEnable?: string;
  websocketEnable?: string;
  natsEnable?: string;
  alwaysOnline?: boolean;
  rejectCall?: boolean;
  readMessages?: boolean;
  ignoreGroups?: boolean;
  ignoreStatus?: boolean;
}

export interface PairConfig {
  subscribe: string[];
  phone: string;
}

export interface AdvancedSettings {
  alwaysOnline?: boolean;
  rejectCall?: boolean;
  readMessages?: boolean;
  ignoreGroups?: boolean;
  ignoreStatus?: boolean;
}

/**
 * Connect to an instance (get QR code or connection status)
 * POST /instance/connect
 * Requires instance token in apikey header
 */
export const connectInstance = async (
  instanceToken: string,
  config?: ConnectConfig
): Promise<{ jid: string; webhookUrl: string; eventString: string }> => {
  const payload = {
    webhookUrl: config?.webhookUrl || '',
    subscribe: config?.subscribe || [],
    rabbitmqEnable: config?.rabbitmqEnable || '',
    websocketEnable: config?.websocketEnable || '',
    natsEnable: config?.natsEnable || '',
  };

  const response = await apiClient.post<{
    message: string;
    data: { jid: string; webhookUrl: string; eventString: string };
  }>(
    '/instance/connect',
    payload,
    {
      headers: {
        apikey: instanceToken,
      },
    }
  );
  return response.data.data;
};

/**
 * Pair instance with phone number (get pairing code)
 * POST /instance/pair
 * Requires instance token in apikey header
 */
export const pairInstance = async (
  instanceToken: string,
  config: PairConfig
): Promise<{ pairingCode: string }> => {
  const response = await apiClient.post<{
    message: string;
    data: { PairingCode: string };
  }>(
    '/instance/pair',
    {
      subscribe: config.subscribe,
      phone: config.phone,
    },
    {
      headers: {
        apikey: instanceToken,
      },
    }
  );

  return {
    pairingCode: response.data.data.PairingCode,
  };
};

/**
 * Get advanced settings for an instance
 * GET /instance/:instanceId/advanced-settings
 * Requires instance token in apikey header
 */
export const getAdvancedSettings = async (
  instanceId: string,
  instanceToken: string
): Promise<AdvancedSettings> => {
  const response = await apiClient.get<{
    message: string;
    data: AdvancedSettings;
  }>(
    `/instance/${instanceId}/advanced-settings`,
    {
      headers: {
        apikey: instanceToken,
      },
    }
  );
  return response.data.data;
};

/**
 * Update advanced settings for an instance
 * PUT /instance/:instanceId/advanced-settings
 * Requires instance token in apikey header
 */
export const updateAdvancedSettings = async (
  instanceId: string,
  instanceToken: string,
  settings: AdvancedSettings
): Promise<void> => {
  await apiClient.put(
    `/instance/${instanceId}/advanced-settings`,
    settings,
    {
      headers: {
        apikey: instanceToken,
      },
    }
  );
};

/**
 * Get QR Code for an instance
 * GET /instance/qr
 * Requires instance token in apikey header
 */
export const getQrCode = async (
  instanceToken: string
): Promise<{ qrcode: string; code: string }> => {
  // The API answers { data: { qrcode, code } } in lowercase. This used to read
  // data.Qrcode / data.Code (capitalised): axios type annotations are not
  // checked at runtime, so the fields came back undefined with no error and the
  // modal opened with a blank QR code. Both spellings are accepted so older
  // installs keep working.
  const response = await apiClient.get<{
    message: string;
    data: { qrcode?: string; code?: string; Qrcode?: string; Code?: string };
  }>('/instance/qr', {
    headers: {
      apikey: instanceToken,
    },
  });

  const data = response.data?.data;

  return {
    qrcode: data?.qrcode ?? data?.Qrcode ?? '',
    code: data?.code ?? data?.Code ?? '',
  };
};

/**
 * Proxy configuration for an instance (admin apikey).
 * GET/POST/DELETE /instance/proxy/:instanceId, POST .../test and .../reconnect
 */
export const getProxy = async (
  instanceId: string
): Promise<ProxyConfig | null> => {
  const response = await apiClient.get<{
    message: string;
    data: ProxyConfig | null;
  }>(`/instance/proxy/${instanceId}`);
  return response.data.data;
};

export const setProxy = async (
  instanceId: string,
  config: ProxyConfig
): Promise<void> => {
  await apiClient.post(`/instance/proxy/${instanceId}`, config);
};

/** Tests a proxy. Omit `config` (or pass an empty host) to test the saved one. */
export const testProxy = async (
  instanceId: string,
  config?: ProxyConfig
): Promise<ProxyTestResult> => {
  const response = await apiClient.post<ProxyTestResult>(
    `/instance/proxy/${instanceId}/test`,
    config ?? {}
  );
  return response.data;
};

export const reconnectProxy = async (instanceId: string): Promise<void> => {
  await apiClient.post(`/instance/proxy/${instanceId}/reconnect`);
};

export const deleteProxy = async (instanceId: string): Promise<void> => {
  await apiClient.delete(`/instance/proxy/${instanceId}`);
};

/**
 * Get connection status of an instance
 * GET /instance/status
 */
export const getConnectionState = async (): Promise<ConnectionState> => {
  const response = await apiClient.get<ConnectionState>('/instance/status');
  return response.data;
};

/**
 * Disconnect/logout an instance
 * DELETE /instance/logout
 */
export const logoutInstance = async (instanceToken: string): Promise<void> => {
  await apiClient.delete('/instance/logout', {
    headers: {
      apikey: instanceToken,
    },
  });
};

/**
 * Delete an instance permanently
 * DELETE /instance/delete/:instanceId
 */
export const deleteInstance = async (instanceId: string): Promise<void> => {
  await apiClient.delete(`/instance/delete/${instanceId}`);
};

/**
 * Reconnect an instance
 * POST /instance/reconnect
 */
export const restartInstance = async (): Promise<void> => {
  await apiClient.post('/instance/reconnect');
};

/**
 * Send a text message
 * POST /send/text
 * Requires instance token in apikey header
 */
export const sendMessage = async (
  instanceToken: string,
  payload: { number: string; text: string }
): Promise<{ message: string; data: unknown }> => {
  const response = await apiClient.post<{
    message: string;
    data: unknown;
  }>(
    '/send/text',
    payload,
    {
      headers: {
        apikey: instanceToken,
      },
    }
  );
  return response.data;
};

/**
 * Send a button message (test scenarios)
 * POST /send/button
 */
export const sendButtonMessage = async (
  instanceToken: string,
  payload: Record<string, unknown>
): Promise<{ message: string; data: unknown }> => {
  const response = await apiClient.post<{ message: string; data: unknown }>(
    '/send/button',
    payload,
    { headers: { apikey: instanceToken } }
  );
  return response.data;
};

/**
 * Send a list message (test scenarios)
 * POST /send/list
 */
export const sendListMessage = async (
  instanceToken: string,
  payload: Record<string, unknown>
): Promise<{ message: string; data: unknown }> => {
  const response = await apiClient.post<{ message: string; data: unknown }>(
    '/send/list',
    payload,
    { headers: { apikey: instanceToken } }
  );
  return response.data;
};

/**
 * Send a carousel message (test scenarios)
 * POST /send/carousel
 */
export const sendCarouselMessage = async (
  instanceToken: string,
  payload: Record<string, unknown>
): Promise<{ message: string; data: unknown }> => {
  const response = await apiClient.post<{ message: string; data: unknown }>(
    '/send/carousel',
    payload,
    { headers: { apikey: instanceToken } }
  );
  return response.data;
};

export default {
  fetchInstances,
  fetchInstance,
  fetchInstanceOverview,
  createInstance,
  connectInstance,
  pairInstance,
  getAdvancedSettings,
  updateAdvancedSettings,
  getQrCode,
  getProxy,
  setProxy,
  testProxy,
  reconnectProxy,
  deleteProxy,
  getConnectionState,
  logoutInstance,
  deleteInstance,
  restartInstance,
  sendMessage,
  sendButtonMessage,
  sendListMessage,
  sendCarouselMessage,
};
