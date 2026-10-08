"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";

const TABS = [
  { href: "/admin", label: "Overview", match: (p: string) => p === "/admin" },
  { href: "/admin/users", label: "Users", match: (p: string) => p.startsWith("/admin/users") },
  { href: "/admin/quizzes", label: "Quizzes", match: (p: string) => p.startsWith("/admin/quiz") },
];

export function AdminTabs() {
  const pathname = usePathname() || "";
  return (
    <>
      {TABS.map((t) => (
        <Link key={t.href} href={t.href} className={t.match(pathname) ? "on" : ""} aria-current={t.match(pathname) ? "page" : undefined}>
          {t.label}
        </Link>
      ))}
    </>
  );
}
