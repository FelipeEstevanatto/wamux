export type ChatKind = 'contact' | 'group' | 'channel';

type Translate = (key: string, vars?: Record<string, string | number>) => string;

/** The user part of a JID, without the device suffix or the domain. */
export function jidUser(jid: string): string {
  if (!jid) return '';
  return jid.split('@')[0].split(':')[0] || jid;
}

export function chatKind(jid: string): ChatKind {
  if (jid.endsWith('@g.us')) return 'group';
  if (jid.endsWith('@newsletter')) return 'channel';
  return 'contact';
}

/**
 * Title is the saved name. Detail is the number, or the id when the name is
 * unknown, so the inbox never uses a raw JID as the title.
 * A named group or channel keeps the subject alone: the id is not a number.
 */
export function chatHeading(
  jid: string,
  name: string | undefined,
  t: Translate
): { title: string; detail: string } {
  if (!jid) return { title: t('messages.chatFallback'), detail: '' };

  const id = jidUser(jid);
  const trimmed = name?.trim() ?? '';
  const named = Boolean(trimmed) && trimmed !== id && trimmed !== jid;
  const kind = chatKind(jid);

  if (named) {
    return { title: trimmed, detail: kind === 'contact' ? id : '' };
  }

  const title =
    kind === 'group'
      ? t('messages.unknownGroup')
      : kind === 'channel'
        ? t('messages.unknownChannel')
        : t('messages.unknownContact');
  return { title, detail: id };
}
