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
}

export interface HistoryMessage {
  id: string;
  message_id: string;
  instance_id: string;
  timestamp: string;
  status: string;
  source: string;
  chat_jid: string;
  sender_jid: string;
  message_type: string;
  text_content: string;
  media_url: string;
  media_mimetype: string;
  quoted_message_id: string;
  is_from_me: boolean;
}

/** One frame from /ws: `{ queue, payload }`, payload being the event JSON. */
export interface InstanceEvent {
  queue: string;
  event: string;
  data: Record<string, unknown>;
  raw: Record<string, unknown>;
}
