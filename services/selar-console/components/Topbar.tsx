// Topbar.tsx — SELAR top navigation bar.
// Reads user state from SelarProvider context. Shows real initials
// and cohort assignment instead of hardcoded values.

"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useSelar } from "@/lib/context";

const NAV_ITEMS = [
  { key: "library", label: "Library", href: "/library" },
  { key: "reader", label: "Reader", href: "/reader" },
  { key: "graph", label: "Graph", href: "/graph" },
  { key: "quiz", label: "Quiz", href: "/quiz" },
  { key: "settings", label: "Settings", href: "/settings" },
] as const;

const COHORT_CONFIG = {
  control: { label: "Control", color: "#9a938a" },
  treatment_auto: { label: "Auto-link", color: "#7a8c5c" },
  treatment_hitl: { label: "HITL", color: "#c96442" },
} as const;

export function Topbar() {
  const pathname = usePathname();
  const router = useRouter();
  const { user } = useSelar();

  const cohort = user?.cohort ?? "treatment_hitl";
  const cohortInfo = COHORT_CONFIG[cohort];

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
        <span className="brand-mark" />
        <span>SELAR</span>
      </div>

      <nav className="nav">
        {NAV_ITEMS.map(({ key, label, href }) => {
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

        <span
          className="cohort-chip"
          style={{ borderColor: cohortInfo.color + "66" }}
        >
          <span
            className="dot"
            style={{ background: cohortInfo.color }}
          />
          {cohortInfo.label}
        </span>

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
