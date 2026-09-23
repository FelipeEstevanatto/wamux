import { LogOut, Menu, Moon, Sun } from 'lucide-react';
import useAuth from '@/hooks/useAuth';
import { useDarkMode } from '@/hooks/useDarkMode';
import { PRODUCT_NAME } from '@/constants/branding';

/**
 * Top bar. API Tester and Swagger now live in the sidebar (they are separate
 * pages, so they belong with the other navigation), which leaves this bar with
 * the mobile menu button, the theme toggle and logout.
 */
function Header({ onOpenMenu }: { onOpenMenu?: () => void }) {
  const { logout } = useAuth();
  const { theme, toggleTheme } = useDarkMode();

  return (
    <header className="flex h-16 items-center gap-2 border-b border-sidebar-border bg-sidebar px-3 py-3 shadow-sm sm:px-4">
      {/* Left: menu + product name, shown only where the sidebar is hidden. */}
      <div className="flex min-w-0 items-center gap-2 md:hidden">
        <button
          type="button"
          onClick={onOpenMenu}
          className="inline-flex h-10 w-10 shrink-0 items-center justify-center rounded-md text-sidebar-foreground transition-colors hover:bg-sidebar-accent"
          aria-label="Abrir menu"
        >
          <Menu className="h-5 w-5" />
        </button>
        <span className="truncate text-base font-bold text-primary">
          {PRODUCT_NAME}
        </span>
      </div>

      {/* Right */}
      <div className="ml-auto flex items-center gap-1 sm:gap-2">
        <button
          type="button"
          onClick={toggleTheme}
          className="flex items-center gap-2 rounded-md px-3 py-2 text-sm font-medium text-sidebar-foreground transition-colors hover:bg-sidebar-accent"
          title={theme === 'dark' ? 'Modo claro' : 'Modo escuro'}
          aria-label={theme === 'dark' ? 'Ativar modo claro' : 'Ativar modo escuro'}
        >
          {theme === 'dark' ? (
            <Sun className="h-4 w-4" />
          ) : (
            <Moon className="h-4 w-4" />
          )}
        </button>

        <button
          type="button"
          onClick={logout}
          className="flex items-center gap-2 rounded-md px-3 py-2 text-sm font-medium text-sidebar-foreground transition-colors hover:bg-sidebar-accent"
          title="Sair"
        >
          <LogOut className="h-4 w-4" />
          <span className="hidden sm:inline">Sair</span>
        </button>
      </div>
    </header>
  );
}

export default Header;
