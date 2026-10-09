import Link from "next/link";
import { ProcessingDisclosure } from "@/components/ProcessingDisclosure";
import { PageHeader } from "@/components/ui/Card";

/** Optional help, not a registration step or a fabricated completion checklist. */
export default function OnboardingPage() {
  return <div className="app-page"><div className="app-page-inner">
    <PageHeader eyebrow="Optional help" title="Start with one reading"
      description="No setup wizard. Your library shows the next step for the material you add."
      actions={<Link href="/library#start-here" className="ui-btn ui-btn--primary">Open library guide</Link>} />
    <section className="ui-card" style={{ padding: 24, display: "grid", gap: 16 }} aria-label="Getting started">
      <h2>Add material you are allowed to use</h2>
      <p>In Library, choose Add content for the supported file formats, a web page, or pasted text. Google Drive import is alongside it when configured; choose one PDF at a time.</p>
      <ProcessingDisclosure />
      <h2>Wait for Ready, then open the reading</h2>
      <p>Queued and processing readings update in Library. You can leave and return later. If processing fails, use Retry or add another reading. Ready means processing finished, not that you have learned the material.</p>
      <h2>Read at your own pace</h2>
      <p>With more readings, SELAR may suggest connections. Check the source passages before relying on them. You do not need a connection to start reading.</p>
      <p><Link href="/settings">Reading preferences</Link> are optional. You can dismiss the library guide and reopen it from Start here whenever you need help.</p>
      <p>Do not upload sensitive or confidential material.</p>
    </section>
  </div></div>;
}
