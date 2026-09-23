import { NavLink } from 'react-router-dom';
import { LayoutDashboard, Smartphone, Info } from 'lucide-react';
import { cn } from '@/utils/cn';
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

const navItems = [
  { to: '/manager', label: 'Dashboard', icon: LayoutDashboard },
  { to: '/manager/instances', label: 'Instâncias', icon: Smartphone },
  { to: '/manager/about', label: 'Sobre', icon: Info },
];

function Sidebar() {
  const { stats } = useServerStats();
  const version = stats?.system?.version;

  return (
    <div className="hidden md:flex bg-sidebar text-sidebar-foreground flex-col w-56 border-r border-sidebar-border">
      {/* Logo Header */}
      <div className="h-16 flex flex-col items-start justify-center px-4 border-b border-sidebar-border">
        <h2 className="text-lg font-bold text-primary leading-tight">
          {PRODUCT_NAME}
        </h2>
        <span className="text-[10px] uppercase tracking-wide text-muted-foreground">
          fork de {FORK_OF_NAME}
        </span>
      </div>

      {/* Navigation Menu */}
      <nav className="space-y-1.5 flex-1 px-2 py-4">
        {navItems.map((item) => (
          <NavLink
            key={item.to}
            to={item.to}
            end={item.to === '/manager'}
            className={({ isActive }) =>
              cn(
                'flex items-center gap-3 px-3 py-2.5 rounded-md transition-all',
                isActive
                  ? 'bg-primary/10 text-primary'
                  : 'text-muted-foreground hover:text-foreground hover:bg-accent'
              )
            }
          >
            {({ isActive }) => (
              <>
                <item.icon className={cn('flex-shrink-0 h-5 w-5', isActive && 'text-primary')} />
                <span className="font-medium">{item.label}</span>
              </>
            )}
          </NavLink>
        ))}
      </nav>

      {/* Sidebar Footer */}
      <div className="mt-auto p-4 border-t border-sidebar-border space-y-2">
        <div className="text-sm text-primary font-medium">{PRODUCT_NAME}</div>
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

export default Sidebar;
