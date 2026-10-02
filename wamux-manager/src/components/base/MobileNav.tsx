import { Sheet, SheetContent, SheetTitle } from '@/components/ui';
import { SidebarNav } from './Sidebar';
import { useI18n } from '@/i18n/I18nContext';

/**
 * The mobile navigation drawer. Loaded lazily from Layout because the design
 * system's Sheet pulls in Radix Dialog (~90 kB minified), which desktop users
 * never need.
 */
export default function MobileNav({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useI18n();
  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        side="left"
        className="w-72 max-w-[85vw] gap-0 border-sidebar-border bg-sidebar p-0"
      >
        <SheetTitle className="sr-only">{t('common.navigationMenu')}</SheetTitle>
        <SidebarNav onNavigate={() => onOpenChange(false)} />
      </SheetContent>
    </Sheet>
  );
}
