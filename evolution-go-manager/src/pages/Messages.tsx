import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  ArrowLeft,
  Check,
  CheckCheck,
  MessageSquare,
  Paperclip,
  RefreshCw,
  SendHorizontal,
  Wifi,
  WifiOff,
  X,
} from 'lucide-react';
import { toast } from 'sonner';

import { Button, Input, Skeleton } from '@/components/ui';
import useInstancesStore from '@/store/instancesStore';
import useInstanceEvents from '@/hooks/useInstanceEvents';
import * as messagesApi from '@/services/api/messages';
import type { ChatSummary, HistoryMessage } from '@/types/messages';

const PAGE_SIZE = 50;

type ChatKind = 'contact' | 'group' | 'channel';

function jidUser(jid: string): string {
  if (!jid) return '—';
  return jid.split('@')[0].split(':')[0] || jid;
}

function chatKind(jid: string): ChatKind {
  if (jid.endsWith('@g.us')) return 'group';
  if (jid.endsWith('@newsletter')) return 'channel';
  return 'contact';
}

function chatTitle(jid: string): string {
  if (!jid) return 'Conversa';
  if (jid.endsWith('@g.us')) return `Grupo ${jidUser(jid)}`;
  if (jid.endsWith('@newsletter')) return `Canal ${jidUser(jid)}`;
  if (jid.endsWith('@lid')) return `LID ${jidUser(jid)}`;
  return jidUser(jid);
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
  if (status === 'Read') return <CheckCheck className="h-3.5 w-3.5" />;
  if (status === 'Delivered') return <CheckCheck className="h-3.5 w-3.5 opacity-70" />;
  return <Check className="h-3.5 w-3.5 opacity-70" />;
}

function MediaContent({ message }: { message: HistoryMessage }) {
  if (!message.media_url) {
    return <p className="italic opacity-80">[{message.message_type || 'mídia'}]</p>;
  }
  const mime = message.media_mimetype || '';
  if (mime.startsWith('image/')) {
    return (
      <img
        src={message.media_url}
        alt={message.message_type}
        className="mb-1 max-h-72 rounded-md"
        loading="lazy"
      />
    );
  }
  if (mime.startsWith('video/')) {
    return <video src={message.media_url} controls className="mb-1 max-h-72 rounded-md" />;
  }
  return (
    <a
      href={message.media_url}
      target="_blank"
      rel="noreferrer noopener"
      className="mb-1 inline-block underline"
    >
      Abrir {message.message_type || 'arquivo'}
    </a>
  );
}

function Bubble({ message }: { message: HistoryMessage }) {
  const mine = message.is_from_me;
  const isMedia =
    !!message.media_url ||
    ['image', 'video', 'audio', 'document', 'sticker'].includes(message.message_type);
  return (
    <div className={`flex ${mine ? 'justify-end' : 'justify-start'}`}>
      <div
        className={`max-w-[75%] rounded-lg px-3 py-2 text-sm shadow-xs ${
          mine ? 'bg-primary text-primary-foreground' : 'bg-muted text-foreground'
        }`}
      >
        {isMedia && <MediaContent message={message} />}
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
  const instances = useInstancesStore((s) => s.instances);
  const instancesLoading = useInstancesStore((s) => s.isLoading);
  const fetchInstances = useInstancesStore((s) => s.fetchInstances);
  const connected = useMemo(
    () => instances.filter((i) => i.connected),
    [instances]
  );

  const [instanceId, setInstanceId] = useState('');
  const instance = useMemo(
    () => instances.find((i) => i.id === instanceId),
    [instances, instanceId]
  );
  const token = instance?.apikey ?? '';

  const [chats, setChats] = useState<ChatSummary[]>([]);
  const [chatsLoading, setChatsLoading] = useState(false);
  const [refreshing, setRefreshing] = useState(false);
  const [chatQuery, setChatQuery] = useState('');

  const [selectedChat, setSelectedChat] = useState('');
  const [messages, setMessages] = useState<HistoryMessage[]>([]);
  const [messagesLoading, setMessagesLoading] = useState(false);
  const [olderLoading, setOlderLoading] = useState(false);
  const [hasMore, setHasMore] = useState(true);

  const [draft, setDraft] = useState('');
  const [attachFile, setAttachFile] = useState<File | null>(null);
  const [sending, setSending] = useState(false);
  const [newNumber, setNewNumber] = useState('');
  const [mobileThread, setMobileThread] = useState(false);

  const fileInputRef = useRef<HTMLInputElement>(null);

  // Populate the instance list when this page is opened directly. The store is
  // otherwise only filled by the Instances/Dashboard pages, so a direct load of
  // /manager/messages would otherwise report "no connected instance".
  useEffect(() => {
    void fetchInstances();
  }, [fetchInstances]);

  // Pick the first connected instance once one is available.
  useEffect(() => {
    if (!instanceId && connected.length > 0) setInstanceId(connected[0].id);
  }, [connected, instanceId]);

  const loadChats = useCallback(async () => {
    if (!token) return;
    setChatsLoading(true);
    try {
      setChats(await messagesApi.listChats(token));
    } catch {
      toast.error('Não foi possível carregar as conversas');
    } finally {
      setChatsLoading(false);
    }
  }, [token]);

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
        toast.error('Não foi possível carregar o histórico');
      } finally {
        setMessagesLoading(false);
      }
    },
    [token]
  );

  // Reset everything when the selected instance changes.
  useEffect(() => {
    setChats([]);
    setSelectedChat('');
    setMessages([]);
    setMobileThread(false);
    if (token) void loadChats();
  }, [token, loadChats]);

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
      toast.error('Não foi possível carregar mensagens antigas');
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
        setAttachFile(null);
      } else {
        await messagesApi.sendText(token, { number: selectedChat, text });
      }
      setDraft('');
      await refreshThread();
      void loadChats();
    } catch {
      toast.error('Não foi possível enviar a mensagem');
    } finally {
      setSending(false);
    }
  };

  const filteredChats = useMemo(() => {
    const q = chatQuery.trim().toLowerCase();
    if (!q) return chats;
    return chats.filter((c) => c.chat_jid.toLowerCase().includes(q));
  }, [chats, chatQuery]);

  const sections = useMemo(() => {
    const contacts: ChatSummary[] = [];
    const groups: ChatSummary[] = [];
    const channels: ChatSummary[] = [];
    for (const c of filteredChats) {
      const kind = chatKind(c.chat_jid);
      (kind === 'group' ? groups : kind === 'channel' ? channels : contacts).push(c);
    }
    return [
      { title: 'Contatos', items: contacts },
      { title: 'Grupos', items: groups },
      { title: 'Canais', items: channels },
    ].filter((s) => s.items.length > 0);
  }, [filteredChats]);

  const ordered = useMemo(() => [...messages].reverse(), [messages]);
  const activeChat = useMemo(
    () => chats.find((c) => c.chat_jid === selectedChat),
    [chats, selectedChat]
  );

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
          ? 'Atualizações em tempo real conectadas'
          : wsStatus === 'connecting'
            ? 'Conectando ao tempo real…'
            : 'Tempo real offline — reconectando'
      }
    >
      {wsStatus === 'open' ? (
        <Wifi className="h-3.5 w-3.5" />
      ) : (
        <WifiOff className="h-3.5 w-3.5" />
      )}
      {wsStatus === 'open'
        ? 'Tempo real'
        : wsStatus === 'connecting'
          ? 'Conectando…'
          : 'Offline'}
    </span>
  );

  if (instancesLoading && instances.length === 0) {
    return (
      <div className="flex h-full items-center justify-center p-8 text-sm text-muted-foreground">
        Carregando instâncias…
      </div>
    );
  }

  if (connected.length === 0) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-3 p-8 text-center">
        <MessageSquare className="h-10 w-10 text-muted-foreground" />
        <p className="text-sm text-muted-foreground">
          Nenhuma instância conectada. Conecte uma instância para ver as conversas.
        </p>
      </div>
    );
  }

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex flex-wrap items-center gap-3 border-b border-border p-4">
        <h1 className="text-xl font-semibold">Mensagens</h1>
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
          <RefreshCw className={refreshing ? 'animate-spin' : ''} /> Atualizar
        </Button>
        {wsBadge}
      </div>

      <div className="grid min-h-0 flex-1 lg:grid-cols-[320px_1fr]">
        {/* Conversation list */}
        <aside
          className={`min-h-0 flex-col border-r border-border ${
            mobileThread ? 'hidden lg:flex' : 'flex'
          }`}
        >
          <div className="space-y-2 border-b border-border p-3">
            <Input
              placeholder="Filtrar conversas…"
              value={chatQuery}
              onChange={(e) => setChatQuery(e.target.value)}
            />
            <div className="flex gap-2">
              <Input
                placeholder="Nova conversa (número)"
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
                Abrir
              </Button>
            </div>
          </div>
          <div className="min-h-0 flex-1 overflow-y-auto">
            {chatsLoading && chats.length === 0 ? (
              <div className="space-y-2 p-3">
                {Array.from({ length: 6 }).map((_, i) => (
                  <Skeleton key={i} className="h-12 w-full" />
                ))}
              </div>
            ) : sections.length === 0 ? (
              <p className="p-4 text-sm text-muted-foreground">
                Sem conversas salvas. Inicie uma acima.
              </p>
            ) : (
              sections.map((section) => (
                <div key={section.title}>
                  <div className="sticky top-0 bg-background/95 px-3 py-1.5 text-[11px] font-semibold tracking-wide text-muted-foreground uppercase">
                    {section.title}
                  </div>
                  {section.items.map((c) => (
                    <button
                      key={c.chat_jid}
                      onClick={() => openChat(c.chat_jid)}
                      className={`flex w-full flex-col gap-1 border-b border-border px-3 py-2 text-left hover:bg-accent ${
                        selectedChat === c.chat_jid ? 'bg-accent' : ''
                      }`}
                    >
                      <span className="flex items-center justify-between gap-2">
                        <span className="truncate text-sm font-medium">
                          {chatTitle(c.chat_jid)}
                        </span>
                        <span className="shrink-0 text-[11px] text-muted-foreground">
                          {formatShort(c.last_timestamp)}
                        </span>
                      </span>
                      <span className="truncate text-xs text-muted-foreground">
                        {c.last_from_me ? 'Você: ' : ''}
                        {c.last_text_content || (c.last_message_type ? `[${c.last_message_type}]` : '')}
                      </span>
                    </button>
                  ))}
                </div>
              ))
            )}
          </div>
        </aside>

        {/* Thread */}
        <section
          className={`min-h-0 flex-col ${mobileThread ? 'flex' : 'hidden lg:flex'}`}
        >
          {!selectedChat ? (
            <div className="flex flex-1 items-center justify-center p-8 text-sm text-muted-foreground">
              Selecione uma conversa ou inicie uma nova.
            </div>
          ) : (
            <>
              <div className="flex items-center gap-2 border-b border-border p-3">
                <Button
                  variant="ghost"
                  size="icon"
                  className="lg:hidden"
                  onClick={() => setMobileThread(false)}
                >
                  <ArrowLeft />
                </Button>
                <div className="min-w-0">
                  <p className="truncate text-sm font-medium">
                    {chatTitle(selectedChat)}
                  </p>
                  <p className="truncate text-xs text-muted-foreground">
                    {activeChat ? `${activeChat.message_count} mensagens` : ''}
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
                      {olderLoading ? 'Carregando…' : 'Carregar antigas'}
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
                    Nenhuma mensagem ainda. Envie a primeira abaixo.
                  </p>
                ) : (
                  <div className="flex flex-col gap-2">
                    {ordered.map((m) => (
                      <Bubble key={m.message_id} message={m} />
                    ))}
                  </div>
                )}
              </div>

              {attachFile && (
                <div className="flex items-center gap-2 border-t border-border px-3 pt-2 text-xs text-muted-foreground">
                  <Paperclip className="h-3.5 w-3.5" />
                  <span className="truncate">{attachFile.name}</span>
                  <button onClick={() => setAttachFile(null)} title="Remover anexo">
                    <X className="h-3.5 w-3.5" />
                  </button>
                </div>
              )}
              <div className="flex items-end gap-2 border-t border-border p-3">
                <input
                  ref={fileInputRef}
                  type="file"
                  className="hidden"
                  onChange={(e) => setAttachFile(e.target.files?.[0] ?? null)}
                />
                <Button
                  variant="outline"
                  size="icon"
                  onClick={() => fileInputRef.current?.click()}
                  title="Anexar foto, vídeo ou documento"
                >
                  <Paperclip />
                </Button>
                <Input
                  value={draft}
                  onChange={(e) => setDraft(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter' && !e.shiftKey) {
                      e.preventDefault();
                      void send();
                    }
                  }}
                  placeholder="Digite uma mensagem…"
                />
                <Button
                  onClick={() => void send()}
                  disabled={sending || (!draft.trim() && !attachFile)}
                >
                  <SendHorizontal /> Enviar
                </Button>
              </div>
            </>
          )}
        </section>
      </div>
    </div>
  );
}
