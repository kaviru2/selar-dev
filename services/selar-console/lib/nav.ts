// nav.ts — the app shell's primary navigation, in one place.
//
// To add a section, append an item. The Topbar renders the list in order,
// highlights the item whose `match` prefixes the current path, and hides
// `adminOnly` items unless the signed-in user has role "admin".
//
// Planned additions (owned by other workstreams) can be appended to the
// list as they become ready.

import type { IconName } from "@/components/ui/Icon";

export interface NavItem {
  key: string;
  label: string;
  href: string;
  icon: IconName;
  /** Path prefixes that mark this item active (defaults to [href]). */
  match?: string[];
  /** Only shown to users whose role is "admin". */
  adminOnly?: boolean;
}

export const NAV_ITEMS: NavItem[] = [
  { key: "library", label: "Library", href: "/library", icon: "book" },
  { key: "reader", label: "Reader", href: "/reader", icon: "doc" },
  { key: "review", label: "Review", href: "/review", icon: "quiz" },
  { key: "progress", label: "Progress", href: "/progress", icon: "graph" },
  { key: "chat", label: "Chat", href: "/chat", icon: "chat" },
  { key: "graph", label: "Graph", href: "/graph", icon: "graph" },
  { key: "quizzes", label: "Quizzes", href: "/quizzes", icon: "quiz" },
  { key: "settings", label: "Settings", href: "/settings", icon: "settings" },
  { key: "admin", label: "Admin", href: "/admin", icon: "shield", adminOnly: true },
];

export function visibleNavItems(items: NavItem[], role?: string | null): NavItem[] {
  return items.filter((item) => !item.adminOnly || role === "admin");
}

export function isActive(item: NavItem, pathname: string): boolean {
  return (item.match ?? [item.href]).some((p) => pathname === p || pathname.startsWith(`${p}/`));
}
