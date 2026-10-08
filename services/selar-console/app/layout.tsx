// layout.tsx — Root layout for the SELAR console.
// Loads the typefaces via next/font (Figtree for UI, Fraunces for display
// headings, Source Serif 4 for reading text, JetBrains Mono for code/meta),
// imports the design system CSS, sets SEO/social metadata, and applies the
// colour scheme before first paint (lib/theme.tsx).
// Fonts are exposed as CSS variables and consumed by app/tokens.css, so a
// brand font swap only needs changes here and in the --selar-font-* tokens.
// Favicon, icon, apple-icon, opengraph-image and twitter-image are file conventions in app/;
// the web manifest is app/manifest.ts. Brand sources: brand/ at the repo root.

import type { Metadata, Viewport } from "next";
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

const SITE_URL = process.env.NEXT_PUBLIC_SITE_URL ?? "https://selar-console.vercel.app";
const DESCRIPTION =
  "Research prototype from UCSC. SELAR is a PDF reader that suggests a possible link to something you read earlier, shows both passages, and asks you to explain, compare and decide. AI suggests, sources show, you explain and decide.";

export const metadata: Metadata = {
  metadataBase: new URL(SITE_URL),
  title: {
    default: "SELAR: Semantic Linking for Active Retention",
    template: "%s · SELAR",
  },
  description: DESCRIPTION,
  applicationName: "SELAR",
  openGraph: {
    type: "website",
    siteName: "SELAR",
    title: "SELAR: link what you read, in your own words",
    description: DESCRIPTION,
    url: "/",
    locale: "en_GB",
  },
  twitter: {
    card: "summary_large_image",
    title: "SELAR: link what you read, in your own words",
    description: DESCRIPTION,
  },
};

export const viewport: Viewport = {
  themeColor: "#1F5135",
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
