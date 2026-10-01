import { Sheet, SheetContent, SheetTitle } from '@/components/ui';
import { SidebarNav } from './Sidebar';

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
  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        side="left"
        className="w-72 max-w-[85vw] gap-0 border-sidebar-border bg-sidebar p-0"
      >
        <SheetTitle className="sr-only">Menu de navegação</SheetTitle>
        <SidebarNav onNavigate={() => onOpenChange(false)} />
      </SheetContent>
    </Sheet>
  );
}
