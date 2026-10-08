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
  },
  turbopack: {
    root: path.resolve(__dirname),
  },
};

export default nextConfig;
