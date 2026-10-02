import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  ArrowLeft,
  Check,
  CheckCheck,
  Info,
  MessageSquare,
  Paperclip,
  RefreshCw,
  Search,
  SendHorizontal,
  Users,
  Wifi,
  WifiOff,
  X,
} from 'lucide-react';
import { toast } from 'sonner';

import { Button, Input, Skeleton } from '@/components/ui';
import { useI18n } from '@/i18n/I18nContext';
import useInstances from '@/hooks/useInstances';
import useInstanceEvents from '@/hooks/useInstanceEvents';
import * as messagesApi from '@/services/api/messages';
import { fetchServerStats } from '@/services/api/server';
import type { ChatSummary, HistoryMessage } from '@/types/messages';
import { cn } from '@/utils/cn';

const PAGE_SIZE = 50;

type ChatKind = 'contact' | 'group' | 'channel';
type TabId = 'all' | ChatKind;

// Feature flags reported by GET /server/stats. When mediaLocal/history are off
// there is nothing stored to preview or read back, and the UI says so.
interface FeatureFlags {
  historyEnabled: boolean;
  mediaLocal: boolean;
  webhookFiles: boolean;
  loaded: boolean;
}

function jidUser(jid: string): string {
  if (!jid) return '—';
  return jid.split('@')[0].split(':')[0] || jid;
}

function chatKind(jid: string): ChatKind {
  if (jid.endsWith('@g.us')) return 'group';
  if (jid.endsWith('@newsletter')) return 'channel';
  return 'contact';
}

function chatTitle(
  jid: string,
  t: (key: string, vars?: Record<string, string | number>) => string
): string {
  if (!jid) return t('messages.chatFallback');
  if (jid.endsWith('@g.us')) return t('messages.chatGroup', { id: jidUser(jid) });
  if (jid.endsWith('@newsletter')) return t('messages.chatChannel', { id: jidUser(jid) });
  if (jid.endsWith('@lid')) return t('messages.chatLid', { id: jidUser(jid) });
  return jidUser(jid);
}

// displayTitle prefers the server-resolved name (group subject / contact name),
// falling back to the number/JID when the name is unknown or the instance is
// offline.
function displayTitle(
  chat: ChatSummary | undefined,
  jid: string,
  t: (key: string, vars?: Record<string, string | number>) => string
): string {
  return chat?.name?.trim() || chatTitle(jid, t);
}

// toChatJid turns a typed value into a chat JID: a full JID is kept, a bare
// number becomes a phone-number JID.
function toChatJid(raw: string): string {
  const value = raw.trim();
  if (!value) return '';
  if (value.includes('@')) return value;
  return value.replace(/[^0-9]/g, '') + '@s.whatsapp.net';
}

function fileKind(file: File): 'image' | 'video' | 'audio' | 'document' {
  if (file.type.startsWith('image/')) return 'image';
  if (file.type.startsWith('video/')) return 'video';
  if (file.type.startsWith('audio/')) return 'audio';
  return 'document';
}

function parseTs(ts: string): Date | null {
  if (!ts) return null;
  const d = new Date(ts.includes('T') ? ts : ts.replace(' ', 'T'));
  return Number.isNaN(d.getTime()) ? null : d;
}

function formatShort(ts: string): string {
  const d = parseTs(ts);
  return d ? d.toLocaleString('pt-BR', { day: '2-digit', month: '2-digit' }) : '';
}

function formatTime(ts: string): string {
  const d = parseTs(ts);
  return d ? d.toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' }) : '';
}

// A small, theme-safe palette for group sender names, picked deterministically
// from the name so the same person keeps the same colour across renders.
const SENDER_COLORS = [
  'text-rose-500',
  'text-emerald-500',
  'text-sky-500',
  'text-amber-500',
  'text-violet-500',
  'text-teal-500',
  'text-pink-500',
  'text-indigo-500',
];

function senderColor(name: string): string {
  let h = 0;
  for (let i = 0; i < name.length; i++) h = (h * 31 + name.charCodeAt(i)) >>> 0;
  return SENDER_COLORS[h % SENDER_COLORS.length];
}

// mergeMessages merges two newest-first pages by message_id and re-sorts
// descending, so a live refresh can update a message's status without
// duplicating it or dropping pages already loaded.
function mergeMessages(
  current: HistoryMessage[],
  incoming: HistoryMessage[]
): HistoryMessage[] {
  const byId = new Map<string, HistoryMessage>();
  for (const m of [...current, ...incoming]) byId.set(m.message_id, m);
  return Array.from(byId.values()).sort((a, b) => {
    if (a.timestamp !== b.timestamp) return a.timestamp < b.timestamp ? 1 : -1;
    return a.id < b.id ? 1 : -1;
  });
}

function StatusTick({ status }: { status: string }) {
  if (status === 'Read') return <CheckCheck className="h-3.5 w-3.5 text-sky-400" />;
  if (status === 'Delivered') return <CheckCheck className="h-3.5 w-3.5 opacity-70" />;
  return <Check className="h-3.5 w-3.5 opacity-70" />;
}

function humanSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${Math.round(bytes / 1024)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

// Resolved object URLs are cached so re-renders and re-mounts do not refetch.
const mediaObjectUrlCache = new Map<string, string>();

// Profile-picture URLs are cached the same way. The empty string is a cached
// "no picture", so a contact without one is not re-requested every render.
const avatarCache = new Map<string, string>();

function isAbsoluteUrl(url: string): boolean {
  return url.startsWith('http://') || url.startsWith('https://');
}

function useChatAvatar(instanceToken: string, jid: string, enabled: boolean): string {
  const user = jidUser(jid);
  const key = `${instanceToken}|${user}`;
  // Resolve the cached value during render (and re-resolve when the target
  // changes) so a known avatar paints on the first pass — no effect-driven
  // extra render, which is what the lint rule is asking for.
  const [src, setSrc] = useState(() => avatarCache.get(key) ?? '');
  const [lastKey, setLastKey] = useState(key);
  if (key !== lastKey) {
    setLastKey(key);
    setSrc(avatarCache.get(key) ?? '');
  }

  useEffect(() => {
    if (!enabled || !instanceToken || !user || user === '—') return;
    if (avatarCache.has(key)) return; // known hit (or a cached miss): don't refetch
    let active = true;
    messagesApi
      .getAvatar(instanceToken, user)
      .then((url) => {
        avatarCache.set(key, url);
        if (active) setSrc(url);
      })
      .catch(() => {
        // Cache the miss so a contact without a picture is not retried.
        avatarCache.set(key, '');
      });
    return () => {
      active = false;
    };
  }, [key, instanceToken, user, enabled]);

  return src;
}

/**
 * A WhatsApp-like round avatar: the profile picture when there is one, the
 * group/initial glyph otherwise. Groups and channels always show the glyph
 * (their picture is not resolvable through the user-avatar endpoint).
 */
function ChatAvatar({
  jid,
  token,
  label,
  size = 40,
}: {
  jid: string;
  token: string;
  label?: string;
  size?: number;
}) {
  const kind = chatKind(jid);
  const isGroupLike = kind !== 'contact';
  const src = useChatAvatar(token, jid, !isGroupLike);
  const [failed, setFailed] = useState(false);

  const initial = (label?.trim() || jidUser(jid)).charAt(0).toUpperCase() || '?';

  return (
    <span
      className="inline-flex shrink-0 items-center justify-center overflow-hidden rounded-full bg-muted text-xs font-semibold text-muted-foreground"
      style={{ width: size, height: size }}
      aria-hidden="true"
    >
      {isGroupLike ? (
        <Users className="h-5 w-5" />
      ) : src && !failed ? (
        <img
          src={src}
          alt=""
          className="h-full w-full object-cover"
          loading="lazy"
          onError={() => setFailed(true)}
        />
      ) : (
        <span>{initial}</span>
      )}
    </span>
  );
}

/**
 * Resolve a message's `media_url` into something an <img>/<video>/<a> can use.
 * Absolute object-store URLs are used as-is; API paths (`/chat/media/<id>`) are
 * fetched with the instance token and turned into a blob URL.
 */
function useMediaSrc(mediaUrl: string, token: string): string {
  const cacheKey = `${token}|${mediaUrl}`;
  // Absolute URLs and cache hits are known synchronously, so they render without
  // an effect round-trip.
  const immediate = isAbsoluteUrl(mediaUrl)
    ? mediaUrl
    : mediaObjectUrlCache.get(cacheKey) ?? '';
  const [src, setSrc] = useState(immediate);
  const [lastKey, setLastKey] = useState(cacheKey);
  if (cacheKey !== lastKey) {
    setLastKey(cacheKey);
    setSrc(isAbsoluteUrl(mediaUrl) ? mediaUrl : mediaObjectUrlCache.get(cacheKey) ?? '');
  }

  useEffect(() => {
    // Nothing to do when there is no media, the URL is absolute, or it is cached
    // (including an absolute URL already handled above).
    if (!mediaUrl || isAbsoluteUrl(mediaUrl) || !token) return;
    if (mediaObjectUrlCache.has(cacheKey)) return;

    let active = true;
    messagesApi
      .fetchMediaObjectUrl(token, mediaUrl)
      .then((url) => {
        if (!active) {
          URL.revokeObjectURL(url);
          return;
        }
        mediaObjectUrlCache.set(cacheKey, url);
        setSrc(url);
      })
      .catch(() => {
        if (active) setSrc('');
      });

    return () => {
      active = false;
    };
  }, [cacheKey, mediaUrl, token]);

  return src;
}

function MediaContent({
  message,
  token,
  mediaLocal,
}: {
  message: HistoryMessage;
  token: string;
  mediaLocal: boolean;
}) {
  const { t } = useI18n();
  const src = useMediaSrc(message.media_url || '', token);

  if (!message.media_url) {
    const label = message.message_type || t('messages.mediaFallback');
    // Explain why a file shows only a placeholder instead of a preview.
    return (
      <p className="italic opacity-80">
        [{label}]
        {!mediaLocal && (
          <span className="ml-1 not-italic opacity-70" title="MEDIA_LOCAL_STORE=false">
            {t('messages.mediaNotStored')}
          </span>
        )}
      </p>
    );
  }
  if (!src) {
    return (
      <p className="italic opacity-80">
        {t('messages.mediaLoading', { type: message.message_type || t('messages.mediaFallback') })}
      </p>
    );
  }

  const mime = message.media_mimetype || '';
  if (mime.startsWith('image/')) {
    return (
      <img
        src={src}
        alt={message.message_type}
        className="mb-1 max-h-72 rounded-md"
        loading="lazy"
      />
    );
  }
  if (mime.startsWith('video/')) {
    return <video src={src} controls className="mb-1 max-h-72 rounded-md" />;
  }
  if (mime.startsWith('audio/')) {
    return <audio src={src} controls className="mb-1 w-64" />;
  }
  return (
    <a
      href={src}
      target="_blank"
      rel="noreferrer noopener"
      className="mb-1 inline-block underline"
    >
      {t('messages.openFile', { type: message.message_type || t('messages.fileFallback') })}
    </a>
  );
}

function Bubble({
  message,
  token,
  mediaLocal,
  senderName,
  showSender,
}: {
  message: HistoryMessage;
  token: string;
  mediaLocal: boolean;
  senderName?: string;
  showSender: boolean;
}) {
  const { t } = useI18n();
  const mine = message.is_from_me;
  const isMedia =
    !!message.media_url ||
    ['image', 'video', 'audio', 'document', 'sticker'].includes(message.message_type);
  // Prefer the resolved name; fall back to the sender number, then a generic
  // label so a group message always shows who wrote it.
  const label = senderName?.trim() || jidUser(message.sender_jid) || t('messages.unknownSender');
  return (
    <div className={`flex ${mine ? 'justify-end' : 'justify-start'}`}>
      <div
        className={`max-w-[75%] rounded-lg px-3 py-2 text-sm shadow-xs ${
          mine ? 'bg-primary text-primary-foreground' : 'bg-muted text-foreground'
        }`}
      >
        {showSender && !mine && (
          <p className={`mb-0.5 text-[11px] font-semibold ${senderColor(label)}`}>
            {label}
          </p>
        )}
        {isMedia && <MediaContent message={message} token={token} mediaLocal={mediaLocal} />}
        {message.text_content && (
          <p className="break-words whitespace-pre-wrap">{message.text_content}</p>
        )}
        <span
          className={`mt-1 flex items-center justify-end gap-1 text-[10px] ${
            mine ? 'text-primary-foreground/70' : 'text-muted-foreground'
          }`}
        >
          {formatTime(message.timestamp)}
          {mine && <StatusTick status={message.status} />}
        </span>
      </div>
    </div>
  );
}

export default function Messages() {
  const { t } = useI18n();
  const {
    instances,
    isLoading: instancesLoading,
    fetchInstances,
  } = useInstances();
  const connected = useMemo(
    () => instances.filter((i) => i.connected),
    [instances]
  );

  const [selectedInstanceId, setInstanceId] = useState('');
  // Fall back to the first connected instance as a derived value, so no effect
  // has to set it (and the very first render already targets an instance).
  const instanceId = selectedInstanceId || connected[0]?.id || '';
  const instance = useMemo(
    () => instances.find((i) => i.id === instanceId),
    [instances, instanceId]
  );
  const token = instance?.apikey ?? '';
  // Live events are only published to /ws when the instance's WebSocket option
  // is enabled; otherwise the socket connects but stays silent.
  const wsEnabled =
    instance?.websocketEnable === 'enabled' || instance?.websocketEnable === 'true';

  const [chats, setChats] = useState<ChatSummary[]>([]);
  // The token whose conversation list has already loaded. "Loading" is derived
  // from this rather than stored, so the fetch effect never has to set state
  // synchronously.
  const [chatsLoadedFor, setChatsLoadedFor] = useState<string | null>(null);
  const [refreshing, setRefreshing] = useState(false);
  const [chatQuery, setChatQuery] = useState('');
  const [tab, setTab] = useState<TabId>('all');

  const [selectedChat, setSelectedChat] = useState('');
  const [messages, setMessages] = useState<HistoryMessage[]>([]);
  const [messagesLoading, setMessagesLoading] = useState(false);
  const [olderLoading, setOlderLoading] = useState(false);
  const [hasMore, setHasMore] = useState(true);
  // Sender names are keyed by the chat they belong to, so a stale map from the
// previous conversation is never rendered while the next one loads.
  const [senderNames, setSenderNames] = useState<{
    chat: string;
    names: Record<string, string>;
  }>({ chat: '', names: {} });

  const [draft, setDraft] = useState('');
  const [attachFile, setAttachFile] = useState<File | null>(null);
  const [attachPreview, setAttachPreview] = useState('');
  const [sending, setSending] = useState(false);
  const [newNumber, setNewNumber] = useState('');
  const [mobileThread, setMobileThread] = useState(false);

  // Contacts picker + on-demand history recovery. Names come from the stored
  // conversations (GET /chat/contacts); the WhatsApp store list is the fallback.
  const [contacts, setContacts] = useState<{ jid: string; name: string }[]>([]);
  const [contactsLoading, setContactsLoading] = useState(false);
  const [showContacts, setShowContacts] = useState(false);
  const [syncing, setSyncing] = useState(false);

  const [flags, setFlags] = useState<FeatureFlags>({
    historyEnabled: true,
    mediaLocal: true,
    webhookFiles: true,
    loaded: false,
  });

  const fileInputRef = useRef<HTMLInputElement>(null);
  const objectUrlRef = useRef('');

  // setAttachment swaps the pending file, revoking the previous preview URL.
  const setAttachment = (file: File | null) => {
    if (objectUrlRef.current) {
      URL.revokeObjectURL(objectUrlRef.current);
      objectUrlRef.current = '';
    }
    if (file && (file.type.startsWith('image/') || file.type.startsWith('video/'))) {
      objectUrlRef.current = URL.createObjectURL(file);
    }
    setAttachFile(file);
    setAttachPreview(objectUrlRef.current);
  };

  useEffect(
    () => () => {
      if (objectUrlRef.current) URL.revokeObjectURL(objectUrlRef.current);
    },
    []
  );

  // Populate the instance list when this page is opened directly. The store is
  // otherwise only filled by the Instances/Dashboard pages, so a direct load of
  // /manager/messages would otherwise report "no connected instance".
  useEffect(() => {
    void fetchInstances();
  }, [fetchInstances]);

  // Read the server's feature flags once: they decide whether history readback
  // and local media previews are available, and whether we warn about it.
  useEffect(() => {
    fetchServerStats()
      .then((stats) => {
        const s = stats.storage;
        setFlags({
          historyEnabled: s?.historyEnabled ?? true,
          mediaLocal: s?.mediaLocal ?? true,
          webhookFiles: s?.webhookFiles ?? true,
          loaded: true,
        });
      })
      .catch(() => {
        // Older server without the flags: assume everything is on.
        setFlags({ historyEnabled: true, mediaLocal: true, webhookFiles: true, loaded: true });
      });
  }, []);

  const loadChats = useCallback(async () => {
    if (!token) return;
    try {
      setChats(await messagesApi.listChats(token));
    } catch {
      toast.error(t('messages.errorLoadChats'));
    } finally {
      // Marks the list as loaded for this token, which clears the derived
      // "loading" state below.
      setChatsLoadedFor(token);
    }
  }, [token, t]);

  const loadThread = useCallback(
    async (chat: string) => {
      if (!token || !chat) return;
      setMessagesLoading(true);
      setHasMore(true);
      try {
        const page = await messagesApi.getHistory(token, chat, PAGE_SIZE);
        setMessages(page);
        setHasMore(page.length >= PAGE_SIZE);
      } catch {
        toast.error(t('messages.errorLoadHistory'));
      } finally {
        setMessagesLoading(false);
      }
    },
    [token, t]
  );

  // Reset the conversation view when the selected instance changes, during render
// rather than in an effect (the "adjust state when an input changes" pattern).
  // The list itself is (re)loaded by the effect below.
  const [lastToken, setLastToken] = useState(token);
  if (token !== lastToken) {
    setLastToken(token);
    setChats([]);
    setSelectedChat('');
    setMessages([]);
    setMobileThread(false);
  }

  // Load the conversation list for the current instance. Whether it is still
  // loading is derived from `chatsLoadedFor`, so this effect only fetches.
  useEffect(() => {
    // loadChats only writes state after its `await` completes — this is the
    // standard fetch-on-deps-change pattern, not a synchronous render cascade.
    // The rule's static analysis is conservative about async loaders.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    void loadChats();
  }, [token, loadChats]);

  const chatsLoading = !!token && chatsLoadedFor !== token;

  // Resolve the authors of the open conversation so a group thread can label
  // who sent each message. Only meaningful for groups/channels (a 1:1 chat's
  // sender is the peer already shown in the header).
  useEffect(() => {
    if (!token || !selectedChat) return;
    // A 1:1 chat's sender is the peer already shown in the header; only groups
    // and channels need per-message labels.
    if (chatKind(selectedChat) === 'contact') return;
    let active = true;
    messagesApi
      .getSenderNames(token, selectedChat)
      .then((names) => {
        if (active) setSenderNames({ chat: selectedChat, names });
      })
      .catch(() => {
        /* names are best-effort; fall back to the raw number */
      });
    return () => {
      active = false;
    };
  }, [token, selectedChat]);

  const refreshThread = useCallback(async () => {
    if (!token || !selectedChat) return;
    try {
      const page = await messagesApi.getHistory(token, selectedChat, PAGE_SIZE);
      setMessages((prev) => mergeMessages(prev, page));
    } catch {
      // transient; the next event retries
    }
  }, [token, selectedChat]);

  const openChat = (chat: string) => {
    setSelectedChat(chat);
    setMobileThread(true);
    void loadThread(chat);
  };

  const loadOlder = async () => {
    if (!token || !selectedChat || olderLoading || messages.length === 0) return;
    const oldest = messages[messages.length - 1];
    setOlderLoading(true);
    try {
      const page = await messagesApi.getHistory(
        token,
        selectedChat,
        PAGE_SIZE,
        oldest.timestamp
      );
      if (page.length === 0) setHasMore(false);
      else setMessages((prev) => mergeMessages(prev, page));
    } catch {
      toast.error(t('messages.errorLoadOlder'));
    } finally {
      setOlderLoading(false);
    }
  };

  // Live updates are applied by refetching (debounced) rather than parsing each
  // event payload, so the view stays correct whatever the event shape is.
  const refreshTimer = useRef<number | undefined>(undefined);
  const scheduleRefresh = useCallback(
    (withChats: boolean) => {
      if (refreshTimer.current) window.clearTimeout(refreshTimer.current);
      refreshTimer.current = window.setTimeout(() => {
        void refreshThread();
        if (withChats) void loadChats();
      }, 400);
    },
    [refreshThread, loadChats]
  );

  const wsStatus = useInstanceEvents(instance?.id, (event) => {
    if (event.event === 'Message' || event.event === 'SendMessage') {
      scheduleRefresh(true);
    } else if (event.event === 'Receipt') {
      scheduleRefresh(false);
    }
  });

  useEffect(
    () => () => {
      if (refreshTimer.current) window.clearTimeout(refreshTimer.current);
    },
    []
  );

  // Manual refresh with visible feedback: the API answers locally in a few ms,
  // so keep the spinner up for a minimum time (same approach as Instances).
  const manualRefresh = async () => {
    if (refreshing) return;
    setRefreshing(true);
    const startedAt = Date.now();
    await loadChats();
    const elapsed = Date.now() - startedAt;
    if (elapsed < 600) await new Promise((r) => setTimeout(r, 600 - elapsed));
    setRefreshing(false);
  };

  const startConversation = () => {
    const jid = toChatJid(newNumber);
    if (!jid) return;
    setNewNumber('');
    setSelectedChat(jid);
    setMobileThread(true);
    setMessages([]);
    setHasMore(false);
    // Show it in the list straight away, then load any existing history.
    setChats((prev) =>
      prev.some((c) => c.chat_jid === jid)
        ? prev
        : [
            {
              chat_jid: jid,
              last_message_id: '',
              last_timestamp: '',
              last_message_type: '',
              last_text_content: '',
              last_status: '',
              last_media_url: '',
              last_sender_jid: '',
              last_from_me: false,
              message_count: 0,
            },
            ...prev,
          ]
    );
    void loadThread(jid);
  };

  // Load the instance's known contacts on demand for the picker.
  const loadContacts = async () => {
    if (!token) return;
    setContactsLoading(true);
    try {
      const list = await messagesApi.listChatContacts(token);
      setContacts(list.map((c) => ({ jid: c.jid, name: c.name?.trim() || c.jid })));
    } catch {
      toast.error(t('messages.errorLoadContacts'));
    } finally {
      setContactsLoading(false);
    }
  };

  // Ask WhatsApp for historical messages, then reload what we have stored.
  // Uses the newest known message of the selected chat as the cursor.
  const recoverHistory = async () => {
    if (!token || syncing) return;
    if (!selectedChat) {
      toast.error(t('messages.errorNoChatToRecover'));
      return;
    }
    setSyncing(true);
    try {
      const isGroup = selectedChat.endsWith('@g.us');
      const newest = messages[0];
      let messageInfo = {
        Chat: selectedChat,
        IsFromMe: newest?.is_from_me ?? false,
        IsGroup: isGroup,
        ID: newest?.message_id ?? '',
        Timestamp: newest?.timestamp ? newest.timestamp.replace(' ', 'T') + 'Z' : new Date().toISOString(),
      };
      // Without a stored anchor, ask the instance for the newest message id.
      if (!newest) {
        const page = await messagesApi.getHistory(token, selectedChat, 1);
        if (page[0]) {
          messageInfo = {
            Chat: selectedChat,
            IsFromMe: page[0].is_from_me,
            IsGroup: isGroup,
            ID: page[0].message_id,
            Timestamp: page[0].timestamp.replace(' ', 'T') + 'Z',
          };
        }
      }
      await messagesApi.requestHistorySync(token, messageInfo, PAGE_SIZE);
      toast.success(t('messages.syncRequested'));
    } catch {
      toast.error(t('messages.errorSync'));
    } finally {
      setSyncing(false);
    }
  };

  const send = async () => {
    const text = draft.trim();
    if (!token || !selectedChat || sending) return;
    if (!text && !attachFile) return;
    setSending(true);
    try {
      if (attachFile) {
        await messagesApi.sendMedia(token, {
          number: selectedChat,
          type: fileKind(attachFile),
          caption: text || undefined,
          filename: attachFile.name,
          file: attachFile,
        });
        setAttachment(null);
      } else {
        await messagesApi.sendText(token, { number: selectedChat, text });
      }
      setDraft('');
      await refreshThread();
      void loadChats();
    } catch {
      toast.error(t('messages.errorSend'));
    } finally {
      setSending(false);
    }
  };

  // Chats matching the text filter, then the active tab.
  const filteredChats = useMemo(() => {
    const q = chatQuery.trim().toLowerCase();
    if (!q) return chats;
    return chats.filter(
      (c) =>
        c.chat_jid.toLowerCase().includes(q) ||
        (c.name ?? '').toLowerCase().includes(q)
    );
  }, [chats, chatQuery]);

  // Tab counts reflect the text filter, so a badge always matches what the tab
  // would show.
  const counts = useMemo(() => {
    const c = { all: filteredChats.length, contact: 0, group: 0, channel: 0 };
    for (const chat of filteredChats) c[chatKind(chat.chat_jid)] += 1;
    return c;
  }, [filteredChats]);

  const tabs = useMemo(
    () =>
      [
        { id: 'all' as TabId, label: t('messages.tabAll'), count: counts.all },
        { id: 'contact' as TabId, label: t('messages.sectionContacts'), count: counts.contact },
        { id: 'group' as TabId, label: t('messages.sectionGroups'), count: counts.group },
        { id: 'channel' as TabId, label: t('messages.sectionChannels'), count: counts.channel },
      ].filter((tabDef) => tabDef.id === 'all' || tabDef.count > 0),
    [counts, t]
  );

  // The tab actually shown: if the active category has no rows (e.g. a filter
  // excluded them all), fall back to "all" without a state write, so the list is
  // never mysteriously empty.
  const activeTab: TabId = tabs.some((tabDef) => tabDef.id === tab) ? tab : 'all';

  const visibleChats = useMemo(() => {
    if (activeTab === 'all') return filteredChats;
    return filteredChats.filter((c) => chatKind(c.chat_jid) === activeTab);
  }, [filteredChats, activeTab]);

  const ordered = useMemo(() => [...messages].reverse(), [messages]);
  const activeChat = useMemo(
    () => chats.find((c) => c.chat_jid === selectedChat),
    [chats, selectedChat]
  );
  const activeKind = selectedChat ? chatKind(selectedChat) : 'contact';
  const showSenderNames = activeKind !== 'contact';
  // Names only apply to the currently open chat (see the state shape).
  const namesForChat =
    senderNames.chat === selectedChat ? senderNames.names : {};
  // Resolved name when known, else the number/JID (activeChat may be undefined
  // for a conversation started from a typed number before it is in the list).
  const activeTitle = displayTitle(activeChat, selectedChat, t);

  const wsBadge = (
    <span
      className={`inline-flex items-center gap-1 text-xs ${
        wsStatus === 'open'
          ? 'text-green-600'
          : wsStatus === 'connecting'
            ? 'text-amber-600'
            : 'text-red-600'
      }`}
      title={
        wsStatus === 'open'
          ? t('messages.wsConnectedTitle')
          : wsStatus === 'connecting'
            ? t('messages.wsConnectingTitle')
            : t('messages.wsOfflineTitle')
      }
    >
      {wsStatus === 'open' ? (
        <Wifi className="h-3.5 w-3.5" />
      ) : (
        <WifiOff className="h-3.5 w-3.5" />
      )}
      {wsStatus === 'open'
        ? t('messages.wsConnected')
        : wsStatus === 'connecting'
          ? t('messages.wsConnecting')
          : t('messages.wsOffline')}
    </span>
  );

  if (instancesLoading && instances.length === 0) {
    return (
      <div className="flex h-full items-center justify-center p-8 text-sm text-muted-foreground">
        {t('messages.loadingInstances')}
      </div>
    );
  }

  if (connected.length === 0) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-3 p-8 text-center">
        <MessageSquare className="h-10 w-10 text-muted-foreground" />
        <p className="text-sm text-muted-foreground">
          {t('messages.noConnectedInstance')}
        </p>
      </div>
    );
  }

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex flex-wrap items-center gap-3 border-b border-border p-4">
        <h1 className="text-xl font-semibold">{t('messages.title')}</h1>
        <select
          value={instanceId}
          onChange={(e) => setInstanceId(e.target.value)}
          className="h-9 rounded-md border border-input bg-background px-3 text-sm"
        >
          {connected.map((i) => (
            <option key={i.id} value={i.id}>
              {i.instanceName}
              {i.owner ? ` (${i.owner})` : ''}
            </option>
          ))}
        </select>
        <Button variant="outline" size="sm" onClick={() => void manualRefresh()} disabled={refreshing}>
          <RefreshCw className={refreshing ? 'animate-spin' : ''} /> {t('messages.refresh')}
        </Button>
        {wsEnabled ? (
          wsBadge
        ) : (
          <span
            className="inline-flex items-center gap-1 text-xs text-amber-600"
            title={t('messages.wsDisabledTitle')}
          >
            <WifiOff className="h-3.5 w-3.5" /> {t('messages.wsDisabled')}
          </span>
        )}
      </div>

      {flags.loaded && !flags.mediaLocal && (
        <div className="flex items-start gap-2 border-b border-border bg-muted/50 px-4 py-2 text-xs text-muted-foreground">
          <Info className="mt-0.5 h-3.5 w-3.5 shrink-0" />
          <span>
            {t('messages.mediaLocalDisabledPre')}
            <strong>{t('messages.mediaLocalDisabledStrong')}</strong>
            {' '}(<code>MEDIA_LOCAL_STORE=false</code>
            {!flags.historyEnabled && (
              <>
                {' '}{t('messages.and')} <code>DATABASE_SAVE_MESSAGES=false</code>
              </>
            )}
            {t('messages.mediaLocalDisabledMid')}<code>MEDIA_LOCAL_STORE=true</code>
            {!flags.historyEnabled && (
              <>
                {' '}{t('messages.and')} <code>DATABASE_SAVE_MESSAGES=true</code>
              </>
            )}
            {t('messages.mediaLocalDisabledEnd')}
          </span>
        </div>
      )}
      {flags.loaded && !flags.historyEnabled && flags.mediaLocal && (
        <div className="flex items-start gap-2 border-b border-border bg-muted/50 px-4 py-2 text-xs text-muted-foreground">
          <Info className="mt-0.5 h-3.5 w-3.5 shrink-0" />
          <span>
            {t('messages.historyDisabledPre')}
            <strong>{t('messages.historyDisabledStrong')}</strong>
            {' '}(<code>DATABASE_SAVE_MESSAGES=false</code>{t('messages.historyDisabledMid')}
            <strong>{t('messages.recover')}</strong>{t('messages.historyDisabledEnd')}
            <code>DATABASE_SAVE_MESSAGES=true</code>{t('messages.historyDisabledSuffix')}
          </span>
        </div>
      )}

      <div className="grid min-h-0 flex-1 lg:grid-cols-[340px_1fr]">
        {/* Conversation list */}
        <aside
          className={`min-h-0 flex-col border-r border-border ${
            mobileThread ? 'hidden lg:flex' : 'flex'
          }`}
        >
          <div className="space-y-2 border-b border-border p-3">
            <div className="relative">
              <Search className="pointer-events-none absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
              <Input
                className="pl-8 pr-8"
                placeholder={t('messages.filterPlaceholder')}
                value={chatQuery}
                onChange={(e) => setChatQuery(e.target.value)}
              />
              {chatQuery && (
                <button
                  type="button"
                  onClick={() => setChatQuery('')}
                  aria-label={t('messages.clearFilter')}
                  title={t('messages.clearFilter')}
                  className="absolute right-2 top-1/2 -translate-y-1/2 rounded-sm p-0.5 text-muted-foreground transition-colors hover:text-foreground"
                >
                  <X className="h-3.5 w-3.5" />
                </button>
              )}
            </div>
            <div className="flex gap-2">
              <Input
                placeholder={t('messages.newChatPlaceholder')}
                value={newNumber}
                onChange={(e) => setNewNumber(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') {
                    e.preventDefault();
                    startConversation();
                  }
                }}
              />
              <Button
                size="sm"
                variant="outline"
                onClick={startConversation}
                disabled={!newNumber.trim()}
              >
                {t('messages.open')}
              </Button>
            </div>
            <div className="flex gap-2">
              <Button
                size="sm"
                variant="outline"
                className="flex-1"
                onClick={() => {
                  const next = !showContacts;
                  setShowContacts(next);
                  if (next && contacts.length === 0) void loadContacts();
                }}
              >
                {t('messages.contacts')}
              </Button>
              <Button
                size="sm"
                variant="outline"
                className="flex-1"
                onClick={() => void recoverHistory()}
                disabled={syncing}
                title={t('messages.recoverTitle')}
              >
                <RefreshCw className={syncing ? 'animate-spin' : ''} /> {t('messages.recover')}
              </Button>
            </div>
            {showContacts && (
              <div className="max-h-56 overflow-y-auto rounded-md border border-border">
                {contactsLoading ? (
                  <p className="p-2 text-xs text-muted-foreground">
                    {t('messages.loadingContacts')}
                  </p>
                ) : contacts.length === 0 ? (
                  <p className="p-2 text-xs text-muted-foreground">
                    {t('messages.noContacts')}
                  </p>
                ) : (
                  contacts.map((c) => (
                    <button
                      key={c.jid}
                      type="button"
                      className="block w-full truncate px-2 py-1.5 text-left text-sm hover:bg-muted"
                      onClick={() => {
                        setNewNumber(c.jid);
                        setShowContacts(false);
                      }}
                      title={c.jid}
                    >
                      {c.name}
                    </button>
                  ))
                )}
              </div>
            )}
          </div>

          {/* Category tabs (contacts / groups / channels) with counts, like the
              WhatsApp chat list. */}
          <div className="flex flex-wrap gap-1 border-b border-border p-2">
            {tabs.map((tabDef) => (
              <button
                key={tabDef.id}
                type="button"
                onClick={() => setTab(tabDef.id)}
                className={cn(
                  'inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs font-medium transition-colors',
                  activeTab === tabDef.id
                    ? 'bg-primary text-primary-foreground'
                    : 'text-muted-foreground hover:bg-muted'
                )}
              >
                {tabDef.label}
                <span
                  className={cn(
                    'rounded-full px-1.5 text-[10px] leading-4',
                    activeTab === tabDef.id
                      ? 'bg-primary-foreground/20'
                      : 'bg-foreground/10'
                  )}
                >
                  {tabDef.count}
                </span>
              </button>
            ))}
          </div>

          <div className="min-h-0 flex-1 overflow-y-auto">
            {chatsLoading && chats.length === 0 ? (
              <div className="space-y-2 p-3">
                {Array.from({ length: 6 }).map((_, i) => (
                  <Skeleton key={i} className="h-14 w-full" />
                ))}
              </div>
            ) : chats.length === 0 ? (
              <p className="p-4 text-sm text-muted-foreground">
                {t('messages.noChats')}
              </p>
            ) : visibleChats.length === 0 ? (
              <p className="p-4 text-sm text-muted-foreground">
                {t('messages.noMessagesFiltered')}
              </p>
            ) : (
              visibleChats.map((c) => {
                const title = displayTitle(c, c.chat_jid, t);
                // Show the phone number as a subtitle when the row displays a
                // real name above it (a 1:1 contact whose name was resolved).
                const number = chatKind(c.chat_jid) === 'contact' ? jidUser(c.chat_jid) : '';
                const showNumber = !!number && number !== title;
                return (
                  <button
                    key={c.chat_jid}
                    onClick={() => openChat(c.chat_jid)}
                    className={`flex w-full items-center gap-3 border-b border-border px-3 py-2 text-left hover:bg-accent ${
                      selectedChat === c.chat_jid ? 'bg-accent' : ''
                    }`}
                  >
                    <ChatAvatar jid={c.chat_jid} token={token} label={title} />
                    <div className="min-w-0 flex-1">
                      <span className="flex items-center justify-between gap-2">
                        <span className="truncate text-sm font-medium">{title}</span>
                        <span className="shrink-0 text-[11px] text-muted-foreground">
                          {formatShort(c.last_timestamp)}
                        </span>
                      </span>
                      {showNumber && (
                        <span className="block truncate text-[10px] text-muted-foreground">
                          {number}
                        </span>
                      )}
                      <span className="flex items-center justify-between gap-2">
                        <span className="truncate text-xs text-muted-foreground">
                          {c.last_from_me ? t('messages.youPrefix') : ''}
                          {c.last_text_content ||
                            (c.last_message_type ? `[${c.last_message_type}]` : '')}
                        </span>
                        <span className="shrink-0 text-[10px] text-muted-foreground">
                          {t('messages.messageCount', { count: c.message_count })}
                        </span>
                      </span>
                    </div>
                  </button>
                );
              })
            )}
          </div>
        </aside>

        {/* Thread */}
        <section
          className={`min-h-0 flex-col ${mobileThread ? 'flex' : 'hidden lg:flex'}`}
        >
          {!selectedChat ? (
            <div className="flex flex-1 items-center justify-center p-8 text-sm text-muted-foreground">
              {t('messages.selectChat')}
            </div>
          ) : (
            <>
              <div className="flex items-center gap-3 border-b border-border p-3">
                <Button
                  variant="ghost"
                  size="icon"
                  className="lg:hidden"
                  onClick={() => setMobileThread(false)}
                >
                  <ArrowLeft />
                </Button>
                <ChatAvatar jid={selectedChat} token={token} label={activeTitle} size={36} />
                <div className="min-w-0">
                  <p className="truncate text-sm font-medium">{activeTitle}</p>
                  <p className="truncate text-xs text-muted-foreground">
                    {[
                      activeKind === 'contact' ? jidUser(selectedChat) : '',
                      activeChat
                        ? t('messages.messageCount', { count: activeChat.message_count })
                        : '',
                    ]
                      .filter(Boolean)
                      .join(' · ')}
                  </p>
                </div>
              </div>

              <div className="min-h-0 flex-1 overflow-y-auto p-4">
                {hasMore && messages.length > 0 && (
                  <div className="mb-3 flex justify-center">
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => void loadOlder()}
                      disabled={olderLoading}
                    >
                      {olderLoading ? t('common.loading') : t('messages.loadOlder')}
                    </Button>
                  </div>
                )}
                {messagesLoading ? (
                  <div className="space-y-2">
                    {Array.from({ length: 8 }).map((_, i) => (
                      <Skeleton key={i} className="h-10 w-2/3" />
                    ))}
                  </div>
                ) : ordered.length === 0 ? (
                  <p className="py-10 text-center text-sm text-muted-foreground">
                    {t('messages.noMessages')}
                  </p>
                ) : (
                  <div className="flex flex-col gap-2">
                    {ordered.map((m) => (
                      <Bubble
                        key={m.message_id}
                        message={m}
                        token={token}
                        mediaLocal={flags.mediaLocal}
                        showSender={showSenderNames}
                        senderName={namesForChat[jidUser(m.sender_jid)]}
                      />
                    ))}
                  </div>
                )}
              </div>

              {attachFile && (
                <div className="flex items-center gap-3 border-t border-border p-3">
                  {attachPreview ? (
                    attachFile.type.startsWith('video/') ? (
                      <video
                        src={attachPreview}
                        className="h-14 w-14 rounded-md object-cover"
                        muted
                      />
                    ) : (
                      <img
                        src={attachPreview}
                        alt={attachFile.name}
                        className="h-14 w-14 rounded-md object-cover"
                      />
                    )
                  ) : (
                    <div className="flex h-14 w-14 items-center justify-center rounded-md bg-muted">
                      <Paperclip className="h-5 w-5 text-muted-foreground" />
                    </div>
                  )}
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-xs font-medium">{attachFile.name}</p>
                    <p className="text-[11px] text-muted-foreground">
                      {humanSize(attachFile.size)}
                    </p>
                  </div>
                  <button onClick={() => setAttachment(null)} title={t('messages.removeAttachment')}>
                    <X className="h-4 w-4" />
                  </button>
                </div>
              )}
              <div className="flex items-end gap-2 border-t border-border p-3">
                <input
                  ref={fileInputRef}
                  type="file"
                  className="hidden"
                  onChange={(e) => setAttachment(e.target.files?.[0] ?? null)}
                />
                <Button
                  variant="outline"
                  size="icon"
                  onClick={() => fileInputRef.current?.click()}
                  title={t('messages.attachTitle')}
                >
                  <Paperclip />
                </Button>
                <Input
                  value={draft}
                  onChange={(e) => setDraft(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter' && !e.shiftKey) {
                      e.preventDefault();
                      if (!sending) void send();
                    }
                  }}
                  placeholder={t('messages.messagePlaceholder')}
                />
                <Button
                  onClick={() => void send()}
                  disabled={sending || (!draft.trim() && !attachFile)}
                >
                  <SendHorizontal /> {t('messages.send')}
                </Button>
              </div>
            </>
          )}
        </section>
      </div>
    </div>
  );
}