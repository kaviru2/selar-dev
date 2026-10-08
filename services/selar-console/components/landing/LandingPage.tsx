// LandingPage.tsx — public marketing page for SELAR.
// Rendered at "/" for signed-out visitors and at "/about" for everyone.
//
// Copy rules (research integrity, see issue #77):
// - SELAR is a research prototype. Never claim it improves memory, retention
//   or grades: the controlled 7-day study is planned and pending ethics.
// - Only cite the formative survey figures and published learning research.
// - No testimonials, logos, user counts or pricing.

import Link from "next/link";
import { ButtonLink } from "@/components/ui/Button";
import { Icon, type IconName } from "@/components/ui/Icon";
import { Illustration, type IllustrationName } from "@/components/ui/Illustration";
import { Wordmark } from "@/components/ui/Wordmark";

const STEPS: { n: string; title: string; body: string; illo: IllustrationName; tint: string }[] = [
  {
    n: "1",
    title: "Notice",
    body: "While you read a PDF, SELAR may spot a passage that seems to echo something from an earlier reading. It flags it as a possible link. It's only a guess at this stage.",
    illo: "connect",
    tint: "green",
  },
  {
    n: "2",
    title: "Explain",
    body: "Before you see the AI's reason, you're asked to put the connection in your own words. If you can't see one, saying so is a perfectly good answer.",
    illo: "explain",
    tint: "sky",
  },
  {
    n: "3",
    title: "Compare",
    body: "Both source passages appear side by side, quoted exactly, with page references. You check the suggestion against what the texts actually say.",
    illo: "compare",
    tint: "warm",
  },
  {
    n: "4",
    title: "Decide",
    body: "Keep the link, change it, or reject it. Nothing is saved to your reading history unless you choose to keep it.",
    illo: "decide",
    tint: "amber",
  },
];

const RESEARCH: { icon: IconName; title: string; body: string; cite: string; href: string }[] = [
  {
    icon: "bulb",
    title: "Depth of processing",
    body: "Information that is processed for meaning tends to be remembered better than information processed only at the surface.",
    cite: "Craik & Lockhart (1972). Journal of Verbal Learning and Verbal Behavior, 11(6), 671–684.",
    href: "https://doi.org/10.1016/S0022-5371(72)80001-X",
  },
  {
    icon: "pen",
    title: "Self-explanation",
    body: "Learners who explain material to themselves while studying tend to build better understanding than those who only reread it.",
    cite: "Chi, de Leeuw, Chiu & LaVancher (1994). Cognitive Science, 18(3), 439–477.",
    href: "https://doi.org/10.1207/s15516709cog1803_3",
  },
  {
    icon: "scale",
    title: "Comparison",
    body: "Comparing two cases side by side helps people notice the structure they share, and makes that idea easier to apply later.",
    cite: "Gentner, Loewenstein & Thompson (2003). Journal of Educational Psychology, 95(2), 393–408.",
    href: "https://doi.org/10.1037/0022-0663.95.2.393",
  },
  {
    icon: "history",
    title: "Retrieval practice",
    body: "Bringing earlier material back to mind is a well-studied way to make it more durable than restudying it.",
    cite: "Roediger & Karpicke (2006). Psychological Science, 17(3), 249–255.",
    href: "https://doi.org/10.1111/j.1467-9280.2006.01693.x",
  },
];

const SURVEY = [
  { n: 31, label: "would like a tool that links new reading to earlier reading" },
  { n: 28, label: "are concerned about how accurate AI suggestions are" },
  { n: 24, label: "worry that AI help could mean less active thinking" },
];

const NOT_LIST: { title: string; body: string }[] = [
  { title: "Not a summariser", body: "It never condenses your reading into AI notes. You read the source." },
  { title: "Not an answer machine", body: "It asks you questions. Your explanation comes before its reason." },
  { title: "Not the final word", body: "Every suggestion can be wrong. You can reject it, and that's a useful outcome too." },
  { title: "Not a proven study aid (yet)", body: "We haven't tested whether it helps people remember. That study is still to come." },
];

export function LandingPage() {
  return (
    <div className="lp">
      <a className="ui-skip" href="#main">Skip to content</a>

      <header className="lp-nav">
        <Wordmark />
        <span className="ui-badge ui-badge--amber lp-nav-badge">Research prototype</span>
        <nav aria-label="Page sections" className="lp-nav-links">
          <a href="#how">How it works</a>
          <a href="#research">The research</a>
          <a href="#not">What it isn&apos;t</a>
          <a href="#team">Team</a>
        </nav>
        <div className="lp-nav-cta">
          <ButtonLink href="/login" variant="ghost" size="sm">Sign in</ButtonLink>
          <ButtonLink href="/register" variant="primary" size="sm">Create account</ButtonLink>
        </div>
      </header>

      <main id="main">
        {/* ——— Hero ——— */}
        <section className="lp-hero" aria-labelledby="hero-title">
          <div className="lp-hero-copy">
            <span className="ui-eyebrow">Semantic Linking for Active Retention</span>
            <h1 id="hero-title">
              Today&apos;s reading, meet <span className="lp-underline">last week&apos;s</span>.
            </h1>
            <p className="lp-lede">
              SELAR is a PDF reader that sometimes says: <em>&ldquo;this might connect to something you read before.&rdquo;</em> It shows you both passages, asks how you think they relate, and lets you decide whether the link is worth keeping.
            </p>
            <p className="lp-principle">
              <span><Icon name="sparkles" size={15} /> AI suggests</span>
              <span><Icon name="doc" size={15} /> Sources show</span>
              <span><Icon name="pen" size={15} /> You explain and decide</span>
            </p>
            <div className="lp-hero-actions">
              <ButtonLink href="/register" variant="primary" size="lg">
                Try the prototype <Icon name="arrow_right" size={15} />
              </ButtonLink>
              <ButtonLink href="#how" size="lg">See how it works</ButtonLink>
            </div>
            <p className="lp-small">Free university prototype. Use readings you&apos;re happy to share with a research tool.</p>
          </div>
          <div className="lp-hero-art">
            <Illustration name="hero" width="100%" title="Two reading cards, an earlier reading and the one you are reading now, joined by a dotted line with the question: how are they linked?" />
          </div>
        </section>

        {/* ——— How it works ——— */}
        <section id="how" className="lp-section" aria-labelledby="how-title">
          <div className="lp-section-head">
            <span className="ui-eyebrow">How it works</span>
            <h2 id="how-title">Four small steps, and you make the call</h2>
            <p>Each suggestion walks you through the same short loop. It usually takes a minute or two. You can skip any suggestion you&apos;re not in the mood for.</p>
          </div>
          <ol className="lp-steps">
            {STEPS.map((s) => (
              <li key={s.n} className={`lp-step lp-step--${s.tint}`}>
                <div className="lp-step-art"><Illustration name={s.illo} width={200} /></div>
                <div className="lp-step-copy">
                  <span className="lp-step-n" aria-hidden="true">{s.n}</span>
                  <h3><span className="ui-visually-hidden">Step {s.n}: </span>{s.title}</h3>
                  <p>{s.body}</p>
                </div>
              </li>
            ))}
          </ol>
        </section>

        {/* ——— Research ——— */}
        <section id="research" className="lp-section lp-section--tint" aria-labelledby="research-title">
          <div className="lp-section-head">
            <span className="ui-eyebrow">The research behind it</span>
            <h2 id="research-title">Built on well-studied ideas about learning</h2>
            <p>SELAR&apos;s design draws on four findings from learning research. These are reasons for the design. They aren&apos;t evidence that SELAR itself works; testing that is what the study is for.</p>
          </div>
          <div className="lp-research">
            {RESEARCH.map((r) => (
              <article key={r.title} className="lp-research-item">
                <span className="lp-research-icon"><Icon name={r.icon} size={20} /></span>
                <div>
                  <h3>{r.title}</h3>
                  <p>{r.body}</p>
                  <p className="lp-cite">
                    <a href={r.href} rel="noopener noreferrer" target="_blank">
                      {r.cite.slice(0, r.cite.lastIndexOf(" ") + 1)}
                      <span className="nw">{r.cite.slice(r.cite.lastIndexOf(" ") + 1)} <Icon name="external" size={11} /></span>
                      <span className="ui-visually-hidden"> (opens in a new tab)</span>
                    </a>
                  </p>
                </div>
              </article>
            ))}
          </div>

          <div className="lp-survey">
            <div className="lp-survey-head">
              <h3>What students told us first</h3>
              <p>Before building anything, we ran a formative survey (38 eligible respondents). Of the 36 who answered these questions:</p>
            </div>
            <ul className="lp-survey-list">
              {SURVEY.map((s) => (
                <li key={s.label}>
                  <span className="lp-survey-n">{s.n}<span> of 36</span></span>
                  <span className="lp-survey-bar" aria-hidden="true"><span style={{ width: `${(s.n / 36) * 100}%` }} /></span>
                  <span className="lp-survey-label">{s.label}</span>
                </li>
              ))}
            </ul>
            <p className="lp-small">Those concerns shaped the design. Every suggestion quotes both sources so you can check it, and you explain the link before the AI gives its reason.</p>
          </div>
        </section>

        {/* ——— What it is not ——— */}
        <section id="not" className="lp-section" aria-labelledby="not-title">
          <div className="lp-not">
            <div className="lp-not-art"><Illustration name="reading" width="100%" /></div>
            <div>
              <span className="ui-eyebrow">What it isn&apos;t</span>
              <h2 id="not-title">The AI doesn&apos;t do the thinking for you</h2>
              <ul className="lp-not-list">
                {NOT_LIST.map((n) => (
                  <li key={n.title}>
                    <span className="lp-not-x" aria-hidden="true"><Icon name="x" size={13} /></span>
                    <div>
                      <strong>{n.title}.</strong>{" "}{n.body}
                    </div>
                  </li>
                ))}
              </ul>
            </div>
          </div>
        </section>

        {/* ——— Prototype + team ——— */}
        <section id="team" className="lp-section" aria-labelledby="team-title">
          <div className="ui-notice lp-proto" role="note">
            <Icon name="info" size={20} />
            <div>
              <strong>This is a research prototype.</strong>{" "}SELAR is a final-year research project and is still changing. We haven&apos;t yet measured whether it affects what people remember. A controlled 7-day retention study is planned and will only start after ethics approval. Features may change or break, and AI suggestions can be wrong.
            </div>
          </div>

          <div className="lp-team">
            <div>
              <span className="ui-eyebrow">Who&apos;s behind it</span>
              <h2 id="team-title">Made by students, for students</h2>
              <p>SELAR is being built for IS4101 (Group 04) at the University of Colombo School of Computing.</p>
            </div>
            <dl className="lp-team-list">
              <div>
                <dt>Team</dt>
                <dd>G.P.G.S. Ganegoda</dd>
                <dd>H.K.S.R. Hapuarachchi</dd>
                <dd>K.A.D.A.A. Kuruppu Arachchi</dd>
              </div>
              <div>
                <dt>Supervisor</dt>
                <dd>Dr. Thushani A. Weerasinghe</dd>
              </div>
            </dl>
          </div>
        </section>

        {/* ——— Final CTA ——— */}
        <section className="lp-cta" aria-labelledby="cta-title">
          <Illustration name="library" width={170} />
          <div>
            <h2 id="cta-title">Bring two readings.<br /> See what you make of them.</h2>
            <p>Create an account, upload a couple of PDFs from the same course, and try the notice, explain, compare and decide loop yourself.</p>
          </div>
          <div className="lp-cta-actions">
            <ButtonLink href="/register" variant="primary" size="lg">Create account</ButtonLink>
            <ButtonLink href="/login" size="lg">I already have one</ButtonLink>
          </div>
        </section>
      </main>

      <footer className="lp-foot">
        <Wordmark size={22} />
        <span>Semantic Linking for Active Retention · a UCSC research prototype</span>
        <span className="lp-foot-links">
          <Link href="/login">Sign in</Link>
          <Link href="/register">Create account</Link>
        </span>
      </footer>
    </div>
  );
}
