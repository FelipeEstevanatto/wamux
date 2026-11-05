import { type FocusEvent, type MouseEvent, useEffect, useState } from 'react';
import { NavLink } from 'react-router-dom';
import { BookOpen, ExternalLink, PanelLeftClose, PanelLeftOpen } from 'lucide-react';
import { cn } from '@/utils/cn';
import { navItems } from '@/constants/navigation';
import useAuth from '@/hooks/useAuth';
import useServerStats from '@/hooks/useServerStats';
import { useSidebarCollapsed } from '@/hooks/useSidebarCollapsed';
import GithubIcon from './GithubIcon';
import { COPYRIGHT_LINE, PRODUCT_NAME, REPO } from '@/constants/branding';
import { prefetchLikelyRoutes, prefetchRoute } from '@/utils/prefetch';
import { useI18n } from '@/i18n/I18nContext';

// Dashboard/Instâncias/Sobre are SPA routes; API Tester is also a route, and
// Swagger is served by the Go server (outside the SPA), so it is a plain link.
// The nav items live in `@/constants/navigation` (shared with the palette).

const itemClass = (isActive: boolean, collapsed = false) =>
  cn(
    'flex items-center rounded-md py-2.5 transition-all',
    collapsed ? 'justify-center px-0' : 'gap-3 px-3',
    isActive
      ? 'bg-primary/10 text-primary'
      : 'text-muted-foreground hover:bg-accent hover:text-foreground'
  );

/**
 * The sidebar's contents. Rendered both by the fixed desktop sidebar and inside
 * the mobile drawer, so the two can never drift. `onNavigate` lets the drawer
 * close itself when an item is picked. `collapsed` shows only the icons, each
 * with its title on hover or focus; the drawer never uses it.
 */
export function SidebarNav({
  onNavigate,
  collapsed = false,
  onToggleCollapsed,
}: {
  onNavigate?: () => void;
  collapsed?: boolean;
  onToggleCollapsed?: () => void;
}) {
  const { t } = useI18n();
  // Fixed position, because the nav scrolls and would clip anything inside it.
  const [tip, setTip] = useState<{ label: string; top: number; left: number } | null>(null);

  const showTip = (label: string) => (e: MouseEvent<HTMLElement> | FocusEvent<HTMLElement>) => {
    if (!collapsed) return;
    const box = e.currentTarget.getBoundingClientRect();
    setTip({ label, top: box.top + box.height / 2, left: box.right + 10 });
  };
  const hideTip = () => setTip(null);
  const { stats } = useServerStats();
  const { apiUrl } = useAuth();
  const version = stats?.system?.version;

  // Warm the most likely next route chunks once the app is idle.
  useEffect(() => {
    prefetchLikelyRoutes();
  }, []);

  const swaggerHref = apiUrl
    ? `${apiUrl.replace(/\/$/, '')}/swagger/index.html`
    : '/swagger/index.html';

  return (
    <div className="flex h-full w-full flex-col bg-sidebar text-sidebar-foreground">
      {/* Logo Header — links back to the Dashboard */}
      <NavLink
        to="/manager"
        end
        onClick={onNavigate}
        onMouseEnter={() => prefetchRoute('/manager')}
        onFocus={() => prefetchRoute('/manager')}
        title={t('sidebar.homeTitle')}
        className={cn(
          'flex h-16 items-center border-b border-sidebar-border transition-colors hover:bg-sidebar-accent/50',
          collapsed ? 'justify-center' : 'px-4'
        )}
      >
        <h2 className="text-lg font-bold leading-tight text-primary">
          {collapsed ? PRODUCT_NAME.charAt(0) : PRODUCT_NAME}
        </h2>
      </NavLink>

      {/* Navigation Menu */}
      <nav className="flex-1 space-y-1.5 overflow-y-auto px-2 py-4">
        {navItems.map((item) => (
          <NavLink
            key={item.to}
            to={item.to}
            end={item.end}
            onClick={onNavigate}
            onMouseEnter={(e) => {
              prefetchRoute(item.to);
              showTip(t(item.labelKey))(e);
            }}
            onMouseLeave={hideTip}
            onFocus={(e) => {
              prefetchRoute(item.to);
              showTip(t(item.labelKey))(e);
            }}
            onBlur={hideTip}
            className={({ isActive }) => itemClass(isActive, collapsed)}
          >
            {({ isActive }) => (
              <>
                <item.icon
                  className={cn('h-5 w-5 shrink-0', isActive && 'text-primary')}
                />
                <span className={collapsed ? 'sr-only' : 'font-medium'}>
                  {t(item.labelKey)}
                </span>
              </>
            )}
          </NavLink>
        ))}

        <a
          href={swaggerHref}
          target="_blank"
          rel="noreferrer noopener"
          onClick={onNavigate}
          className={itemClass(false, collapsed)}
          title={collapsed ? undefined : t('sidebar.swaggerTitle')}
          onMouseEnter={showTip(t('sidebar.swagger'))}
          onMouseLeave={hideTip}
          onFocus={showTip(t('sidebar.swagger'))}
          onBlur={hideTip}
        >
          <BookOpen className="h-5 w-5 shrink-0" />
          <span className={collapsed ? 'sr-only' : 'font-medium'}>{t('sidebar.swagger')}</span>
          {!collapsed && <ExternalLink className="ml-auto h-3.5 w-3.5 opacity-60" />}
        </a>
      </nav>

      {onToggleCollapsed && (
        <div className="border-t border-sidebar-border p-2">
          <button
            type="button"
            onClick={() => {
              hideTip();
              onToggleCollapsed();
            }}
            onMouseEnter={showTip(t('sidebar.expand'))}
            onMouseLeave={hideTip}
            onFocus={showTip(t('sidebar.expand'))}
            onBlur={hideTip}
            aria-expanded={!collapsed}
            aria-label={collapsed ? t('sidebar.expand') : t('sidebar.collapse')}
            className={cn(itemClass(false, collapsed), 'w-full text-sm')}
          >
            {collapsed ? (
              <PanelLeftOpen className="h-5 w-5 shrink-0" />
            ) : (
              <>
                <PanelLeftClose className="h-5 w-5 shrink-0" />
                <span className="font-medium">{t('sidebar.collapse')}</span>
              </>
            )}
          </button>
        </div>
      )}

      {/* Sidebar Footer */}
      {!collapsed && (
      <div className="mt-auto space-y-2 border-t border-sidebar-border p-4">
        <div className="text-sm font-medium text-primary">{PRODUCT_NAME}</div>
        <div className="text-xs text-muted-foreground">
          {version ? t('sidebar.version', { version }) : t('sidebar.versionEmpty')}
        </div>
        <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
          <a
            href={REPO}
            target="_blank"
            rel="noreferrer noopener"
            className="inline-flex items-center gap-1.5 text-xs text-muted-foreground transition-colors hover:text-foreground"
            title={t('sidebar.repoTitle')}
          >
            <GithubIcon className="h-3.5 w-3.5" />
            {t('sidebar.github')}
          </a>
        </div>
        <div className="text-[11px] text-muted-foreground">{COPYRIGHT_LINE}</div>
      </div>
      )}

      {collapsed && tip && (
        <div
          role="tooltip"
          style={{ top: tip.top, left: tip.left }}
          className="pointer-events-none fixed z-50 -translate-y-1/2 whitespace-nowrap rounded-md border border-border bg-popover px-2.5 py-1.5 text-xs font-medium text-popover-foreground shadow-md"
        >
          {tip.label}
        </div>
      )}
    </div>
  );
}

/**
 * The fixed sidebar. Hidden below `md`, where Layout opens the same content in a
 * drawer instead.
 */
function Sidebar() {
  const { collapsed, toggle } = useSidebarCollapsed();
  return (
    <aside
      className={cn(
        'hidden shrink-0 border-r border-sidebar-border transition-[width] duration-200 md:block',
        collapsed ? 'w-16' : 'w-56'
      )}
    >
      <SidebarNav collapsed={collapsed} onToggleCollapsed={toggle} />
    </aside>
  );
}

export default Sidebar;
