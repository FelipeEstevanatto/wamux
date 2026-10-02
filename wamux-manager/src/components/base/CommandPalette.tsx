import { useEffect, useMemo, useState, type KeyboardEvent as ReactKeyboardEvent } from 'react';
import { useNavigate } from 'react-router-dom';
import { Search } from 'lucide-react';
import { Dialog, DialogContent, DialogTitle } from '@/components/ui';
import useInstancesStore from '@/store/instancesStore';
import { useDarkMode } from '@/hooks/useDarkMode';
import useAuth from '@/hooks/useAuth';
import { navItems } from '@/constants/navigation';
import { useI18n } from '@/i18n/I18nContext';
import { cn } from '@/utils/cn';

interface Command {
  id: string;
  group: string;
  label: string;
  run: () => void;
}

/**
 * ⌘K / Ctrl+K command palette: jump to a page or instance, or run a quick
 * action. Built on the accessible Dialog primitive, so focus is trapped and
 * Escape closes it.
 */
export default function CommandPalette() {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState('');
  const [active, setActive] = useState(0);
  const navigate = useNavigate();
  const { t } = useI18n();
  const { toggleTheme } = useDarkMode();
  const { logout, apiUrl } = useAuth();
  const instances = useInstancesStore((s) => s.instances);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault();
        setOpen((v) => !v);
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, []);

  useEffect(() => {
    if (open) {
      setQuery('');
      setActive(0);
    }
  }, [open]);

  const commands = useMemo<Command[]>(() => {
    const pages: Command[] = navItems.map((n) => ({
      id: `page:${n.to}`,
      group: t('palette.pages'),
      label: t(n.labelKey),
      run: () => navigate(n.to),
    }));
    const insts: Command[] = instances.map((i) => ({
      id: `inst:${i.id}`,
      group: t('palette.instances'),
      label: i.instanceName,
      run: () => navigate('/manager/instances'),
    }));
    const swagger = `${(apiUrl || '').replace(/\/$/, '')}/swagger/index.html`;
    const actions: Command[] = [
      {
        id: 'action:theme',
        group: t('palette.actions'),
        label: t('palette.toggleTheme'),
        run: toggleTheme,
      },
      {
        id: 'action:swagger',
        group: t('palette.actions'),
        label: t('palette.openSwagger'),
        run: () => window.open(swagger, '_blank', 'noopener'),
      },
      {
        id: 'action:logout',
        group: t('palette.actions'),
        label: t('palette.logout'),
        run: logout,
      },
    ];
    return [...pages, ...insts, ...actions];
  }, [instances, navigate, t, toggleTheme, logout, apiUrl]);

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    return q
      ? commands.filter((c) => c.label.toLowerCase().includes(q))
      : commands;
  }, [commands, query]);

  useEffect(() => setActive(0), [query]);

  const runAt = (index: number) => {
    const cmd = filtered[index];
    if (!cmd) return;
    setOpen(false);
    cmd.run();
  };

  const onKeyDown = (e: ReactKeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      setActive((i) => Math.min(i + 1, filtered.length - 1));
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      setActive((i) => Math.max(i - 1, 0));
    } else if (e.key === 'Enter') {
      e.preventDefault();
      runAt(active);
    }
  };

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogContent className="top-[20%] max-w-lg translate-y-0 gap-0 overflow-hidden p-0">
        <DialogTitle className="sr-only">{t('palette.placeholder')}</DialogTitle>
        <div className="flex items-center gap-2 border-b border-sidebar-border px-3">
          <Search className="h-4 w-4 shrink-0 text-muted-foreground" />
          <input
            autoFocus
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={onKeyDown}
            placeholder={t('palette.placeholder')}
            aria-label={t('palette.placeholder')}
            className="h-11 w-full bg-transparent text-sm outline-none placeholder:text-muted-foreground"
          />
        </div>

        <div className="max-h-80 overflow-y-auto p-2">
          {filtered.length === 0 && (
            <p className="px-3 py-6 text-center text-sm text-muted-foreground">
              {t('palette.noResults')}
            </p>
          )}
          {filtered.map((cmd, index) => {
            const showGroup =
              index === 0 || filtered[index - 1].group !== cmd.group;
            return (
              <div key={cmd.id}>
                {showGroup && (
                  <p className="px-3 pb-1 pt-2 text-[11px] font-medium uppercase tracking-wide text-muted-foreground">
                    {cmd.group}
                  </p>
                )}
                <button
                  type="button"
                  onMouseEnter={() => setActive(index)}
                  onClick={() => runAt(index)}
                  className={cn(
                    'flex w-full items-center rounded-md px-3 py-2 text-left text-sm',
                    index === active
                      ? 'bg-accent text-accent-foreground'
                      : 'text-foreground'
                  )}
                >
                  {cmd.label}
                </button>
              </div>
            );
          })}
        </div>

        <div className="border-t border-sidebar-border px-3 py-2 text-[11px] text-muted-foreground">
          {t('palette.hint')}
        </div>
      </DialogContent>
    </Dialog>
  );
}
