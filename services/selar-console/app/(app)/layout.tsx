// layout.tsx — App shell layout for all authenticated views.
// Fetches the user server-side via getSession(), wraps children
// in SelarProvider for global state, and renders the Topbar.

import { Topbar } from "@/components/Topbar";
import { AnalyticsProvider, ConsentBanner } from "@/components/AnalyticsProvider";
import { SelarProvider } from "@/lib/context";
import { redirect } from "next/navigation";
import { getSessionStatus } from "@/lib/auth";

export default async function AppLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const session = await getSessionStatus();
  // A rejected token must be cleared, otherwise the proxy would keep sending
  // /login back into the app. Server Components cannot set cookies, so hand
  // off to the route handler that expires selar_token and redirects to /login.
  if (session.status === "invalid") {
    redirect("/api/auth/session-expired");
  }
  const user = session.status === "authenticated" ? session.user : null;

  return (
    <SelarProvider initialUser={user}>
      <AnalyticsProvider user={user}>
        <div className="selar-app">
          <Topbar />
          <ConsentBanner />
          {children}
        </div>
      </AnalyticsProvider>
    </SelarProvider>
  );
}
