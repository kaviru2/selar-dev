// layout.tsx — App shell layout for all authenticated views.
// Fetches the user server-side via getSession(), wraps children
// in SelarProvider for global state, and renders the Topbar.

import { Topbar } from "@/components/Topbar";
import { SelarProvider } from "@/lib/context";
import { getSession } from "@/lib/auth";

export default async function AppLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const user = await getSession();

  return (
    <SelarProvider initialUser={user}>
      <div className="selar-app">
        <Topbar />
        {children}
      </div>
    </SelarProvider>
  );
}
