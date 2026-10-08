// manifest.ts — Web app manifest (Next.js metadata route, served at /manifest.webmanifest).
// Icons are the SELAR brand mark (Linny the linnet); sources live in brand/ at the repo root.

import type { MetadataRoute } from "next";

export default function manifest(): MetadataRoute.Manifest {
  return {
    name: "SELAR: Semantic Linking for Active Retention",
    short_name: "SELAR",
    description:
      "Research prototype: a PDF reader that suggests links to your earlier reading and asks you to explain and decide.",
    start_url: "/",
    display: "standalone",
    background_color: "#F1F5F9",
    theme_color: "#1F5135",
    icons: [
      { src: "/icon-192.png", sizes: "192x192", type: "image/png" },
      { src: "/icon-512.png", sizes: "512x512", type: "image/png" },
      { src: "/icon-maskable-512.png", sizes: "512x512", type: "image/png", purpose: "maskable" },
    ],
  };
}
