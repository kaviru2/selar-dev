// page.tsx — Public landing page.
// proxy.ts lets signed-out visitors through to "/" and sends signed-in
// users to the app; the same page is reachable by anyone at /about.

import { LandingPage } from "@/components/landing/LandingPage";

export default function Home() {
  return <LandingPage />;
}
