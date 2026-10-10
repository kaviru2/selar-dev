// next.config.ts — Next.js 16 configuration for SELAR console.
// Uses standalone output for Docker deployment and React Compiler for
// automatic memoization.

import type { NextConfig } from "next";
import path from "path";

const nextConfig: NextConfig = {
  output: "standalone",
  reactCompiler: true,
  // forbidden() for the server-side /admin role gate (403 page).
  experimental: {
    authInterrupts: true,
    // Reuse a visited page's RSC payload for 30 s in the browser's own router
    // cache, so Library -> Quizzes -> back does not refetch every page.
    // Client-side and per-tab only (never a shared/CDN cache); mutations
    // already call router.refresh(), which invalidates it.
    staleTimes: { dynamic: 30 },
  },
  turbopack: {
    root: path.resolve(__dirname),
  },
};

export default nextConfig;
