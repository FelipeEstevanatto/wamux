import { Languages, LogOut, Menu, Moon, Sun } from 'lucide-react';
import { Link } from 'react-router-dom';
import useAuth from '@/hooks/useAuth';
import { useDarkMode } from '@/hooks/useDarkMode';
import { PRODUCT_NAME } from '@/constants/branding';
import { useI18n } from '@/i18n/I18nContext';
import { LANGUAGES } from '@/i18n/translations';

/**
 * Top bar. API Tester and Swagger now live in the sidebar (they are separate
 * pages, so they belong with the other navigation), which leaves this bar with
 * the mobile menu button, the language switcher, the theme toggle and logout.
 */
function Header({ onOpenMenu }: { onOpenMenu?: () => void }) {
  const { logout } = useAuth();
  const { theme, toggleTheme } = useDarkMode();
  const { lang, setLang, t } = useI18n();

  const nextLang = LANGUAGES[(LANGUAGES.findIndex((l) => l.id === lang) + 1) % LANGUAGES.length];
  const iconButton =
    'flex items-center gap-2 rounded-md px-3 py-2 text-sm font-medium text-sidebar-foreground transition-colors hover:bg-sidebar-accent';

  return (
    <header className="flex h-16 items-center gap-2 border-b border-sidebar-border bg-sidebar px-3 py-3 shadow-sm sm:px-4">
      {/* Left: menu + product name, shown only where the sidebar is hidden. */}
      <div className="flex min-w-0 items-center gap-2 md:hidden">
        <button
          type="button"
          onClick={onOpenMenu}
          className="inline-flex h-10 w-10 shrink-0 items-center justify-center rounded-md text-sidebar-foreground transition-colors hover:bg-sidebar-accent"
          aria-label={t('header.openMenu')}
        >
          <Menu className="h-5 w-5" />
        </button>
        <Link
          to="/manager"
          className="truncate text-base font-bold text-primary transition-colors hover:text-primary/80"
          title={t('sidebar.homeTitle')}
        >
          {PRODUCT_NAME}
        </Link>
      </div>

      {/* Right */}
      <div className="ml-auto flex items-center gap-1 sm:gap-2">
        <button
          type="button"
          onClick={() => setLang(nextLang.id)}
          className={iconButton}
          title={`${t('header.language')}: ${nextLang.label}`}
          aria-label={`${t('header.language')}: ${nextLang.label}`}
        >
          <Languages className="h-4 w-4" />
          <span className="hidden text-xs uppercase sm:inline">{lang}</span>
        </button>

        <button
          type="button"
          onClick={toggleTheme}
          className={iconButton}
          title={theme === 'dark' ? t('header.lightMode') : t('header.darkMode')}
          aria-label={theme === 'dark' ? t('header.lightMode') : t('header.darkMode')}
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
          className={iconButton}
          title={t('header.logout')}
        >
          <LogOut className="h-4 w-4" />
          <span className="hidden sm:inline">{t('header.logout')}</span>
        </button>
      </div>
    </header>
  );
}

export default Header;
