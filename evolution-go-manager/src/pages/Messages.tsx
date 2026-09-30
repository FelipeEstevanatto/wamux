import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  ArrowLeft,
  Check,
  CheckCheck,
  MessageSquare,
  RefreshCw,
  SendHorizontal,
} from 'lucide-react';
import { toast } from 'sonner';

import { Button, Input, Skeleton } from '@/components/ui';
import useInstancesStore from '@/store/instancesStore';
import useInstanceEvents from '@/hooks/useInstanceEvents';
import * as messagesApi from '@/services/api/messages';
import type { ChatSummary, HistoryMessage } from '@/types/messages';

const PAGE_SIZE = 50;

function jidUser(jid: string): string {
  if (!jid) return '—';
  return jid.split('@')[0].split(':')[0] || jid;
}

function chatTitle(jid: string): string {
  if (!jid) return 'Conversa';
  if (jid.endsWith('@g.us')) return `Grupo ${jidUser(jid)}`;
  if (jid.endsWith('@newsletter')) return `Canal ${jidUser(jid)}`;
  if (jid.endsWith('@lid')) return `LID ${jidUser(jid)}`;
  return jidUser(jid);
}

function parseTs(ts: string): Date | null {
  if (!ts) return null;
  const d = new Date(ts.includes('T') ? ts : ts.replace(' ', 'T'));
  return Number.isNaN(d.getTime()) ? null : d;
}

function formatShort(ts: string): string {
  const d = parseTs(ts);
  if (!d) return '';
  return d.toLocaleString('pt-BR', { day: '2-digit', month: '2-digit' });
}

function formatTime(ts: string): string {
  const d = parseTs(ts);
  if (!d) return '';
  return d.toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' });
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

function Bubble({ message }: { message: HistoryMessage }) {
  const mine = message.is_from_me;
  const text =
    message.text_content || `[${message.message_type || 'mensagem'}]`;
  return (
    <div className={`flex ${mine ? 'justify-end' : 'justify-start'}`}>
      <div
        className={`max-w-[75%] rounded-lg px-3 py-2 text-sm shadow-xs ${
          mine ? 'bg-primary text-primary-foreground' : 'bg-muted text-foreground'
        }`}
      >
        <p className="break-words whitespace-pre-wrap">{text}</p>
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
  const [chatQuery, setChatQuery] = useState('');

  const [selectedChat, setSelectedChat] = useState('');
  const [messages, setMessages] = useState<HistoryMessage[]>([]);
  const [messagesLoading, setMessagesLoading] = useState(false);
  const [olderLoading, setOlderLoading] = useState(false);
  const [hasMore, setHasMore] = useState(true);

  const [draft, setDraft] = useState('');
  const [sending, setSending] = useState(false);
  const [mobileThread, setMobileThread] = useState(false);

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

  useInstanceEvents(instance?.id, (event) => {
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

  const send = async () => {
    const text = draft.trim();
    if (!token || !selectedChat || !text || sending) return;
    setSending(true);
    try {
      await messagesApi.sendText(token, { number: selectedChat, text });
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

  const ordered = useMemo(() => [...messages].reverse(), [messages]);
  const activeChat = useMemo(
    () => chats.find((c) => c.chat_jid === selectedChat),
    [chats, selectedChat]
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
            </option>
          ))}
        </select>
        <Button
          variant="outline"
          size="sm"
          onClick={() => void loadChats()}
          disabled={chatsLoading}
        >
          <RefreshCw className={chatsLoading ? 'animate-spin' : ''} /> Atualizar
        </Button>
      </div>

      <div className="grid min-h-0 flex-1 lg:grid-cols-[320px_1fr]">
        {/* Conversation list */}
        <aside
          className={`min-h-0 flex-col border-r border-border ${
            mobileThread ? 'hidden lg:flex' : 'flex'
          }`}
        >
          <div className="border-b border-border p-3">
            <Input
              placeholder="Filtrar conversas…"
              value={chatQuery}
              onChange={(e) => setChatQuery(e.target.value)}
            />
          </div>
          <div className="min-h-0 flex-1 overflow-y-auto">
            {chatsLoading && chats.length === 0 ? (
              <div className="space-y-2 p-3">
                {Array.from({ length: 6 }).map((_, i) => (
                  <Skeleton key={i} className="h-12 w-full" />
                ))}
              </div>
            ) : filteredChats.length === 0 ? (
              <p className="p-4 text-sm text-muted-foreground">
                Sem conversas salvas.
              </p>
            ) : (
              filteredChats.map((c) => (
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
                    {c.last_text_content || `[${c.last_message_type}]`}
                  </span>
                </button>
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
              Selecione uma conversa.
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
                    Nenhuma mensagem armazenada.
                  </p>
                ) : (
                  <div className="flex flex-col gap-2">
                    {ordered.map((m) => (
                      <Bubble key={m.message_id} message={m} />
                    ))}
                  </div>
                )}
              </div>

              <div className="flex items-end gap-2 border-t border-border p-3">
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
                  disabled={sending || !draft.trim()}
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
