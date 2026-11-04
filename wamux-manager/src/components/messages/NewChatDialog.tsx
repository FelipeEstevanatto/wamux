import { useMemo, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Loader2, MessageSquarePlus, Search, User, Users } from 'lucide-react';

import {
  Button,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  Input,
} from '@/components/ui';
import { useI18n } from '@/i18n/I18nContext';
import * as messagesApi from '@/services/api/messages';
import type { ChatContact, ChatSummary } from '@/types/messages';
import { cn } from '@/utils/cn';

type TabId = 'all' | 'contact' | 'group';

interface NewChatDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  instanceId: string;
  instanceToken: string;
  recentChats: ChatSummary[];
  onSelect: (jid: string) => void;
}

function jidUser(jid: string): string {
  return jid.split('@')[0].split(':')[0] || jid;
}

function toChatJid(raw: string): string {
  const value = raw.trim();
  if (!value) return '';
  if (value.includes('@')) return value;
  return value.replace(/[^0-9]/g, '') + '@s.whatsapp.net';
}

function contactLabel(contact: ChatContact): string {
  return contact.name?.trim() || jidUser(contact.jid);
}

/**
 * A proper new-conversation sheet: searchable contacts/groups, recent chats and
 * a phone-number entry point. Replaces the two inline inputs that used to live
 * in the conversation list toolbar.
 */
export default function NewChatDialog({
  open,
  onOpenChange,
  instanceId,
  instanceToken,
  recentChats,
  onSelect,
}: NewChatDialogProps) {
  const { t } = useI18n();
  const [query, setQuery] = useState('');
  const [phone, setPhone] = useState('');
  const [tab, setTab] = useState<TabId>('all');

  const contactsQuery = useQuery<ChatContact[]>({
    queryKey: ['chat-contacts', instanceId],
    queryFn: () => messagesApi.listChatContacts(instanceToken),
    enabled: open && Boolean(instanceToken),
    staleTime: 60_000,
    retry: 0,
  });

  const contacts = useMemo(
    () => contactsQuery.data ?? [],
    [contactsQuery.data]
  );
  const q = query.trim().toLowerCase();

  const filteredContacts = useMemo(() => {
    const byTab =
      tab === 'all'
        ? contacts
        : contacts.filter((c) => (tab === 'group' ? c.is_group : !c.is_group));
    if (!q) return byTab;
    return byTab.filter(
      (c) =>
        contactLabel(c).toLowerCase().includes(q) ||
        c.jid.toLowerCase().includes(q)
    );
  }, [contacts, q, tab]);

  const counts = useMemo(() => {
    const all = contacts.length;
    const groups = contacts.filter((c) => c.is_group).length;
    return { all, contact: all - groups, group: groups };
  }, [contacts]);

  const recent = useMemo(() => {
    if (q) return [];
    return recentChats
      .filter((c) => c.last_timestamp)
      .slice(0, 5)
      .filter(
        (c) =>
          !contacts.some((contact) => contact.jid === c.chat_jid)
      );
  }, [recentChats, contacts, q]);

  const phoneDigits = phone.replace(/\D/g, '');
  const phoneValid = phoneDigits.length >= 8;

  const select = (jid: string) => {
    if (!jid) return;
    onSelect(jid);
    setQuery('');
    setPhone('');
    onOpenChange(false);
  };

  const tabs: { id: TabId; label: string; count: number }[] = [
    { id: 'all', label: t('messages.tabAll'), count: counts.all },
    { id: 'contact', label: t('messages.sectionContacts'), count: counts.contact },
    { id: 'group', label: t('messages.sectionGroups'), count: counts.group },
  ];

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90dvh] overflow-y-auto max-sm:top-auto max-sm:bottom-0 max-sm:max-w-full max-sm:translate-y-0 max-sm:rounded-b-none">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <MessageSquarePlus className="h-5 w-5 text-primary" />
            {t('messages.newConversation')}
          </DialogTitle>
          <DialogDescription>
            {t('messages.newChatDescription')}
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          <div className="relative">
            <Search className="pointer-events-none absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
            <Input
              className="pl-8"
              autoFocus
              placeholder={t('messages.filterPlaceholder')}
              value={query}
              onChange={(e) => setQuery(e.target.value)}
            />
          </div>

          <div>
            <label
              htmlFor="new-chat-phone"
              className="mb-1 block text-xs font-medium text-muted-foreground"
            >
              {t('messages.newChatPhone')}
            </label>
            <div className="flex gap-2">
              <Input
                id="new-chat-phone"
                type="tel"
                inputMode="tel"
                placeholder={t('messages.newChatPhonePlaceholder')}
                value={phone}
                onChange={(e) => setPhone(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter' && phoneValid) {
                    e.preventDefault();
                    select(toChatJid(phone));
                  }
                }}
              />
              <Button
                type="button"
                onClick={() => select(toChatJid(phone))}
                disabled={!phoneValid}
              >
                {t('messages.open')}
              </Button>
            </div>
            {phone.length > 0 && !phoneValid && (
              <p className="mt-1 text-xs text-destructive">
                {t('messages.newChatInvalidPhone')}
              </p>
            )}
          </div>

          <div className="flex flex-wrap gap-1">
            {tabs.map((tabDef) => (
              <button
                key={tabDef.id}
                type="button"
                onClick={() => setTab(tabDef.id)}
                className={cn(
                  'inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs font-medium transition-colors',
                  tab === tabDef.id
                    ? 'bg-primary text-primary-foreground'
                    : 'text-muted-foreground hover:bg-muted'
                )}
              >
                {tabDef.label}
                <span
                  className={cn(
                    'rounded-full px-1.5 text-[10px] leading-4',
                    tab === tabDef.id
                      ? 'bg-primary-foreground/20'
                      : 'bg-foreground/10'
                  )}
                >
                  {tabDef.count}
                </span>
              </button>
            ))}
          </div>

          {recent.length > 0 && (
            <div className="space-y-1">
              <p className="px-1 text-[11px] font-medium uppercase tracking-wide text-muted-foreground">
                {t('messages.newChatRecent')}
              </p>
              {recent.map((chat) => {
                const label = chat.name?.trim() || jidUser(chat.chat_jid);
                return (
                  <button
                    key={chat.chat_jid}
                    type="button"
                    onClick={() => select(chat.chat_jid)}
                    className="flex w-full items-center gap-3 rounded-md px-2 py-2 text-left hover:bg-accent"
                  >
                    {chat.chat_jid.endsWith('@g.us') ? (
                      <Users className="h-4 w-4 shrink-0 text-muted-foreground" />
                    ) : (
                      <User className="h-4 w-4 shrink-0 text-muted-foreground" />
                    )}
                    <span className="min-w-0 flex-1 truncate text-sm">{label}</span>
                    {chat.last_timestamp && (
                      <span className="shrink-0 text-[11px] text-muted-foreground">
                        {chat.last_timestamp.slice(11, 16)}
                      </span>
                    )}
                  </button>
                );
              })}
            </div>
          )}

          <div className="space-y-1">
            <p className="px-1 text-[11px] font-medium uppercase tracking-wide text-muted-foreground">
              {t('messages.contacts')}
            </p>
            {contactsQuery.isLoading ? (
              <div className="flex items-center gap-2 px-2 py-4 text-sm text-muted-foreground">
                <Loader2 className="h-4 w-4 animate-spin" />
                {t('messages.loadingContacts')}
              </div>
            ) : contactsQuery.isError ? (
              <p className="px-2 py-4 text-sm text-destructive">
                {t('messages.errorLoadContacts')}
              </p>
            ) : filteredContacts.length === 0 ? (
              <p className="px-2 py-4 text-sm text-muted-foreground">
                {t('messages.noContacts')}
              </p>
            ) : (
              <div className="max-h-[45vh] overflow-y-auto overscroll-contain rounded-md border border-border">
                {filteredContacts.map((contact) => (
                  <button
                    key={contact.jid}
                    type="button"
                    onClick={() => select(contact.jid)}
                    className="flex w-full items-center gap-3 border-b border-border px-3 py-2 text-left last:border-b-0 hover:bg-accent"
                  >
                    <span className="inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-full bg-muted text-xs font-semibold text-muted-foreground">
                      {contact.is_group ? (
                        <Users className="h-4 w-4" />
                      ) : (
                        contactLabel(contact).charAt(0).toUpperCase() || '?'
                      )}
                    </span>
                    <span className="min-w-0 flex-1">
                      <span className="block truncate text-sm font-medium">
                        {contactLabel(contact)}
                      </span>
                      <span className="block truncate text-[11px] text-muted-foreground">
                        {jidUser(contact.jid)}
                      </span>
                    </span>
                  </button>
                ))}
              </div>
            )}
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
