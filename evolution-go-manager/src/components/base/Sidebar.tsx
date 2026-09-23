import { NavLink } from 'react-router-dom';
import {
  BookOpen,
  ExternalLink,
  Info,
  LayoutDashboard,
  Smartphone,
  TerminalSquare,
} from 'lucide-react';
import { cn } from '@/utils/cn';
import useAuth from '@/hooks/useAuth';
import useServerStats from '@/hooks/useServerStats';
import GithubIcon from './GithubIcon';
import {
  COPYRIGHT_LINE,
  FORK_DISCLAIMER,
  FORK_OF_NAME,
  FORK_REPO,
  PRODUCT_NAME,
  UPSTREAM_REPO,
} from '@/constants/branding';

// Dashboard/Instâncias/Sobre are SPA routes; API Tester is also a route, and
// Swagger is served by the Go server (outside the SPA), so it is a plain link.
const navItems = [
  { to: '/manager', label: 'Dashboard', icon: LayoutDashboard, end: true },
  { to: '/manager/instances', label: 'Instâncias', icon: Smartphone },
  { to: '/manager/api-tester', label: 'API Tester', icon: TerminalSquare },
  { to: '/manager/about', label: 'Sobre', icon: Info },
];

const itemClass = (isActive: boolean) =>
  cn(
    'flex items-center gap-3 rounded-md px-3 py-2.5 transition-all',
    isActive
      ? 'bg-primary/10 text-primary'
      : 'text-muted-foreground hover:bg-accent hover:text-foreground'
  );

/**
 * The sidebar's contents. Rendered both by the fixed desktop sidebar and inside
 * the mobile drawer, so the two can never drift. `onNavigate` lets the drawer
 * close itself when an item is picked.
 */
export function SidebarNav({ onNavigate }: { onNavigate?: () => void }) {
  const { stats } = useServerStats();
  const { apiUrl } = useAuth();
  const version = stats?.system?.version;

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
        title="Ir para o Dashboard"
        className="flex h-16 flex-col items-start justify-center border-b border-sidebar-border px-4 transition-colors hover:bg-sidebar-accent/50"
      >
        <h2 className="text-lg font-bold leading-tight text-primary">
          {PRODUCT_NAME}
        </h2>
        <span className="text-[10px] uppercase tracking-wide text-muted-foreground">
          fork de {FORK_OF_NAME}
        </span>
      </NavLink>

      {/* Navigation Menu */}
      <nav className="flex-1 space-y-1.5 overflow-y-auto px-2 py-4">
        {navItems.map((item) => (
          <NavLink
            key={item.to}
            to={item.to}
            end={item.end}
            onClick={onNavigate}
            className={({ isActive }) => itemClass(isActive)}
          >
            {({ isActive }) => (
              <>
                <item.icon
                  className={cn('h-5 w-5 flex-shrink-0', isActive && 'text-primary')}
                />
                <span className="font-medium">{item.label}</span>
              </>
            )}
          </NavLink>
        ))}

        <a
          href={swaggerHref}
          target="_blank"
          rel="noreferrer noopener"
          onClick={onNavigate}
          className={itemClass(false)}
          title="Abrir o Swagger da API em nova aba"
        >
          <BookOpen className="h-5 w-5 flex-shrink-0" />
          <span className="font-medium">Swagger</span>
          <ExternalLink className="ml-auto h-3.5 w-3.5 opacity-60" />
        </a>
      </nav>

      {/* Sidebar Footer */}
      <div className="mt-auto space-y-2 border-t border-sidebar-border p-4">
        <div className="text-sm font-medium text-primary">{PRODUCT_NAME}</div>
        <div className="text-xs text-muted-foreground">
          {version ? `versão ${version}` : 'versão —'}
        </div>
        <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
          <a
            href={FORK_REPO}
            target="_blank"
            rel="noreferrer noopener"
            className="inline-flex items-center gap-1.5 text-xs text-muted-foreground transition-colors hover:text-foreground"
            title="Repositório deste fork"
          >
            <GithubIcon className="h-3.5 w-3.5" />
            Fork
          </a>
          <a
            href={UPSTREAM_REPO}
            target="_blank"
            rel="noreferrer noopener"
            className="inline-flex items-center gap-1.5 text-xs text-muted-foreground transition-colors hover:text-foreground"
            title={`Projeto upstream (${FORK_OF_NAME})`}
          >
            Upstream
          </a>
        </div>
        <p className="text-[11px] leading-snug text-muted-foreground">
          {FORK_DISCLAIMER}
        </p>
        <div className="text-[11px] text-muted-foreground">{COPYRIGHT_LINE}</div>
      </div>
    </div>
  );
}

/**
 * The fixed sidebar. Hidden below `md`, where Layout opens the same content in a
 * drawer instead.
 */
function Sidebar() {
  return (
    <aside className="hidden w-56 shrink-0 border-r border-sidebar-border md:block">
      <SidebarNav />
    </aside>
  );
}

export default Sidebar;
