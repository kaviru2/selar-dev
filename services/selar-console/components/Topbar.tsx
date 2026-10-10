// Topbar.tsx — SELAR app shell header.
// Navigation comes from lib/nav.ts. Study assignment, cohort labels,
// and research controls are deliberately absent from prototype navigation.

"use client";

import Link from "next/link";
import { clearPracticeCache } from "@/lib/practice-cache";
import { usePathname, useRouter } from "next/navigation";
import { useEffect, useId, useRef, useState } from "react";
import { useSelar } from "@/lib/context";
import { NAV_ITEMS, isActive, visibleNavItems } from "@/lib/nav";
import { Icon } from "@/components/ui/Icon";
import { ThemeToggle } from "@/components/ui/ThemeToggle";
import { Wordmark } from "@/components/ui/Wordmark";

export function Topbar() {
  const pathname = usePathname() ?? "";
  const router = useRouter();
  const { user } = useSelar();
  const items = visibleNavItems(NAV_ITEMS, user?.role);
  const initials = user?.email ? user.email.split("@")[0].slice(0, 2).toUpperCase() : "??";

  const [menuOpen, setMenuOpen] = useState(false);
  const menuId = useId();
  const menuRef = useRef<HTMLDivElement>(null);
  const buttonRef = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    if (!menuOpen) return;
    const onDown = (e: MouseEvent) => {
      if (!menuRef.current?.contains(e.target as Node) && !buttonRef.current?.contains(e.target as Node)) {
        setMenuOpen(false);
      }
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        setMenuOpen(false);
        buttonRef.current?.focus();
      }
    };
    document.addEventListener("mousedown", onDown);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("mousedown", onDown);
      document.removeEventListener("keydown", onKey);
    };
  }, [menuOpen]);

  const handleLogout = async () => {
    await fetch("/api/auth/logout", { method: "POST" });
    clearPracticeCache();
    router.push("/login");
    router.refresh();
  };

  const onReader = pathname === "/reader" || pathname.startsWith("/reader/");

  return (
    <header className="topbar">
      <a className="ui-skip" href="#main">Skip to content</a>
      <div className="topbar-brand">
        <Wordmark href="/library" size={24} />
      </div>

      <nav className="topbar-nav" aria-label="Main">
        {items.map((item) => {
          const active = isActive(item, pathname);
          return (
            <Link key={item.key} href={item.href} className={active ? "active" : ""} aria-current={active ? "page" : undefined}>
              <Icon name={item.icon} size={15} />
              <span className="topbar-nav-label">{item.label}</span>
            </Link>
          );
        })}
      </nav>

      <div className="topbar-meta">
        {onReader && (
          <span className="kbd-hint" aria-label="Keyboard shortcuts: brackets toggle reader panels">
            <span className="kbd">[</span><span className="kbd">]</span> panels
          </span>
        )}

        <ThemeToggle />

        <div className="topbar-account">
          <button
            ref={buttonRef}
            type="button"
            className="topbar-avatar"
            aria-haspopup="menu"
            aria-expanded={menuOpen}
            aria-controls={menuId}
            aria-label={`Account: ${user?.email ?? "signed in"}`}
            onClick={() => setMenuOpen((open) => !open)}
          >
            {initials}
          </button>
          {menuOpen && (
            <div ref={menuRef} id={menuId} role="menu" className="topbar-menu">
              <div className="topbar-menu-who">
                <span>Signed in as</span>
                <strong>{user?.email ?? "unknown"}</strong>
              </div>
              <Link role="menuitem" href="/settings" onClick={() => setMenuOpen(false)}>
                <Icon name="settings" size={14} /> Settings
              </Link>
              <Link role="menuitem" href="/about" onClick={() => setMenuOpen(false)}>
                <Icon name="info" size={14} /> About SELAR
              </Link>
              <button role="menuitem" type="button" onClick={handleLogout}>
                <Icon name="logout" size={14} /> Sign out
              </button>
            </div>
          )}
        </div>
      </div>
    </header>
  );
}
