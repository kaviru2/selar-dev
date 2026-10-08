// Topbar.tsx — SELAR top navigation bar.
// Reads user state from SelarProvider context. Study assignment and
// unfinished assessments are deliberately absent from prototype navigation.

"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useSelar } from "@/lib/context";
import { ThemeToggle } from "@/components/ui/ThemeToggle";
import { WordmarkMark } from "@/components/ui/Wordmark";

const NAV_ITEMS = [
  { key: "library", label: "Library", href: "/library" },
  { key: "reader", label: "Reader", href: "/reader" },
  { key: "chat", label: "Chat", href: "/chat" },
  { key: "graph", label: "Graph", href: "/graph" },
  { key: "quizzes", label: "Quizzes", href: "/quizzes" },
  { key: "settings", label: "Settings", href: "/settings" },
] as const;

export function Topbar() {
  const pathname = usePathname();
  const router = useRouter();
  const { user } = useSelar();
  // Convenience only: /admin is gated server-side (requireAdmin + API 403).
  const navItems = user?.role === "admin" ? [...NAV_ITEMS, { key: "admin", label: "Admin", href: "/admin" }] : NAV_ITEMS;

  // Get initials from email
  const initials = user?.email
    ? user.email.split("@")[0].slice(0, 2).toUpperCase()
    : "??";

  const handleLogout = async () => {
    await fetch("/api/auth/logout", { method: "POST" });
    router.push("/login");
    router.refresh();
  };

  return (
    <div className="topbar">
      <div className="brand">
        <span className="brand-logo"><WordmarkMark size={22} /></span>
        <span>SELAR</span>
      </div>

      <nav className="nav">
        {navItems.map(({ key, label, href }) => {
          const isActive = pathname.startsWith(href);
          return (
            <Link
              key={key}
              href={href}
              className={isActive ? "active" : ""}
            >
              {label}
            </Link>
          );
        })}
      </nav>

      <div style={{ flex: 1 }} />

      <div className="meta">
        <span className="kbd-hint">
          <span className="kbd">J</span>
          <span className="kbd">K</span>
          navigate ·{" "}
          <span className="kbd">Y</span>
          confirm
        </span>

        <ThemeToggle />

        <button
          className="avatar"
          onClick={handleLogout}
          title={`Signed in as ${user?.email ?? "unknown"} — click to sign out`}
          style={{ border: "none", cursor: "pointer" }}
        >
          {initials}
        </button>
      </div>
    </div>
  );
}
