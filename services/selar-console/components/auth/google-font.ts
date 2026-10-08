// google-font.ts — Roboto Medium for the Google sign-in button only
// (Google's branding guidelines require it). Self-hosted by next/font.
import { Roboto } from "next/font/google";

export const roboto = Roboto({ weight: "500", subsets: ["latin"], display: "swap" });
