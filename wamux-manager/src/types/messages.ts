/**
 * Message-history types for the manager's Messages screen.
 * Mirrors the JSON returned by GET /chat/chats and GET /chat/history.
 */

export interface ChatSummary {
  chat_jid: string;
  last_message_id: string;
  last_timestamp: string;
  last_message_type: string;
  last_text_content: string;
  last_status: string;
  last_media_url: string;
  last_sender_jid: string;
  last_from_me: boolean;
  message_count: number;
  /**
   * Resolved display name (group subject or saved contact name). Empty when the
   * instance is offline or the name is unknown; the UI then falls back to the
   * number/JID.
   */
  name?: string;
}

/** One row of GET /chat/contacts (the conversation picker). */
export interface ChatContact {
  jid: string;
  name?: string;
  is_group: boolean;
}

export interface HistoryMessage {
  id: string;
  message_id: string;
  instance_id: string;
  timestamp: string;
  status: string;
  source: string;
  chat_jid: string;
  /** Set on the client by applyEdits: the text comes from a later edit. */
  edited?: boolean;
  sender_jid: string;
  message_type: string;
  text_content: string;
  media_url: string;
  media_mimetype: string;
  quoted_message_id: string;
  is_from_me: boolean;
  /** Empty on messages stored before these were recorded. */
  file_name?: string;
  file_size?: number;
  is_forwarded?: boolean;
  page_count?: number;
  /** A small JPEG preview, in base64. */
  file_thumbnail?: string;
}

/** One frame from /ws: `{ queue, payload }`, payload being the event JSON. */
export interface InstanceEvent {
  queue: string;
  event: string;
  data: Record<string, unknown>;
  raw: Record<string, unknown>;
}
