import {
  Fragment,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ClipboardEvent,
  type CSSProperties,
  type DragEvent,
} from 'react';
import {
  type InfiniteData,
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query';
import {
  AlertCircle,
  ArrowDown,
  ArrowLeft,
  Check,
  CheckCheck,
  Download,
  FileText,
  FlaskConical,
  History,
  Info,
  Loader2,
  Maximize2,
  MessageSquare,
  MessageSquarePlus,
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
import NewChatDialog from '@/components/messages/NewChatDialog';
import MediaViewer from '@/components/messages/MediaViewer';
import TestMessageModal from '@/components/instances/TestMessageModal';
import { useI18n } from '@/i18n/I18nContext';
import useInstances from '@/hooks/useInstances';
import useInstanceEvents from '@/hooks/useInstanceEvents';
import { useInView } from '@/hooks/useInView';
import { useMediaSrc } from '@/hooks/useMediaSrc';
import { PANE_MAX, PANE_MIN, useResizablePane } from '@/hooks/useResizablePane';
import * as messagesApi from '@/services/api/messages';
import { fetchServerStats } from '@/services/api/server';
import type { ChatSummary, HistoryMessage } from '@/types/messages';
import { cn } from '@/utils/cn';
import { applyEdits } from '@/utils/messageEdits';

const PAGE_SIZE = 50;

type ChatKind = 'contact' | 'group' | 'channel';
type TabId = 'all' | ChatKind;
type ThreadData = InfiniteData<HistoryMessage[], string | undefined>;

interface SendVars {
  instanceId: string;
  token: string;
  chat: string;
  text: string;
  file: File | null;
}

interface SendContext {
  previousThread?: ThreadData;
  previousChats?: ChatSummary[];
  tempId?: string;
}

// Feature flags reported by GET /server/stats. When mediaLocal/history are off
// there is nothing stored to preview or read back, and the UI says so.
interface FeatureFlags {
  historyEnabled: boolean;
  mediaLocal: boolean;
  webhookFiles: boolean;
  loaded: boolean;
}

const messageKeys = {
  chats: (instanceId: string) => ['message-chats', instanceId] as const,
  thread: (instanceId: string, chat: string) =>
    ['message-thread', instanceId, chat] as const,
  senders: (instanceId: string, chat: string) =>
    ['message-senders', instanceId, chat] as const,
  flags: ['server-flags'] as const,
};

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
  if (jid.endsWith('@newsletter'))
    return t('messages.chatChannel', { id: jidUser(jid) });
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
  return d
    ? d.toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' })
    : '';
}

function formatDay(ts: string): string {
  const d = parseTs(ts);
  return d
    ? d.toLocaleDateString('pt-BR', {
        weekday: 'short',
        day: '2-digit',
        month: 'short',
        year: 'numeric',
      })
    : '';
}

function nowDbTimestamp(): string {
  const d = new Date();
  const p = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(
    d.getHours()
  )}:${p(d.getMinutes())}:${p(d.getSeconds())}`;
}

function addOptimisticMessage(
  data: ThreadData | undefined,
  message: HistoryMessage
): ThreadData | undefined {
  if (!data || data.pages.length === 0) return data;
  return {
    ...data,
    pages: [[message, ...data.pages[0]], ...data.pages.slice(1)],
  };
}

function removeMessage(
  data: ThreadData | undefined,
  messageId: string
): ThreadData | undefined {
  if (!data) return data;
  return {
    ...data,
    pages: data.pages.map((page) =>
      page.filter((m) => m.message_id !== messageId)
    ),
  };
}

function upsertChatSummary(
  chats: ChatSummary[] | undefined,
  chatJid: string,
  patch: Partial<ChatSummary>
): ChatSummary[] | undefined {
  if (!chats) return chats;
  const index = chats.findIndex((c) => c.chat_jid === chatJid);
  if (index >= 0) {
    return chats.map((c, i) => (i === index ? { ...c, ...patch } : c));
  }
  const base: ChatSummary = {
    chat_jid: chatJid,
    last_message_id: '',
    last_timestamp: '',
    last_message_type: '',
    last_text_content: '',
    last_status: '',
    last_media_url: '',
    last_sender_jid: '',
    last_from_me: false,
    message_count: 0,
  };
  return [{ ...base, ...patch }, ...chats];
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

function StatusTick({ status }: { status: string }) {
  if (status === 'Read')
    return <CheckCheck className="h-3.5 w-3.5 text-sky-400" />;
  if (status === 'Delivered')
    return <CheckCheck className="h-3.5 w-3.5 opacity-70" />;
  return <Check className="h-3.5 w-3.5 opacity-70" />;
}

function humanSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${Math.round(bytes / 1024)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

// Profile-picture URLs are cached. The empty string is a cached "no picture",
// so a contact without one is not re-requested every render.
const avatarCache = new Map<string, string>();

function useChatAvatar(
  instanceToken: string,
  jid: string,
  enabled: boolean
): string {
  const user = jidUser(jid);
  const key = `${instanceToken}|${user}`;
  // Resolve the cached value during render so a known avatar paints on the
  // first pass, without an effect-driven extra render.
  const [src, setSrc] = useState(() => avatarCache.get(key) ?? '');
  const [lastKey, setLastKey] = useState(key);
  if (key !== lastKey) {
    setLastKey(key);
    setSrc(avatarCache.get(key) ?? '');
  }

  useEffect(() => {
    if (!enabled || !instanceToken || !user || user === '—') return;
    if (avatarCache.has(key)) return;
    let active = true;
    messagesApi
      .getAvatar(instanceToken, user)
      .then((url) => {
        avatarCache.set(key, url);
        if (active) setSrc(url);
      })
      .catch(() => {
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
  const [ref, inView] = useInView<HTMLSpanElement>();
  const src = useChatAvatar(token, jid, !isGroupLike && inView);
  const [failed, setFailed] = useState(false);

  const initial = (label?.trim() || jidUser(jid)).charAt(0).toUpperCase() || '?';

  return (
    <span
      ref={ref}
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

function MessageText({ text }: { text: string }) {
  const parts = text.split(/(https?:\/\/[^\s]+)/g);
  return (
    <>
      {parts.map((part, index) =>
        /^https?:\/\//.test(part) ? (
          <a
            key={`${part}-${index}`}
            href={part}
            target="_blank"
            rel="noreferrer noopener"
            className="break-all underline opacity-90 hover:opacity-100"
          >
            {part}
          </a>
        ) : (
          <Fragment key={index}>{part}</Fragment>
        )
      )}
    </>
  );
}

function MediaContent({
  message,
  token,
  mediaLocal,
  onOpenMedia,
}: {
  message: HistoryMessage;
  token: string;
  mediaLocal: boolean;
  onOpenMedia?: (message: HistoryMessage) => void;
}) {
  const { t } = useI18n();
  const { src, loading, failed } = useMediaSrc(message.media_url || '', token);

  if (!message.media_url) {
    const label = message.message_type || t('messages.mediaFallback');
    return (
      <p className="italic opacity-80">
        [{label}]
        {!mediaLocal && (
          <span
            className="ml-1 not-italic opacity-70"
            title="MEDIA_LOCAL_STORE=false"
          >
            {t('messages.mediaNotStored')}
          </span>
        )}
      </p>
    );
  }
  if (loading) {
    return (
      <div className="mb-1 h-40 w-56 max-w-full animate-pulse rounded-md bg-black/10" />
    );
  }
  if (failed || !src) {
    return (
      <p className="mb-1 flex items-center gap-1.5 italic opacity-80">
        <AlertCircle className="h-4 w-4" />
        {t('messages.mediaUnavailable')}
      </p>
    );
  }

  const mime = message.media_mimetype || '';
  if (mime.startsWith('image/')) {
    return (
      <button
        type="button"
        onClick={() => onOpenMedia?.(message)}
        className="mb-1 block max-w-full overflow-hidden rounded-md"
        title={t('messages.mediaViewerTitle')}
      >
        <img
          src={src}
          alt={message.message_type}
          className="max-h-72 w-auto max-w-full object-cover transition-opacity hover:opacity-90"
          loading="lazy"
        />
      </button>
    );
  }
  if (mime.startsWith('video/')) {
    return (
      <div className="relative mb-1">
        <video src={src} controls className="max-h-72 max-w-full rounded-md" />
        <button
          type="button"
          onClick={() => onOpenMedia?.(message)}
          className="absolute right-2 top-2 rounded-full bg-background/80 p-1.5 text-foreground shadow-sm"
          title={t('messages.mediaViewerTitle')}
          aria-label={t('messages.mediaViewerTitle')}
        >
          <Maximize2 className="h-4 w-4" />
        </button>
      </div>
    );
  }
  if (mime.startsWith('audio/')) {
    return <audio src={src} controls className="mb-1 w-64 max-w-full" />;
  }
  return (
    <a
      href={src}
      download
      target="_blank"
      rel="noreferrer noopener"
      className="mb-1 flex max-w-xs items-center gap-2 rounded-md border border-current/20 bg-black/5 p-2 text-xs underline-offset-2 hover:underline"
    >
      <FileText className="h-5 w-5 shrink-0" />
      <span className="min-w-0 flex-1 truncate">
        {message.message_type || t('messages.fileFallback')}
      </span>
      <Download className="h-4 w-4 shrink-0" />
    </a>
  );
}

function Bubble({
  message,
  token,
  mediaLocal,
  senderName,
  showSender,
  quotedMessage,
  onOpenMedia,
}: {
  message: HistoryMessage;
  token: string;
  mediaLocal: boolean;
  senderName?: string;
  showSender: boolean;
  quotedMessage?: HistoryMessage;
  onOpenMedia?: (message: HistoryMessage) => void;
}) {
  const { t } = useI18n();
  const mine = message.is_from_me;
  const isMedia =
    !!message.media_url ||
    ['image', 'video', 'audio', 'document', 'sticker'].includes(
      message.message_type
    );
  // Prefer the resolved name; fall back to the sender number, then a generic
  // label so a group message always shows who wrote it.
  const label =
    senderName?.trim() ||
    jidUser(message.sender_jid) ||
    t('messages.unknownSender');
  const hasBody = Boolean(message.text_content) || isMedia;

  return (
    <div className={`mb-1.5 flex last:mb-0 ${mine ? 'justify-end' : 'justify-start'}`}>
      <div
        className={`max-w-[85%] rounded-lg px-3 py-2 text-sm shadow-xs sm:max-w-[75%] ${
          mine ? 'bg-primary text-primary-foreground' : 'bg-muted text-foreground'
        }`}
      >
        {showSender && !mine && (
          <p className={`mb-0.5 text-[11px] font-semibold ${senderColor(label)}`}>
            {label}
          </p>
        )}

        {message.quoted_message_id && (
          <div
            className={`mb-1 rounded border-l-2 px-2 py-1 text-xs ${
              mine
                ? 'border-primary-foreground/40 bg-primary-foreground/10'
                : 'border-primary bg-primary/5'
            }`}
          >
            {quotedMessage ? (
              <>
                <p className="truncate font-medium opacity-90">
                  {quotedMessage.is_from_me
                    ? t('messages.you')
                    : jidUser(quotedMessage.sender_jid) ||
                      t('messages.unknownSender')}
                </p>
                <p className="truncate opacity-70">
                  {quotedMessage.text_content ||
                    `[${quotedMessage.message_type}]`}
                </p>
              </>
            ) : (
              <p className="italic opacity-70">
                {t('messages.quotedUnavailable')}
              </p>
            )}
          </div>
        )}

        {isMedia && (
          <MediaContent
            message={message}
            token={token}
            mediaLocal={mediaLocal}
            onOpenMedia={onOpenMedia}
          />
        )}
        {message.text_content ? (
          <p className="break-words whitespace-pre-wrap">
            <MessageText text={message.text_content} />
          </p>
        ) : !hasBody ? (
          <p className="italic opacity-80">
            [{message.message_type || t('messages.unsupportedMessage')}]
          </p>
        ) : null}

        <span
          className={`mt-1 flex items-center justify-end gap-1 text-[10px] ${
            mine ? 'text-primary-foreground/70' : 'text-muted-foreground'
          }`}
          title={message.timestamp}
        >
          {message.edited && <span>{t('messages.edited')}</span>}
          {formatTime(message.timestamp)}
          {mine && <StatusTick status={message.status} />}
        </span>
      </div>
    </div>
  );
}

export default function Messages() {
  const { t } = useI18n();
  const queryClient = useQueryClient();
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
    instance?.websocketEnable === 'enabled' ||
    instance?.websocketEnable === 'true';

  const [refreshing, setRefreshing] = useState(false);
  const [chatQuery, setChatQuery] = useState('');
  const [tab, setTab] = useState<TabId>('all');

  const [selectedChat, setSelectedChat] = useState('');
  const [draft, setDraft] = useState('');
  const [attachFile, setAttachFile] = useState<File | null>(null);
  const [attachPreview, setAttachPreview] = useState('');
  const [uploadProgress, setUploadProgress] = useState<number | null>(null);
  const [sending, setSending] = useState(false);
  const [mobileThread, setMobileThread] = useState(false);
  const [testModalOpen, setTestModalOpen] = useState(false);
  const [newChatOpen, setNewChatOpen] = useState(false);
  const [syncing, setSyncing] = useState(false);
  const [dragging, setDragging] = useState(false);
  const [viewer, setViewer] = useState<{
    items: HistoryMessage[];
    index: number;
  } | null>(null);

  const [atBottom, setAtBottom] = useState(true);
  const atBottomRef = useRef(true);
  const threadRef = useRef<HTMLDivElement>(null);
  const panesRef = useRef<HTMLDivElement>(null);
  const listPane = useResizablePane('wamux.messages.listWidth', panesRef);
  const prevChatRef = useRef('');
  const prevCountRef = useRef(0);
  const refreshTimer = useRef<number | undefined>(undefined);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const objectUrlRef = useRef('');

  // setAttachment swaps the pending file, revoking the previous preview URL.
  const setAttachment = (file: File | null) => {
    if (objectUrlRef.current) {
      URL.revokeObjectURL(objectUrlRef.current);
      objectUrlRef.current = '';
    }
    if (
      file &&
      (file.type.startsWith('image/') || file.type.startsWith('video/'))
    ) {
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

  // Populate the instance list when this page is opened directly.
  useEffect(() => {
    void fetchInstances();
  }, [fetchInstances]);

  const refreshThreadAndChats = useCallback(
    (withChats: boolean) => {
      if (!instanceId) return;
      if (refreshTimer.current) window.clearTimeout(refreshTimer.current);
      refreshTimer.current = window.setTimeout(() => {
        if (selectedChat) {
          void queryClient.invalidateQueries({
            queryKey: messageKeys.thread(instanceId, selectedChat),
          });
        }
        if (withChats) {
          void queryClient.invalidateQueries({
            queryKey: messageKeys.chats(instanceId),
          });
        }
      }, 400);
    },
    [instanceId, queryClient, selectedChat]
  );

  const wsStatus = useInstanceEvents(instance?.id, (event) => {
    if (event.event === 'Message' || event.event === 'SendMessage') {
      refreshThreadAndChats(true);
    } else if (event.event === 'Receipt') {
      refreshThreadAndChats(false);
    }
  });

  useEffect(
    () => () => {
      if (refreshTimer.current) window.clearTimeout(refreshTimer.current);
    },
    []
  );

  const chatsQuery = useQuery<ChatSummary[]>({
    queryKey: messageKeys.chats(instanceId),
    queryFn: () => messagesApi.listChats(token),
    enabled: Boolean(token),
    staleTime: 15_000,
    refetchInterval: wsStatus === 'open' ? false : 15_000,
  });
  const chats = useMemo(() => chatsQuery.data ?? [], [chatsQuery.data]);
  const chatsLoading = chatsQuery.isLoading;

  const threadQuery = useInfiniteQuery({
    queryKey: messageKeys.thread(instanceId, selectedChat),
    queryFn: ({ pageParam }) =>
      messagesApi.getHistory(
        token,
        selectedChat,
        PAGE_SIZE,
        pageParam as string | undefined
      ),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (lastPage) =>
      lastPage.length < PAGE_SIZE
        ? undefined
        : lastPage[lastPage.length - 1]?.timestamp,
    enabled: Boolean(token && selectedChat),
    staleTime: 10_000,
    refetchInterval:
      selectedChat && wsStatus !== 'open' ? 10_000 : false,
  });

  const sendersQuery = useQuery<Record<string, string>>({
    queryKey: messageKeys.senders(instanceId, selectedChat),
    queryFn: () => messagesApi.getSenderNames(token, selectedChat),
    enabled:
      Boolean(token && selectedChat) &&
      chatKind(selectedChat) !== 'contact',
    staleTime: 5 * 60_000,
  });

  const flagsQuery = useQuery({
    queryKey: messageKeys.flags,
    queryFn: fetchServerStats,
    staleTime: 60_000,
  });

  const flags = useMemo<FeatureFlags>(() => {
    const s = flagsQuery.data?.storage;
    return {
      historyEnabled: s?.historyEnabled ?? true,
      mediaLocal: s?.mediaLocal ?? true,
      webhookFiles: s?.webhookFiles ?? true,
      loaded: flagsQuery.isFetched,
    };
  }, [flagsQuery.data, flagsQuery.isFetched]);

  const messages = useMemo(() => {
    const byId = new Map<string, HistoryMessage>();
    for (const page of threadQuery.data?.pages ?? []) {
      for (const message of page) byId.set(message.message_id, message);
    }
    const sorted = Array.from(byId.values()).sort((a, b) => {
      if (a.timestamp !== b.timestamp) return a.timestamp < b.timestamp ? 1 : -1;
      return a.id < b.id ? 1 : -1;
    });
    return applyEdits(sorted);
  }, [threadQuery.data]);

  const ordered = useMemo(() => [...messages].reverse(), [messages]);
  const hasMore = Boolean(threadQuery.hasNextPage);
  const messagesLoading = threadQuery.isLoading;
  const olderLoading = threadQuery.isFetchingNextPage;

  const activeChat = useMemo(
    () => chats.find((c) => c.chat_jid === selectedChat),
    [chats, selectedChat]
  );
  const activeKind = selectedChat ? chatKind(selectedChat) : 'contact';
  const showSenderNames = activeKind !== 'contact';
  const namesForChat = sendersQuery.data ?? {};
  const activeTitle = displayTitle(activeChat, selectedChat, t);
  const messagesById = useMemo(
    () => new Map(messages.map((m) => [m.message_id, m])),
    [messages]
  );
  const mediaMessages = useMemo(
    () => ordered.filter((m) => Boolean(m.media_url)),
    [ordered]
  );

  // Reset the conversation view when the selected instance changes, during
  // render rather than in an effect.
  const [lastToken, setLastToken] = useState(token);
  if (token !== lastToken) {
    setLastToken(token);
    setSelectedChat('');
    setMobileThread(false);
    setNewChatOpen(false);
    setViewer(null);
  }

  // Keep the thread scrolled to the newest message when the chat opens or when
  // new messages arrive while the user is already at the bottom. Loading older
  // pages must not move the viewport.
  useEffect(() => {
    const el = threadRef.current;
    if (!el) return;
    const chatChanged = prevChatRef.current !== selectedChat;
    const grew = ordered.length > prevCountRef.current;
    if (chatChanged || (atBottomRef.current && grew)) {
      el.scrollTop = el.scrollHeight;
      atBottomRef.current = true;
      setAtBottom(true);
    }
    prevChatRef.current = selectedChat;
    prevCountRef.current = ordered.length;
  }, [selectedChat, ordered.length, messagesLoading]);

  const handleScroll = () => {
    const el = threadRef.current;
    if (!el) return;
    const bottom =
      el.scrollHeight - el.scrollTop - el.clientHeight < 80;
    atBottomRef.current = bottom;
    setAtBottom(bottom);
  };

  const scrollToBottom = () => {
    const el = threadRef.current;
    if (!el) return;
    el.scrollTop = el.scrollHeight;
    atBottomRef.current = true;
    setAtBottom(true);
  };

  const openChat = useCallback((chat: string) => {
    if (!chat) return;
    setSelectedChat(chat);
    setMobileThread(true);
    atBottomRef.current = true;
    setAtBottom(true);
  }, []);

  const loadOlder = () => {
    if (threadQuery.hasNextPage && !threadQuery.isFetchingNextPage) {
      void threadQuery.fetchNextPage();
    }
  };

  const sendMutation = useMutation<unknown, unknown, SendVars, SendContext>({
    mutationFn: (vars) => {
      if (vars.file) {
        return messagesApi.sendMedia(
          vars.token,
          {
            number: vars.chat,
            type: fileKind(vars.file),
            caption: vars.text || undefined,
            filename: vars.file.name,
            file: vars.file,
          },
          (percent) => setUploadProgress(percent)
        );
      }
      return messagesApi.sendText(vars.token, {
        number: vars.chat,
        text: vars.text,
      });
    },
    onMutate: async (vars) => {
      const threadKey = messageKeys.thread(vars.instanceId, vars.chat);
      const chatsKey = messageKeys.chats(vars.instanceId);
      await queryClient.cancelQueries({ queryKey: threadKey });

      const previousThread = queryClient.getQueryData<ThreadData>(threadKey);
      const previousChats = queryClient.getQueryData<ChatSummary[]>(chatsKey);
      const tempId = `local-${Date.now()}-${Math.random()
        .toString(36)
        .slice(2)}`;
      const timestamp = nowDbTimestamp();
      const type = vars.file ? fileKind(vars.file) : 'text';
      const temp: HistoryMessage = {
        id: tempId,
        message_id: tempId,
        instance_id: vars.instanceId,
        timestamp,
        status: 'Sent',
        source: jidUser(vars.chat),
        chat_jid: vars.chat,
        sender_jid: '',
        message_type: type,
        text_content: vars.text,
        media_url: '',
        media_mimetype: vars.file?.type ?? '',
        quoted_message_id: '',
        is_from_me: true,
      };

      if (previousThread) {
        queryClient.setQueryData<ThreadData>(threadKey, (old) =>
          addOptimisticMessage(old, temp)
        );
      }
      queryClient.setQueryData<ChatSummary[]>(chatsKey, (old) =>
        upsertChatSummary(old, vars.chat, {
          last_message_id: tempId,
          last_timestamp: timestamp,
          last_message_type: type,
          last_text_content: vars.text,
          last_status: 'Sent',
          last_from_me: true,
          message_count: (old?.find((c) => c.chat_jid === vars.chat)
            ?.message_count ?? 0) + 1,
        })
      );

      return { previousThread, previousChats, tempId };
    },
    onError: (error, vars, context) => {
      if (context?.previousThread) {
        queryClient.setQueryData(
          messageKeys.thread(vars.instanceId, vars.chat),
          context.previousThread
        );
      }
      if (context?.previousChats) {
        queryClient.setQueryData(
          messageKeys.chats(vars.instanceId),
          context.previousChats
        );
      }
      const message =
        (error as { message?: string })?.message || t('messages.errorSend');
      toast.error(message);
    },
    onSuccess: async (_data, vars, context) => {
      // Refetch first so the server message is present before the optimistic
      // row is removed (avoids a brief empty gap).
      await queryClient.invalidateQueries({
        queryKey: messageKeys.thread(vars.instanceId, vars.chat),
      });
      if (context?.tempId) {
        queryClient.setQueryData<ThreadData>(
          messageKeys.thread(vars.instanceId, vars.chat),
          (old) => removeMessage(old, context.tempId!)
        );
      }
      void queryClient.invalidateQueries({
        queryKey: messageKeys.chats(vars.instanceId),
      });
    },
    onSettled: () => {
      setUploadProgress(null);
      setSending(false);
    },
  });

  const selectedChatRef = useRef(selectedChat);
  useEffect(() => {
    selectedChatRef.current = selectedChat;
  }, [selectedChat]);

  const send = async () => {
    const text = draft.trim();
    if (!token || !selectedChat || sending) return;
    if (!text && !attachFile) return;

    const file = attachFile;
    const chatAtSend = selectedChat;
    setSending(true);
    setUploadProgress(file ? 0 : null);
    atBottomRef.current = true;
    setAtBottom(true);
    try {
      await sendMutation.mutateAsync({
        instanceId,
        token,
        chat: selectedChat,
        text,
        file,
      });
      if (selectedChatRef.current === chatAtSend) {
        setDraft((current) => (current === text ? '' : current));
        setAttachment(null);
      }
    } catch {
      // The mutation shows the error toast and restores the optimistic cache.
    }
  };

  const manualRefresh = async () => {
    if (refreshing) return;
    setRefreshing(true);
    const startedAt = Date.now();
    try {
      await Promise.all([
        chatsQuery.refetch(),
        selectedChat ? threadQuery.refetch() : Promise.resolve(),
      ]);
    } finally {
      const elapsed = Date.now() - startedAt;
      if (elapsed < 600) {
        await new Promise((resolve) => setTimeout(resolve, 600 - elapsed));
      }
      setRefreshing(false);
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
        Timestamp: newest?.timestamp
          ? newest.timestamp.replace(' ', 'T') + 'Z'
          : new Date().toISOString(),
      };
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
        {
          id: 'contact' as TabId,
          label: t('messages.sectionContacts'),
          count: counts.contact,
        },
        {
          id: 'group' as TabId,
          label: t('messages.sectionGroups'),
          count: counts.group,
        },
        {
          id: 'channel' as TabId,
          label: t('messages.sectionChannels'),
          count: counts.channel,
        },
      ].filter((tabDef) => tabDef.id === 'all' || tabDef.count > 0),
    [counts, t]
  );

  // The tab actually shown: if the active category has no rows (e.g. a filter
  // excluded them all), fall back to "all" without a state write, so the list is
  // never mysteriously empty.
  const activeTab: TabId = tabs.some((tabDef) => tabDef.id === tab)
    ? tab
    : 'all';

  const visibleChats = useMemo(() => {
    if (activeTab === 'all') return filteredChats;
    return filteredChats.filter((c) => chatKind(c.chat_jid) === activeTab);
  }, [filteredChats, activeTab]);

  const wsBadge = (
    <span
      className={`hidden items-center gap-1 text-xs sm:inline-flex ${
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

  const openMedia = (message: HistoryMessage) => {
    const index = mediaMessages.findIndex(
      (m) => m.message_id === message.message_id
    );
    setViewer({ items: mediaMessages, index: Math.max(0, index) });
  };

  const handleDrop = (e: DragEvent) => {
    e.preventDefault();
    setDragging(false);
    const file = e.dataTransfer.files?.[0];
    if (file) setAttachment(file);
  };

  const handlePaste = (e: ClipboardEvent) => {
    const file = e.clipboardData.files?.[0];
    if (file) {
      e.preventDefault();
      setAttachment(file);
    }
  };

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
      <div className="flex flex-wrap items-center gap-2 border-b border-border p-3 sm:gap-3 sm:p-4">
        <h1 className="hidden text-xl font-semibold sm:block">
          {t('messages.title')}
        </h1>
        <select
          value={instanceId}
          onChange={(e) => setInstanceId(e.target.value)}
          aria-label={t('messages.title')}
          className="h-9 min-w-0 flex-1 rounded-md border border-input bg-background px-3 text-sm sm:max-w-xs sm:flex-none"
        >
          {connected.map((i) => (
            <option key={i.id} value={i.id}>
              {i.instanceName}
              {i.owner ? ` (${i.owner})` : ''}
            </option>
          ))}
        </select>
        <Button
          variant="outline"
          size="sm"
          onClick={() => setNewChatOpen(true)}
          title={t('messages.newConversation')}
        >
          <MessageSquarePlus />
          <span className="hidden sm:inline">
            {t('messages.newConversation')}
          </span>
        </Button>
        <Button
          variant="outline"
          size="sm"
          onClick={() => void manualRefresh()}
          disabled={refreshing}
          title={t('messages.refresh')}
        >
          <RefreshCw className={refreshing ? 'animate-spin' : ''} />
          <span className="hidden sm:inline">{t('messages.refresh')}</span>
        </Button>
        <Button
          variant="outline"
          size="sm"
          onClick={() => setTestModalOpen(true)}
          title={t('messages.testMessagesTitle')}
        >
          <FlaskConical />
          <span className="hidden lg:inline">{t('messages.testMessages')}</span>
        </Button>
        {wsEnabled ? (
          wsBadge
        ) : (
          <span
            className="hidden items-center gap-1 text-xs text-amber-600 sm:inline-flex"
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
            <strong>{t('messages.mediaLocalDisabledStrong')}</strong>{' '}
            (<code>MEDIA_LOCAL_STORE=false</code>
            {!flags.historyEnabled && (
              <>
                {' '}
                {t('messages.and')} <code>DATABASE_SAVE_MESSAGES=false</code>
              </>
            )}
            {t('messages.mediaLocalDisabledMid')}
            <code>MEDIA_LOCAL_STORE=true</code>
            {!flags.historyEnabled && (
              <>
                {' '}
                {t('messages.and')} <code>DATABASE_SAVE_MESSAGES=true</code>
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
            <strong>{t('messages.historyDisabledStrong')}</strong>{' '}
            (<code>DATABASE_SAVE_MESSAGES=false</code>
            {t('messages.historyDisabledMid')}
            <strong>{t('messages.recover')}</strong>
            {t('messages.historyDisabledEnd')}
            <code>DATABASE_SAVE_MESSAGES=true</code>
            {t('messages.historyDisabledSuffix')}
          </span>
        </div>
      )}

      <div
        ref={panesRef}
        style={{ '--list-w': `${listPane.width}px` } as CSSProperties}
        className={cn(
          'relative grid min-h-0 flex-1 lg:grid-cols-[min(var(--list-w),calc(100%_-_360px))_minmax(0,1fr)]',
          listPane.dragging && 'cursor-col-resize select-none'
        )}
      >
        {/* Drag to resize the list, like WhatsApp Desktop. */}
        <div
          role="separator"
          tabIndex={0}
          aria-orientation="vertical"
          aria-label={t('messages.resizeList')}
          aria-valuemin={PANE_MIN}
          aria-valuemax={PANE_MAX}
          aria-valuenow={listPane.width}
          {...listPane.handleProps}
          className={cn(
            'absolute inset-y-0 z-20 hidden w-2 -translate-x-1/2 cursor-col-resize touch-none transition-colors hover:bg-primary/30 focus-visible:bg-primary/30 focus-visible:outline-none lg:left-[min(var(--list-w),calc(100%_-_360px))] lg:block',
            listPane.dragging && 'bg-primary/30'
          )}
        />

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

          <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain">
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
                const number =
                  chatKind(c.chat_jid) === 'contact' ? jidUser(c.chat_jid) : '';
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
                        <span className="truncate text-sm font-medium">
                          {title}
                        </span>
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
                            (c.last_message_type
                              ? `[${c.last_message_type}]`
                              : '')}
                        </span>
                        <span className="shrink-0 text-[10px] text-muted-foreground">
                          {t('messages.messageCount', {
                            count: c.message_count,
                          })}
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
          className={`relative min-h-0 flex-col ${mobileThread ? 'flex' : 'hidden lg:flex'}`}
          onDragOver={(e) => {
            e.preventDefault();
            setDragging(true);
          }}
          onDragLeave={(e) => {
            const next = e.relatedTarget as Node | null;
            if (!next || !e.currentTarget.contains(next)) setDragging(false);
          }}
          onDrop={handleDrop}
        >
          {!selectedChat ? (
            <div className="flex flex-1 items-center justify-center p-8 text-sm text-muted-foreground">
              {t('messages.selectChat')}
            </div>
          ) : (
            <>
              <div className="flex shrink-0 items-center gap-3 border-b border-border bg-background p-3">
                <Button
                  variant="ghost"
                  size="icon"
                  className="lg:hidden"
                  onClick={() => setMobileThread(false)}
                >
                  <ArrowLeft />
                </Button>
                <ChatAvatar
                  jid={selectedChat}
                  token={token}
                  label={activeTitle}
                  size={36}
                />
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-medium">{activeTitle}</p>
                  <p className="truncate text-xs text-muted-foreground">
                    {[
                      activeKind === 'contact' ? jidUser(selectedChat) : '',
                      activeChat
                        ? t('messages.messageCount', {
                            count: activeChat.message_count,
                          })
                        : '',
                    ]
                      .filter(Boolean)
                      .join(' · ')}
                  </p>
                </div>
                <Button
                  variant="outline"
                  size="icon"
                  onClick={() => void recoverHistory()}
                  disabled={syncing}
                  title={t('messages.recoverTitle')}
                  aria-label={t('messages.recoverTitle')}
                >
                  <History className={syncing ? 'animate-spin' : ''} />
                </Button>
              </div>

              {/* The jump button lives in here, so whatever opens below the thread
                  (a media preview, the composer) can never end up under it. */}
              <div className="relative flex min-h-0 flex-1 flex-col">
              <div
                ref={threadRef}
                onScroll={handleScroll}
                className="min-h-0 flex-1 overflow-y-auto overscroll-contain p-3 sm:p-4"
              >
                {hasMore && messages.length > 0 && (
                  <div className="mb-3 flex justify-center">
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={loadOlder}
                      disabled={olderLoading}
                    >
                      {olderLoading
                        ? t('common.loading')
                        : t('messages.loadOlder')}
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
                  ordered.map((m, index) => {
                    const prev = ordered[index - 1];
                    const newDay =
                      !prev || formatDay(prev.timestamp) !== formatDay(m.timestamp);
                    const quoted = m.quoted_message_id
                      ? messagesById.get(m.quoted_message_id)
                      : undefined;
                    const showSender =
                      showSenderNames &&
                      !m.is_from_me &&
                      (newDay ||
                        !prev ||
                        prev.is_from_me ||
                        prev.sender_jid !== m.sender_jid);
                    return (
                      <Fragment key={m.message_id}>
                        {newDay && (
                          <div className="my-3 flex justify-center">
                            <span className="rounded-full bg-muted px-3 py-1 text-[11px] text-muted-foreground">
                              {formatDay(m.timestamp)}
                            </span>
                          </div>
                        )}
                        <Bubble
                          message={m}
                          token={token}
                          mediaLocal={flags.mediaLocal}
                          senderName={namesForChat[jidUser(m.sender_jid)]}
                          showSender={showSender}
                          quotedMessage={quoted}
                          onOpenMedia={openMedia}
                        />
                      </Fragment>
                    );
                  })
                )}
              </div>

              {!atBottom && ordered.length > 0 && (
                <Button
                  type="button"
                  size="icon"
                  variant="outline"
                  onClick={scrollToBottom}
                  className="absolute bottom-4 right-4 z-10 rounded-full shadow-md"
                  title={t('messages.jumpToLatest')}
                  aria-label={t('messages.jumpToLatest')}
                >
                  <ArrowDown className="h-4 w-4" />
                </Button>
              )}
              </div>

              {dragging && (
                <div className="pointer-events-none absolute inset-3 z-20 flex items-center justify-center rounded-xl border-2 border-dashed border-primary bg-background/80 text-sm font-medium text-primary">
                  {t('messages.dropFile')}
                </div>
              )}

              {attachFile && (
                <div className="shrink-0 border-t border-border p-3">
                  <div className="flex items-center gap-3">
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
                      <p className="truncate text-xs font-medium">
                        {attachFile.name}
                      </p>
                      <p className="text-[11px] text-muted-foreground">
                        {humanSize(attachFile.size)}
                      </p>
                      {uploadProgress !== null && (
                        <div className="mt-1 h-1 w-full overflow-hidden rounded-full bg-muted">
                          <div
                            className="h-full bg-primary transition-all"
                            style={{ width: `${uploadProgress}%` }}
                          />
                        </div>
                      )}
                    </div>
                    <button
                      onClick={() => setAttachment(null)}
                      title={t('messages.removeAttachment')}
                    >
                      <X className="h-4 w-4" />
                    </button>
                  </div>
                </div>
              )}

              <div className="flex shrink-0 items-end gap-2 border-t border-border bg-background p-3 pb-[max(0.75rem,env(safe-area-inset-bottom))]">
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
                  onPaste={handlePaste}
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
                  {sending ? (
                    <Loader2 className="animate-spin" />
                  ) : (
                    <SendHorizontal />
                  )}
                  <span className="hidden sm:inline">{t('messages.send')}</span>
                </Button>
              </div>
            </>
          )}
        </section>
      </div>

      <NewChatDialog
        open={newChatOpen}
        onOpenChange={setNewChatOpen}
        instanceId={instanceId}
        instanceToken={token}
        recentChats={chats}
        onSelect={openChat}
      />

      <MediaViewer
        open={Boolean(viewer)}
        items={viewer?.items ?? []}
        initialIndex={viewer?.index ?? 0}
        token={token}
        onClose={() => setViewer(null)}
      />

      <TestMessageModal
        open={testModalOpen}
        onClose={() => setTestModalOpen(false)}
        instance={instance ?? null}
      />
    </div>
  );
}
