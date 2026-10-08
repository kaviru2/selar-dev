// layout.tsx — Root layout for the SELAR console.
// Loads the typefaces via next/font (Figtree for UI, Fraunces for display
// headings, Source Serif 4 for reading text, JetBrains Mono for code/meta),
// imports the design system CSS, sets SEO metadata, and applies the
// colour scheme before first paint (lib/theme.tsx).
// Fonts are exposed as CSS variables and consumed by app/tokens.css, so a
// brand font swap only needs changes here and in the --selar-font-* tokens.

import type { Metadata } from "next";
import { Figtree, Fraunces, Source_Serif_4, JetBrains_Mono } from "next/font/google";
import "./globals.css";
import { ThemeProvider, THEME_INIT_SCRIPT } from "@/lib/theme";

const figtree = Figtree({
  variable: "--font-figtree",
  subsets: ["latin"],
  display: "swap",
});

const fraunces = Fraunces({
  variable: "--font-fraunces",
  subsets: ["latin"],
  display: "swap",
  axes: ["SOFT", "opsz"],
});

const sourceSerif = Source_Serif_4({
  variable: "--font-source-serif",
  subsets: ["latin"],
  display: "swap",
});

const jetbrainsMono = JetBrains_Mono({
  variable: "--font-jetbrains",
  subsets: ["latin"],
  display: "swap",
});

export const metadata: Metadata = {
  title: "SELAR — Semantic Linking for Active Retention",
  description:
    "A PDF reader that surfaces semantic links across your library. Confirm, reject, or relabel AI-identified connections to strengthen retention through retrieval practice.",
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html
      lang="en"
      className={`${figtree.variable} ${fraunces.variable} ${sourceSerif.variable} ${jetbrainsMono.variable}`}
      // The inline script sets data-theme before hydration.
      suppressHydrationWarning
    >
      <head>
        {/* Applies the saved/system colour scheme before first paint (no flash). */}
        <script dangerouslySetInnerHTML={{ __html: THEME_INIT_SCRIPT }} />
      </head>
      <body>
        <ThemeProvider>{children}</ThemeProvider>
      </body>
    </html>
  );
}
