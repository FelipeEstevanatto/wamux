/**
 * Message-history API.
 *
 * These endpoints are scoped to one instance, so the instance token is sent in
 * the `apikey` header (the request interceptor leaves it untouched because a
 * value is already present).
 */

import apiClient from './client';
import type { ChatSummary, HistoryMessage } from '@/types/messages';

const asInstance = (instanceToken: string) => ({
  headers: { apikey: instanceToken },
});

/**
 * List conversations with their newest message. GET /chat/chats
 */
export const listChats = async (
  instanceToken: string,
  limit = 50
): Promise<ChatSummary[]> => {
  const response = await apiClient.get<{ message: string; data: ChatSummary[] }>(
    '/chat/chats',
    { ...asInstance(instanceToken), params: { limit } }
  );
  return response.data.data ?? [];
};

/**
 * Read one conversation's stored messages, newest first.
 * `before` (a "YYYY-MM-DD HH:MM:SS" timestamp) pages backwards.
 * GET /chat/history
 */
export const getHistory = async (
  instanceToken: string,
  chat: string,
  limit = 50,
  before?: string
): Promise<HistoryMessage[]> => {
  const response = await apiClient.get<{ message: string; data: HistoryMessage[] }>(
    '/chat/history',
    {
      ...asInstance(instanceToken),
      params: before ? { chat, limit, before } : { chat, limit },
    }
  );
  return response.data.data ?? [];
};

/**
 * Send a text message. POST /send/text
 */
export const sendText = async (
  instanceToken: string,
  payload: { number: string; text: string }
): Promise<unknown> => {
  const response = await apiClient.post<{ message: string; data: unknown }>(
    '/send/text',
    payload,
    asInstance(instanceToken)
  );
  return response.data.data;
};
