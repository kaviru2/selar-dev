// page.tsx — Root page redirect.
// Redirects unauthenticated users to /library (the default landing view).

import { redirect } from "next/navigation";

export default function RootPage() {
  redirect("/library");
}
