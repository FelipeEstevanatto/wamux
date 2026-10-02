import {
  Info,
  LayoutDashboard,
  MessageSquare,
  Smartphone,
  TerminalSquare,
  type LucideIcon,
} from 'lucide-react';

export interface NavItem {
  to: string;
  labelKey: string;
  icon: LucideIcon;
  end?: boolean;
}

/**
 * Top-level manager routes. Shared by the sidebar and the command palette so
 * the two never drift.
 */
export const navItems: NavItem[] = [
  { to: '/manager', labelKey: 'sidebar.dashboard', icon: LayoutDashboard, end: true },
  { to: '/manager/instances', labelKey: 'sidebar.instances', icon: Smartphone },
  { to: '/manager/messages', labelKey: 'sidebar.messages', icon: MessageSquare },
  { to: '/manager/api-tester', labelKey: 'sidebar.apiTester', icon: TerminalSquare },
  { to: '/manager/about', labelKey: 'sidebar.about', icon: Info },
];
