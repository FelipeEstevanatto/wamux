import { ReactNode, Suspense, lazy, useEffect, useState } from 'react';
import { Outlet } from 'react-router-dom';
import Sidebar from './Sidebar';
import Header from './Header';

// The drawer (and with it Radix Dialog) is only needed once the menu is opened,
// so it is kept out of the initial bundle.
const MobileNav = lazy(() => import('./MobileNav'));

interface LayoutProps {
  children?: ReactNode;
}

function Layout({ children }: LayoutProps) {
  const [menuOpen, setMenuOpen] = useState(false);
  // Once opened, keep the drawer mounted so its close animation can play. This
  // is derived during render rather than via an effect, so opening the menu does
  // not trigger an extra render pass.
  const [menuOpenedOnce, setMenuOpenedOnce] = useState(false);
  const menuMounted = menuOpen || menuOpenedOnce;

  const handleMenuOpenChange = (open: boolean) => {
    setMenuOpen(open);
    if (open) setMenuOpenedOnce(true);
  };

  // On phones the drawer is the only navigation, so fetch its chunk up front
  // (without rendering it) and it opens instantly on the first tap.
  useEffect(() => {
    if (window.matchMedia('(max-width: 767px)').matches) {
      void import('./MobileNav');
    }
  }, []);

  return (
    <div className="flex h-screen bg-background">
      <Sidebar />

      {/* Mobile navigation. The fixed sidebar is hidden below `md`, so without
          this drawer there would be no way to switch pages on a phone. It reuses
          SidebarNav so the two can never drift. */}
      <Suspense fallback={null}>
        {menuMounted && (
          <MobileNav open={menuOpen} onOpenChange={handleMenuOpenChange} />
        )}
      </Suspense>

      <div className="flex min-w-0 flex-1 flex-col overflow-hidden">
        <Header onOpenMenu={() => handleMenuOpenChange(true)} />
        <main className="flex-1 overflow-y-auto">{children || <Outlet />}</main>
      </div>
    </div>
  );
}

export default Layout;
