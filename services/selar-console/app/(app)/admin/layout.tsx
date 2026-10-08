// admin/layout.tsx — server-side gate for everything under /admin.
// requireAdmin() asks the Go API for the current user (role read from the
// database) and calls forbidden() for non-admins, so the area is protected on
// the server, not just hidden from the navigation.

import { AdminTabs } from "@/components/admin/AdminTabs";
import { requireAdmin } from "@/lib/admin-server";
import "./admin.css";

export const dynamic = "force-dynamic";

export default async function AdminLayout({ children }: { children: React.ReactNode }) {
  await requireAdmin();
  return (
    <div className="adm-shell">
      <nav className="adm-tabs" aria-label="Admin sections">
        <AdminTabs />
      </nav>
      {children}
    </div>
  );
}
