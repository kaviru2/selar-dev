"""Build the SELAR visual brand guide (A4 landscape, multi-page PDF) via headless Chrome."""
import os, sys
sys.path.insert(0, os.path.dirname(__file__))
from build_marketing import render, esc, icon, lockup_svg, pose, OUT, RUST_INK
import logos, mascot, copy_text as C
from common import PAL

W, H = 1123, 794


def L(h):
    h = h.lstrip('#'); c = [int(h[i:i + 2], 16) / 255 for i in (0, 2, 4)]
    c = [x / 12.92 if x <= 0.03928 else ((x + 0.055) / 1.055) ** 2.4 for x in c]
    return .2126 * c[0] + .7152 * c[1] + .0722 * c[2]


def cr(a, b):
    la, lb = L(a), L(b)
    return (max(la, lb) + .05) / (min(la, lb) + .05)


def rgb(h):
    h = h.lstrip('#'); return ", ".join(str(int(h[i:i + 2], 16)) for i in (0, 2, 4))


COLORS = [
    ("Forest", PAL["forest"], "Primary. Linny, buttons, dark surfaces", "#FFFFFF"),
    ("Moss", PAL["moss"], "Illustration fills, wing, charts", "#FFFFFF"),
    ("Ink", PAL["ink"], "Text and outlines", "#FFFFFF"),
    ("Slate", PAL["slate"], "Secondary text", "#FFFFFF"),
    ("Rust", PAL["rust"], "The thread: links, focus", "#FFFFFF"),
    ("Rust ink", RUST_INK, "Small rust text on tints", "#FFFFFF"),
    ("Amber", PAL["amber"], "Beak, feet, badges (ink text)", PAL["ink"]),
    ("Sun", PAL["sun"], "Decorative sparkle only", PAL["ink"]),
    ("Mist", PAL["mist"], "App background", PAL["ink"]),
    ("Paper", PAL["paper"], "Reading surface, print", PAL["ink"]),
    ("Sky", PAL["sky"], "Info callouts, blobs", PAL["ink"]),
    ("Blush", PAL["blush"], "Prototype pill, gentle notes", PAL["ink"]),
]

PAIRS = [
    ("Ink", PAL["ink"], "Mist", PAL["mist"]), ("Slate", PAL["slate"], "White", "#FFFFFF"),
    ("Slate", PAL["slate"], "Mist", PAL["mist"]), ("White", "#FFFFFF", "Forest", PAL["forest"]),
    ("Paper", PAL["paper"], "Forest", PAL["forest"]), ("Rust", PAL["rust"], "White", "#FFFFFF"),
    ("Rust ink", RUST_INK, "Blush", PAL["blush"]), ("Ink", PAL["ink"], "Sky", PAL["sky"]),
    ("Ink", PAL["ink"], "Amber", PAL["amber"]), ("White", "#FFFFFF", "Moss", PAL["moss"]),
    ("Rust", PAL["rust"], "Blush", PAL["blush"]), ("Amber", PAL["amber"], "White", "#FFFFFF"),
]

CSS = f"""
@page {{ size:{W}px {H}px; margin:0; }}
html,body {{ width:{W}px !important; height:auto !important; overflow:visible !important; background:#fff; }}
.pg {{ width:{W}px; height:{H}px; padding:52px 60px 40px; position:relative; overflow:hidden; page-break-after:always; display:flex; flex-direction:column; background:var(--paper); }}
.pg:last-child {{ page-break-after:auto; }}
.hd {{ display:flex; justify-content:space-between; align-items:baseline; margin-bottom:22px; }}
.hd .kicker {{ color:var(--rust-ink); font-size:13px; }}
.hd .n {{ font-size:13px; color:var(--slate); }}
h1 {{ font-size:40px; line-height:1.08; font-weight:700; margin-bottom:10px; }}
h2 {{ font-size:20px; font-weight:700; margin:0 0 8px; }}
p, li {{ font-size:14.5px; line-height:1.5; color:#334155; }}
ul {{ padding-left:18px; }}
.foot {{ position:absolute; left:60px; right:60px; bottom:22px; display:flex; justify-content:space-between; font-size:11px; color:var(--slate); }}
.row {{ display:flex; gap:28px; }}
.card {{ background:#fff; border:1.5px solid #E2E8F0; border-radius:14px; padding:18px; }}
.dark {{ background:var(--forest); border-color:var(--forest); }}
.inkbg {{ background:var(--ink); border-color:var(--ink); }}
.cap {{ font-size:12px; color:var(--slate); margin-top:8px; }}
.sw {{ border-radius:12px; overflow:hidden; border:1.5px solid #E2E8F0; background:#fff; }}
.sw.paper .chip {{ box-shadow: inset 0 0 0 1px #CBD5E1; }}
.sw .chip {{ height:120px; padding:10px 12px; font-weight:700; font-size:14px; display:flex; align-items:flex-end; }}
.sw .meta {{ padding:8px 12px 10px; font-size:11.5px; line-height:1.45; color:#334155; }}
.sw .meta b {{ font-family:'JetBrains Mono',monospace; font-size:11.5px; color:var(--ink); }}
table {{ border-collapse:collapse; width:100%; }}
td, th {{ font-size:13px; padding:9px 8px; border-bottom:1px solid #E2E8F0; text-align:left; color:#334155; }}
th {{ color:var(--ink); }}
.ok {{ color:{PAL['forest']}; font-weight:700; }} .no {{ color:{RUST_INK}; font-weight:700; }}
.do, .dont {{ border-radius:12px; padding:12px 14px; font-size:14px; line-height:1.4; }}
.do {{ background:#E8F1EC; }} .dont {{ background:var(--blush); }}
.do b, .dont b {{ display:block; font-size:11px; letter-spacing:.08em; text-transform:uppercase; margin-bottom:4px; }}
.do b {{ color:var(--forest); }} .dont b {{ color:var(--rust-ink); }}
"""


def page(n, kicker, body):
    return (f'<section class="pg"><div class="hd"><span class="kicker">{esc(kicker)}</span><span class="n">SELAR brand guide · {n:02d}</span></div>'
            f'{body}<div class="foot"><span>{esc(C.TEAM)} · Research prototype</span><span>v1.0 · October 2026</span></div></section>')


def build():
    pages = []
    # 1 cover
    pages.append(f'''<section class="pg" style="background:var(--forest);color:#fff;justify-content:space-between">
<div>{lockup_svg("dark", "#FFFFFF", h=74)}</div>
<div style="display:flex;align-items:flex-end;justify-content:space-between">
<div style="max-width:620px"><div class="kicker" style="color:{PAL['sun']};font-size:14px">Brand guide · v1.0</div>
<h1 class="serif" style="font-size:62px;color:#fff;margin:12px 0 16px">AI suggests. Sources show. <em>You decide.</em></h1>
<p style="color:#E2E8F0;font-size:17px">How SELAR looks, sounds and behaves: logo, Linny the mascot, colour, type, voice and the claims we can and can't make.</p></div>
<div style="margin-right:-20px">{pose("hello", 330)}</div></div>
<div style="font-size:12px;color:#CBD5E1">{esc(C.TEAM)} · {esc(", ".join(C.MEMBERS))} · Supervisor: {esc(C.SUPERVISOR)} · Research prototype · v1.0, October 2026</div></section>''')

    # 2 brand idea
    steps = "".join(f'<div class="card" style="flex:1"><div class="serif" style="font-size:30px;color:var(--rust)">{i+1}</div><h2>{esc(t)}</h2><p>{esc(d)}</p></div>' for i, (t, d) in enumerate(C.STEPS))
    pages.append(page(2, "The idea", f'''<h1 class="serif">A careful, cheerful study companion</h1>
<div class="row" style="margin-top:6px;align-items:center"><div style="flex:1.3"><p style="font-size:16px">SELAR is a research prototype PDF reader. While you read, it suggests a possible link to something you read earlier, puts both passages side by side, and asks you to explain, compare and decide. {esc(C.GROUNDING)}</p>
<p style="font-size:16px;margin-top:10px">The brand should make that loop feel friendly and low-stakes, while staying honest: SELAR is early, the AI can be wrong, and the reader is the one doing the thinking.</p>
<h2 style="margin-top:18px">Personality</h2><p><b>Curious</b>, not clever-clever · <b>Warm</b>, not cutesy<br><b>Honest</b>, not hedging · <b>Tidy</b>, not corporate</p></div>
<div style="flex:1;display:flex;justify-content:center">{pose("linking", 300)}</div></div>
<div class="row" style="margin-top:auto;margin-bottom:40px;gap:14px">{steps}</div>'''))

    # 3 logo
    pages.append(page(3, "Logo", f'''<h1 class="serif">Linny holding a page</h1>
<p style="max-width:820px">The mark is Linny the linnet bringing back a page from earlier reading. The wordmark is lowercase <b>selar</b> in Fraunces (soft, weight 650) outlined to paths, so it never depends on installed fonts.</p>
<div class="row" style="margin-top:18px;flex-wrap:wrap;gap:18px">
<div class="card" style="flex:1.4;display:flex;align-items:center;justify-content:center;height:200px">{lockup_svg(h=92)}</div>
<div class="card dark" style="flex:1.4;display:flex;align-items:center;justify-content:center;height:200px">{lockup_svg("dark", "#FFFFFF", h=92)}</div>
<div class="card" style="flex:.6;display:flex;align-items:center;justify-content:center;height:200px">{icon(size=130)}</div></div>
<div class="row" style="margin-top:18px;gap:18px">
<div class="card" style="flex:1.6;display:flex;align-items:center;justify-content:center;height:200px">{lockup_svg(h=96, sub=C.FULL_NAME, sub_color=PAL["slate"])}</div>
<div class="card" style="flex:.7;display:flex;align-items:center;justify-content:center;height:200px;padding:14px">{lockup_svg(h=150, stacked=True)}</div>
<div class="card" style="flex:.45;display:flex;align-items:center;justify-content:center;height:200px">{icon(v="mono-dark", size=100)}</div>
<div class="card inkbg" style="flex:.45;display:flex;align-items:center;justify-content:center;height:200px">{icon(v="mono-light", size=100)}</div></div>
<p class="cap">Horizontal lockup (default) · on dark · icon · lockup with descriptor · stacked · mono ink · mono white. Files: <b>logo/svg</b> and <b>logo/png</b>.</p>'''))

    # 4 usage
    s = lockup_svg(h=70)
    rules = [("Clear space", "Keep empty space around the logo at least the height of the page in Linny's beak (about a quarter of the icon height) on every side."),
             ("Minimum size", "Icon 24 px on screen / 8 mm in print. Below 24 px use the simplified favicon drawing. Horizontal lockup at least 120 px / 30 mm wide."),
             ("Backgrounds", "Colour logo on mist, paper or white. On-dark version on forest or ink. Mono versions on photos and one-colour print."),
             ("Alternatives", "Directions B (Page Bridge) and C (Knot S) are kept in exploration/. Switch as a whole system, never mix marks.")]
    dont = ["Stretch, squash or rotate the logo", "Recolour outside the palette or add gradients", "Put the colour mark on busy photos",
            "Retype the wordmark in another font", "Add shadows, outlines or glows", "Separate Linny from the page in the logo"]
    pages.append(page(4, "Logo usage", f'''<h1 class="serif">Give it room</h1>
<div class="row" style="margin-top:8px"><div style="flex:1">
<div class="card" style="display:flex;justify-content:center;align-items:center;height:210px;background:repeating-linear-gradient(45deg,#fff,#fff 8px,{PAL['blush']} 8px,{PAL['blush']} 10px)">
<div style="padding:22px;background:#fff;outline:2px dashed {PAL['rust']}">{s}</div></div>
<p class="cap">Dashed line: minimum clear space. Hatched area: keep free of text and graphics.</p>
<div class="card" style="display:flex;gap:26px;align-items:flex-end;margin-top:18px;flex-wrap:wrap">{icon(size=64)}{icon(size=40)}{icon(size=24)}
<img src="file://{OUT}/brand/favicon/favicon-32x32.png" width="32" height="32"><img src="file://{OUT}/brand/favicon/favicon-16x16.png" width="16" height="16">
</div><p class="cap">Left to right: 64, 40 and 24 px mark, then the simplified 32 and 16 px favicon drawings (actual size).</p></div>
<div style="flex:1">{"".join(f'<h2>{esc(t)}</h2><p style="margin-bottom:12px">{esc(d)}</p>' for t, d in rules)}
<div class="dont" style="margin-top:6px"><b>Don't</b>{"<br>".join("✕ " + esc(d) for d in dont)}</div></div></div>'''))

    # 5 colour
    sw = "".join(f'''<div class="sw{' paper' if n in ('Paper','Mist') else ''}"><div class="chip" style="background:{h};color:{t}">{esc(n)}</div><div class="meta"><b>{h}</b><br>RGB {rgb(h)}<br>{esc(u)}</div></div>''' for n, h, u, t in COLORS)
    pages.append(page(5, "Colour", f'''<h1 class="serif">Forest, ink and a rust-red thread</h1>
<p>Carried over from the illustrated presentation deck. Paper and Sun add warmth; Rust ink keeps small text accessible on tints.</p>
<div style="display:grid;grid-template-columns:repeat(6,1fr);gap:12px;margin-top:16px">{sw}</div>
<div style="margin-top:auto;margin-bottom:34px"><div class="cap" style="margin-bottom:6px">Rough proportions on a typical surface</div><div style="display:flex;height:34px;border-radius:10px;overflow:hidden;border:1.5px solid #E2E8F0">
<div style="flex:42;background:{PAL['paper']}"></div><div style="flex:18;background:{PAL['mist']}"></div><div style="flex:16;background:{PAL['forest']}"></div><div style="flex:10;background:{PAL['ink']}"></div><div style="flex:6;background:{PAL['sky']}"></div><div style="flex:4;background:{PAL['rust']}"></div><div style="flex:2;background:{PAL['amber']}"></div><div style="flex:2;background:{PAL['sun']}"></div></div></div>'''))

    # 6 contrast
    rows = ""
    for fa, a, ba, b in PAIRS:
        r = cr(a, b)
        verdict = "AA all text" if r >= 4.5 else ("AA large text only" if r >= 3 else "Decorative only")
        cls = "ok" if r >= 4.5 else "no"
        rows += f'<tr><td><span style="background:{b};color:{a};padding:4px 10px;border-radius:6px;font-weight:700;border:1px solid #E2E8F0">Aa {esc(fa)} on {esc(ba)}</span></td><td style="font-family:JetBrains Mono,monospace">{a} / {b}</td><td><b>{r:.2f}:1</b></td><td class="{cls}">{verdict}</td></tr>'
    pages.append(page(6, "Accessibility", f'''<h1 class="serif">Contrast pairs (WCAG 2.2)</h1>
<div class="row"><div style="flex:1.6"><table><tr><th>Sample</th><th>Text / background</th><th>Ratio</th><th>Use</th></tr>{rows}</table></div>
<div style="flex:1"><h2>Rules of thumb</h2><ul><li>Body text: ink or slate on mist, paper or white.</li><li>Small rust text on blush uses <b>rust ink</b> ({RUST_INK}).</li><li>White on moss and amber on white are for large text or graphics only.</li><li>Sun and amber are never text colours on light backgrounds.</li><li>Don't rely on colour alone: kept / changed / rejected links also get an icon and a word.</li></ul>
<p class="cap" style="margin-top:14px">Ratios computed from relative luminance; DESIGN.md is linted with @google/design.md (0 errors, 0 warnings).</p></div></div>'''))

    # 7 type
    pages.append(page(7, "Typography", f'''<h1 class="serif">Fraunces for voice, Figtree for reading</h1>
<div class="row" style="margin-top:8px">
<div class="card" style="flex:1.2"><div class="kicker" style="font-size:12px;color:var(--rust-ink)">Display · Fraunces, SOFT 100, 650–700</div>
<div class="serif" style="font-size:58px;line-height:1.04;font-weight:700;margin:10px 0 14px">Every new page<br>has an old friend.</div>
<div class="serif" style="font-size:26px;font-weight:650;margin-bottom:6px">Aa Bb Cc 0123456789 “link” &amp; ?!</div></div>
<div class="card" style="flex:1"><div class="kicker" style="font-size:12px;color:var(--rust-ink)">UI &amp; body · Figtree 400 / 700</div>
<p style="font-size:19px;color:var(--ink);margin:10px 0">Spotted a possible link to your notes on working memory. Both passages are below. How do they connect?</p>
<p style="font-size:15px">Body 16 px / 1.55 · small 14 px · labels 12 px bold, tracked 0.08em, uppercase.</p>
<div style="font-family:'JetBrains Mono',monospace;font-size:13px;margin-top:14px;color:#334155">JetBrains Mono · p. 14 · doc_7f2a · sim 0.82</div><div style="margin-top:16px;display:flex;gap:10px"><span class="pill" style="background:var(--blush);color:var(--rust-ink);padding:6px 12px;font-size:12px;letter-spacing:.08em;text-transform:uppercase"><span class="dot"></span>Research prototype</span><span class="pill" style="background:var(--forest);color:#fff;padding:8px 18px;font-size:15px">Keep link</span></div></div></div>
<table style="margin-top:auto;margin-bottom:40px"><tr><th>Font</th><th>Role</th><th>Licence</th><th>Source</th></tr>
<tr><td>Fraunces (variable: opsz, wght, SOFT, WONK)</td><td>Headlines, wordmark, numbers</td><td>SIL OFL 1.1</td><td>github.com/google/fonts/tree/main/ofl/fraunces</td></tr>
<tr><td>Figtree (variable: wght)</td><td>Body, UI, captions</td><td>SIL OFL 1.1</td><td>github.com/google/fonts/tree/main/ofl/figtree</td></tr>
<tr><td>JetBrains Mono</td><td>Page refs, IDs, debug</td><td>SIL OFL 1.1</td><td>github.com/google/fonts/tree/main/ofl/jetbrainsmono</td></tr></table>'''))

    # 8 mascot
    tiles = "".join(f'<div style="text-align:center">{pose(p, 140)}<div class="cap" style="margin-top:2px;min-height:34px"><b style="color:var(--ink)">{p}</b><br>{esc(mascot.POSE_LABEL[p])}</div></div>' for p in mascot.POSES)
    pages.append(page(8, "Mascot", f'''<h1 class="serif">Meet Linny the linnet</h1>
<p style="max-width:900px">Linnets are small, chatty finches. Linny carries pages between what you read now and what you read before, then waits for your call. Linny suggests and asks; Linny never grades, scolds or claims to know best.</p>
<div style="display:grid;grid-template-columns:repeat(7,1fr);gap:8px;margin-top:auto">{tiles}</div>
<div class="row" style="margin-top:auto;margin-bottom:40px;gap:14px">
<div class="do" style="flex:1"><b>Do</b>Use one Linny per view, at the moment it helps: onboarding, loading, a new suggestion, an empty library, a kept link.</div>
<div class="do" style="flex:1"><b>Do</b>Use <i>questioning</i> for weak or rejected links. Rejecting a stretch is good thinking, so keep it gentle.</div>
<div class="dont" style="flex:1"><b>Don't</b>Put Linny over text, on warnings about data or errors, or in academic figures and the report.</div></div>'''))

    # 9 illustration + voice
    vd = "".join(f'<div class="row" style="gap:10px;margin-bottom:8px"><div class="do" style="flex:1"><b>Say</b>{esc(d)}</div><div class="dont" style="flex:1"><b>Not</b>{esc(n)}</div></div>' for d, n in C.VOICE_DO_DONT)
    pages.append(page(9, "Voice & illustration", f'''<div class="row"><div style="flex:1.35"><h1 class="serif">Talk like a good study partner</h1>
<p style="margin-bottom:12px">Plain, kind and specific. Ask rather than tell. Name the source. Admit uncertainty without drowning in it. One light joke is fine; three is a lot.</p>{vd}</div>
<div style="flex:1"><h2 style="margin-top:6px">Illustration style</h2><ul><li>Flat shapes, 3.5 px ink outline at mark scale, round caps and joins.</li><li>Pages are rounded rectangles with a folded corner and two text lines.</li><li>The thread is rust; dashed when a link is uncertain.</li><li>Soft sky or blush circles behind characters. No gradients, no photos of people, no stock clip art.</li></ul>
<div class="card" style="margin-top:14px;display:flex;justify-content:center;gap:10px">{pose("reading", 140)}{pose("questioning", 140)}</div></div></div>'''))

    # 10 claims
    pages.append(page(10, "Claims & integrity", f'''<h1 class="serif">What we can and can't say</h1>
<div class="row" style="margin-top:8px"><div style="flex:1.2"><ul>{"".join(f"<li style='margin-bottom:8px;font-size:15.5px'>{esc(r)}</li>" for r in C.CLAIM_RULES)}</ul></div>
<div style="flex:1"><div class="card"><h2>Survey numbers, exactly</h2><p class="cap" style="margin:0 0 8px">{esc(C.SURVEY_NOTE)}</p>
{"".join(f'<div style="display:flex;gap:14px;align-items:baseline;padding:8px 0;border-bottom:1.5px dashed #CBD5E1"><b class="serif" style="font-size:24px;color:var(--rust-ink);flex:0 0 96px">{esc(n)}</b><span style="font-size:14px">{esc(d)}</span></div>' for n, d in C.SURVEY)}</div>
<div class="card" style="margin-top:14px;background:var(--blush);border-color:var(--blush)"><h2>Recommended tagline</h2><div class="serif" style="font-size:26px;font-weight:700">{esc(C.TAGLINES[0][0])}</div></div></div></div>'''))

    # 11 assets index
    pages.append(page(11, "What's in the kit", f'''<h1 class="serif">Files and where they go</h1>
<table style="margin-top:6px">
<tr><th>Folder</th><th>Contents</th></tr>
<tr><td>logo/svg, logo/png</td><td>Lockup (horizontal, descriptor, stacked), icon and wordmark in colour, on-dark, mono ink and mono white. PNG at 512/1024/2048 px.</td></tr>
<tr><td>favicon/</td><td>favicon.ico (16/32/48), favicon-16/32/48, apple-touch-icon 180, icon-192/512, maskable 512, favicon.svg, site.webmanifest.</td></tr>
<tr><td>mascot/</td><td>Linny in 7 poses, SVG and PNG.</td></tr>
<tr><td>exploration/</td><td>All three directions (A Linny, B Page Bridge, C Knot S) with icons and lockups, plus the comparison board.</td></tr>
<tr><td>social/</td><td>OG/Twitter card 1200×630, 3-slide carousel 1080×1350, 2 squares 1080×1080, captions (drafts, not posted).</td></tr>
<tr><td>print/</td><td>A4 "try the prototype" flyer, PDF and PNG.</td></tr>
<tr><td>DESIGN.md, tokens.dtcg.json, theme.css</td><td>Machine-readable tokens (Google DESIGN.md spec), DTCG and Tailwind v4 exports.</td></tr>
<tr><td>Console code</td><td><code>components/brand/Logo.tsx</code> (Logo, LogoMark, Wordmark) and <code>Linny.tsx</code> (pose prop) in services/selar-console.</td></tr>
</table>
<div style="display:flex;gap:14px;margin-top:auto;margin-bottom:14px;align-items:flex-end">
<img src="file://{OUT}/og-image.png" style="height:120px;border-radius:8px;border:1px solid #E2E8F0">
{"".join(f'<img src="file://{OUT}/social/carousel-0{i}.png" style="height:150px;border-radius:8px;border:1px solid #E2E8F0">' for i in (1,2,3))}
<img src="file://{OUT}/social/square-01-principle.png" style="height:120px;border-radius:8px">
<img src="file://{OUT}/selar-flyer-a4.png" style="height:170px;border-radius:4px;border:1px solid #E2E8F0"></div>
<p class="cap" style="margin-bottom:30px">All artwork is original, drawn as code (SVG) by the team's tooling. Fonts are SIL OFL. Sources and build scripts: brand/tools in the selar-dev repo.</p>'''))

    body = "".join(pages)
    return render("SELAR-brand-guide", body, W, H, CSS, pdf=True)


if __name__ == "__main__":
    print(build())
