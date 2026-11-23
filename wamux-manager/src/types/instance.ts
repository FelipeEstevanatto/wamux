/**
 * Instance Types for WaMux Manager
 * Based on WaMux API responses
 */

export type InstanceStatus = 'open' | 'close';

// Raw instance from WaMux API
export interface RawInstance {
  id: string;
  name: string;
  integration?: 'WHATSAPP-BAILEYS' | 'WHATSAPP-BUSINESS' | string;
  token: string;
  webhook: string;
  rabbitmqEnable: string;
  websocketEnable: string;
  natsEnable: string;
  jid: string;
  qrcode: string;
  connected: boolean;
  expiration: number;
  disconnect_reason: string;
  events: string;
  os_name: string;
  proxy: string;
  client_name: string;
  createdAt: string;
  alwaysOnline: boolean;
  rejectCall: boolean;
  msgRejectCall: string;
  readMessages: boolean;
  ignoreGroups: boolean;
  ignoreStatus: boolean;
}

// Normalized instance for our app
export interface Instance {
  id: string;
  instanceName: string;
  status: InstanceStatus;
  serverUrl?: string;
  apikey?: string;
  owner: string;
  profileName?: string;
  profilePicUrl?: string;
  profileStatus?: string;
  qrcode?: {
    base64?: string;
    code?: string;
    pairingCode?: string;
  };
  integration?: string;
  webhook?: string;
  rabbitmqEnable?: string;
  websocketEnable?: string;
  natsEnable?: string;
  events?: string;
  connected: boolean;
  disconnectReason?: string;
  createdAt?: string;
  updatedAt?: string;
  // client_name is the client identifier the instance was created with;
  // osName is what WhatsApp shows as the linked device (DeviceProps.Os).
  clientName?: string;
  osName?: string;
  alwaysOnline?: boolean;
  rejectCall?: boolean;
  readMessages?: boolean;
  ignoreGroups?: boolean;
  ignoreStatus?: boolean;
}

export interface CloudCredentialsInput {
  appId?: string;
  wabaId: string;
  phoneNumberId: string;
  businessId?: string;
  accessToken?: string;
  displayPhoneNumber?: string;
  enforceWindow?: boolean;
}

export interface CloudStatus {
  displayPhoneNumber?: string;
  verifiedName?: string;
  lastVerifiedAt?: string;
  lastWebhookAt?: string;
  subscribed: boolean;
  registered: boolean;
  registrationStatus?: string;
  qualityRating?: string;
  messagingLimit?: string;
  windowHours: number;
  capabilities?: Record<string, boolean>;
  capabilityNotes?: Record<string, string>;
}

export interface CloudDiagnostics {
  tokenValid: boolean;
  appSecretSet: boolean;
  phoneRegistered: boolean;
  subscribed: boolean;
  displayPhoneNumber?: string;
  verifiedName?: string;
  qualityRating?: string;
  messagingLimit?: string;
  lastGraphError?: string;
  lastWebhookAt?: string;
  effectiveRPS?: number;
  templatesApproved?: number;
  templatesPending?: number;
  templatesRejected?: number;
}

export interface CloudTemplate {
  id: string;
  templateId?: string;
  name: string;
  language: string;
  category?: string;
  status?: string;
  components?: unknown;
  rejectedReason?: string;
  qualityScore?: string;
}

export interface CreateInstancePayload {
  instanceId?: string;
  name: string;
  token: string;
  integration?: 'WHATSAPP-BAILEYS' | 'WHATSAPP-BUSINESS';
  cloud?: CloudCredentialsInput;
  websocketEnable?: string;
  proxy?: {
    host: string;
    port: string;
    username?: string;
    password?: string;
  };
  advancedSettings?: {
    alwaysOnline?: boolean;
    rejectCall?: boolean;
    msgRejectCall?: string;
    readMessages?: boolean;
    ignoreGroups?: boolean;
    ignoreStatus?: boolean;
  };
}

export interface ConnectionState {
  instance: string;
  state: InstanceStatus;
}

export interface InstancesResponse {
  data: RawInstance[];
  message: string;
}

// Per-instance summary from GET /instance/overview/:instanceId (AuthAdmin):
// the account's own profile picture/push name, local contact count, number of
// distinct chats and messages persisted for that instance.
export interface InstanceOverview {
  connected: boolean;
  // Platform of the phone that paired (android/ios/...), from the whatsmeow
  // device store. whatsmeow exposes no phone model string.
  platform?: string;
  businessName?: string;
  profileName?: string;
  profilePicUrl?: string;
  contactsCount: number;
  chatsCount: number;
  messagesCount: number;
}

// Proxy configuration (GET/POST /instance/proxy/:instanceId, AuthAdmin).
// GET never returns the password; it reports `hasPassword` instead, and a save
// with an empty password keeps the stored one.
export interface ProxyConfig {
  protocol?: string;
  host: string;
  port: string;
  username?: string;
  password?: string;
  hasPassword?: boolean;
}

// Result of POST /instance/proxy/:instanceId/test.
export interface ProxyTestResult {
  ok: boolean;
  ip?: string;
  serverIp?: string;
  anonymous: boolean;
  whatsappReachable: boolean;
  latencyMs?: number;
  protocol?: string;
  error?: string;
}

// Per-instance S3 config (GET/POST/DELETE /instance/s3). The secret is never
// returned; `secretKeySet` says whether one is stored.
export interface S3Config {
  enabled: boolean;
  endpoint: string;
  region?: string;
  bucket: string;
  accessKey: string;
  secretKey?: string;
  secretKeySet?: boolean;
  pathStyle?: boolean;
  publicUrl?: string;
  mediaDelivery?: 'base64' | 's3' | 'both';
}

// HMAC configuration status (GET/POST/DELETE /instance/hmac). The key itself is
// returned only once, when generated.
export interface HmacStatus {
  configured: boolean;
  globalFallback: boolean;
}

export interface HmacResponse extends HmacStatus {
  generatedKey?: string;
}

// In-memory verdict from GET /instance/status (health) and inventory.
export interface InstanceHealth {
  instanceId: string;
  connected: boolean;
  reconnecting: boolean;
  verdict: 'ok' | 'reconnecting' | 'looping' | 'stalled' | 'closed' | string;
  reasons: string[];
  observedSince?: string;
  lastEventAt?: string;
  lastInboundAt?: string;
  lastAckAt?: string;
  lastDisconnectAt?: string;
  lastDisconnectReason?: string;
  eventType?: string;
  eventRunningAgeMs?: number;
  reconnectOpens?: number;
  reconnectDrops?: number;
  conflicts?: number;
  blockedReason?: string;
}

export interface CloudAccountEvent {
  id: string;
  instanceId: string;
  field: string;
  eventType?: string;
  payload?: string;
  receivedAt?: string;
}

export interface CloudTemplateInput {
  name?: string;
  language?: string;
  category?: string;
  components?: unknown;
}

export interface EmbeddedAsset {
  businessId: string;
  wabaId: string;
  wabaName?: string;
  phoneNumberId: string;
  displayPhoneNumber?: string;
  verifiedName?: string;
}

export interface EmbeddedStart {
  url: string;
  state: string;
}
