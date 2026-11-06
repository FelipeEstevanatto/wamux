import { useEffect, useMemo, useState } from 'react';
import { useInfiniteQuery } from '@tanstack/react-query';
import { Search, X } from 'lucide-react';

import { Button, Input } from '@/components/ui';
import { useI18n } from '@/i18n/I18nContext';
import * as messagesApi from '@/services/api/messages';
import { highlightParts, searchTarget, snippetAround } from '@/utils/messageSearch';

const PAGE_SIZE = 30;
const MIN_LENGTH = 2;
const DEBOUNCE_MS = 300;

/**
 * Searches the open conversation on the server, so it also finds messages that
 * were never loaded. `onOpen` receives the id of the message to jump to.
 */
function ThreadSearch({
  token,
  chat,
  onOpen,
  onClose,
}: {
  token: string;
  chat: string;
  onOpen: (messageId: string) => void;
  onClose: () => void;
}) {
  const { t } = useI18n();
  const [text, setText] = useState('');
  const [query, setQuery] = useState('');
  const [showResults, setShowResults] = useState(true);

  useEffect(() => {
    const timer = window.setTimeout(() => setQuery(text.trim()), DEBOUNCE_MS);
    return () => window.clearTimeout(timer);
  }, [text]);

  const search = useInfiniteQuery({
    queryKey: ['messages', 'search', token, chat, query],
    queryFn: ({ pageParam }) =>
      messagesApi.searchHistory(token, chat, query, PAGE_SIZE, pageParam as string | undefined),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (lastPage) =>
      lastPage.length < PAGE_SIZE ? undefined : lastPage[lastPage.length - 1]?.timestamp,
    enabled: query.length >= MIN_LENGTH,
    staleTime: 30_000,
  });

  const results = useMemo(() => {
    const seen = new Set<string>();
    return (search.data?.pages.flat() ?? []).filter((m) => {
      if (seen.has(m.message_id)) return false;
      seen.add(m.message_id);
      return true;
    });
  }, [search.data]);

  const searching = query.length >= MIN_LENGTH;

  return (
    <div className="relative shrink-0 border-b border-border bg-background p-2">
      <div className="flex items-center gap-2">
        <Search className="h-4 w-4 shrink-0 text-muted-foreground" />
        <Input
          autoFocus
          type="search"
          value={text}
          onChange={(e) => {
            setText(e.target.value);
            setShowResults(true);
          }}
          onFocus={() => setShowResults(true)}
          onKeyDown={(e) => {
            if (e.key === 'Escape') onClose();
          }}
          placeholder={t('messages.searchInChat')}
          aria-label={t('messages.searchInChat')}
          className="h-8"
        />
        <Button
          variant="ghost"
          size="icon"
          onClick={onClose}
          title={t('common.close')}
          aria-label={t('common.close')}
        >
          <X />
        </Button>
      </div>

      {searching && showResults && (
        <div className="absolute inset-x-2 top-full z-20 mt-1 max-h-80 overflow-y-auto rounded-md border border-border bg-background shadow-lg">
          {search.isLoading ? (
            <p className="p-3 text-sm text-muted-foreground">{t('common.loading')}</p>
          ) : search.isError ? (
            <p className="p-3 text-sm text-destructive">{t('messages.searchError')}</p>
          ) : results.length === 0 ? (
            <p className="p-3 text-sm text-muted-foreground">{t('messages.searchEmpty')}</p>
          ) : (
            <ul>
              {results.map((message) => (
                <li key={message.message_id} className="border-b border-border last:border-b-0">
                  <button
                    type="button"
                    onClick={() => {
                      setShowResults(false);
                      onOpen(searchTarget(message));
                    }}
                    className="w-full px-3 py-2 text-left text-sm hover:bg-accent"
                  >
                    <span className="block text-xs text-muted-foreground">{message.timestamp}</span>
                    <span className="block break-words">
                      {highlightParts(snippetAround(message.text_content, query), query).map(
                        (part, index) =>
                          index % 2 === 1 ? (
                            <mark key={index} className="rounded-sm bg-primary/25 text-foreground">
                              {part}
                            </mark>
                          ) : (
                            part
                          )
                      )}
                    </span>
                  </button>
                </li>
              ))}
            </ul>
          )}
          {search.hasNextPage && (
            <div className="p-2 text-center">
              <Button
                variant="outline"
                size="sm"
                disabled={search.isFetchingNextPage}
                onClick={() => void search.fetchNextPage()}
              >
                {search.isFetchingNextPage ? t('common.loading') : t('messages.loadOlder')}
              </Button>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

export default ThreadSearch;
