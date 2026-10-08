// /styleguide — development-only gallery of the shared UI kit.
// Returns 404 in production builds so it never ships to users.

import { notFound } from "next/navigation";
import { Button, ButtonLink } from "@/components/ui/Button";
import { Badge, Card, PageHeader } from "@/components/ui/Card";
import { EmptyState } from "@/components/ui/EmptyState";
import { Icon, type IconName } from "@/components/ui/Icon";
import { Illustration, type IllustrationName } from "@/components/ui/Illustration";
import { Wordmark } from "@/components/ui/Wordmark";

const SWATCHES = [
  ["Deep green", "--selar-green-800"],
  ["Mid green", "--selar-green-500"],
  ["Ink", "--selar-ink"],
  ["Slate", "--selar-slate"],
  ["Light", "--selar-light"],
  ["Sky", "--selar-sky"],
  ["Rust", "--selar-rust"],
  ["Rust tint", "--selar-rust-tint"],
  ["Amber", "--selar-amber"],
] as const;

const ICONS: IconName[] = ["book", "doc", "link", "graph", "chat", "bulb", "scale", "shield", "users", "pen", "history", "check", "x", "info", "upload", "search", "settings", "logout", "menu", "external"];
const ILLOS: IllustrationName[] = ["reading", "connect", "explain", "compare", "decide", "library", "graph", "lost"];
const H2 = { font: "600 20px var(--font-display)" };

export default function StyleguidePage() {
  if (process.env.NODE_ENV === "production") notFound();
  return (
    <main style={{ maxWidth: 1080, margin: "0 auto", padding: "40px 24px 80px", fontFamily: "var(--font-sans)", color: "var(--ink)", background: "var(--bg)" }}>
      <Wordmark subtitle />
      <div style={{ height: 24 }} />
      <PageHeader eyebrow="Design system" title="SELAR UI kit" description="Tokens, buttons, cards, badges, icons, illustrations and empty states shared across the console." actions={<Button variant="primary">Primary action</Button>} />

      <h2 style={H2}>Palette</h2>
      <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fill,minmax(110px,1fr))", gap: 12, marginBottom: 32 }}>
        {SWATCHES.map(([name, v]) => (
          <div key={v}>
            <div style={{ height: 56, borderRadius: 12, background: `var(${v})`, border: "1px solid var(--rule)" }} />
            <div style={{ fontSize: 12, marginTop: 6, fontWeight: 600 }}>{name}</div>
            <code style={{ fontSize: 11, color: "var(--ink-3)" }}>{v}</code>
          </div>
        ))}
      </div>

      <h2 style={H2}>Buttons</h2>
      <div style={{ display: "flex", flexWrap: "wrap", gap: 10, marginBottom: 32 }}>
        <Button variant="primary">Start reading <Icon name="arrow_right" size={14} /></Button>
        <Button>Secondary</Button>
        <Button variant="warm">Warm</Button>
        <Button variant="ghost">Ghost</Button>
        <Button variant="primary" size="sm">Small</Button>
        <Button variant="primary" size="lg">Large</Button>
        <Button disabled>Disabled</Button>
        <ButtonLink href="/login">Link button</ButtonLink>
      </div>

      <h2 style={H2}>Cards and badges</h2>
      <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit,minmax(220px,1fr))", gap: 16, marginBottom: 32 }}>
        <Card><Badge tone="green" dot>Ready</Badge><p>Plain card</p></Card>
        <Card tint="green" interactive><Badge tone="amber" dot>Processing</Badge><p>Green tint, interactive</p></Card>
        <Card tint="sky"><Badge tone="sky" dot>Suggested</Badge><p>Sky tint</p></Card>
        <Card tint="warm"><Badge tone="warm" dot>Needs a look</Badge><p>Warm tint</p></Card>
      </div>

      <h2 style={H2}>Icons</h2>
      <div style={{ display: "flex", flexWrap: "wrap", gap: 16, marginBottom: 32, color: "var(--accent)" }}>
        {ICONS.map((n) => (
          <span key={n} title={n} style={{ display: "grid", placeItems: "center", width: 44, height: 44, borderRadius: 12, background: "var(--tint-green)" }}>
            <Icon name={n} size={20} />
          </span>
        ))}
      </div>

      <h2 style={H2}>Illustrations</h2>
      <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fill,minmax(200px,1fr))", gap: 16, marginBottom: 32 }}>
        {ILLOS.map((n) => (
          <Card key={n} style={{ display: "grid", justifyItems: "center", gap: 8 }}>
            <Illustration name={n} width={180} />
            <code style={{ fontSize: 12 }}>{n}</code>
          </Card>
        ))}
      </div>
      <Card style={{ marginBottom: 32 }}><Illustration name="hero" width="100%" /></Card>

      <h2 style={H2}>Empty state</h2>
      <EmptyState illustration="library" title="Your shelf is waiting" actions={<Button variant="primary">Add your first reading</Button>}>
        Upload a PDF or paste an article. SELAR will look for places where it meets something you read before.
      </EmptyState>

      <div style={{ marginTop: 32 }} className="ui-notice">
        <Icon name="info" size={18} />
        <span><strong>Research prototype.</strong> This is a notice block.</span>
      </div>
    </main>
  );
}
