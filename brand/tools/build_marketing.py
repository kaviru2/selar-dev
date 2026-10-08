"""Render SELAR marketing assets (comparison board, OG cards, flyer, carousel, squares) via headless Chrome."""
import os, sys, subprocess, tempfile, shutil, html
sys.path.insert(0, os.path.dirname(__file__))
sys.path.insert(0, os.path.join(os.path.dirname(__file__), "..", "pylib"))
import logos, mascot, common, copy_text as C
from common import PAL, ROOT, CHROME, font_face_css

OUT = os.path.join(ROOT, "out")
BUILD = os.path.join(ROOT, "build")
os.makedirs(BUILD, exist_ok=True)
INK = PAL["ink"]
RUST_INK = "#9A4322"  # rust darkened for small text on blush/white (AA)


def esc(t):
    return html.escape(t, quote=False)


def icon(fn=logos.linny_icon, v="color", size=100):
    return logos.wrap(fn(v), 100, 100, "SELAR").replace('width="100" height="100"', f'width="{size}" height="{size}"')


def lockup_svg(v="color", word=INK, h=60, sub=None, sub_color=None, fn=logos.linny_icon, stacked=False):
    inner, w, hh = logos.lockup(fn, v, word, sub=sub, sub_color=sub_color, stacked=stacked)
    svg = logos.wrap(inner, w, hh, "SELAR")
    return svg.replace(f'width="{w:.0f}" height="{hh:.0f}"', f'height="{h}" width="{w*h/hh:.0f}"')


def pose(name, size=240):
    return mascot.svg(name, size=size)


def qr_svg(url, px=150, dark=INK):
    import segno, io
    q = segno.make(url, error="m")
    buf = io.BytesIO()
    q.save(buf, kind="svg", scale=1, border=2, dark=dark, light="#FFFFFF", xmldecl=False, svgns=True, nl=False)
    s = buf.getvalue().decode()
    import re
    m = re.search(r'width="(\d+)" height="(\d+)"', s)
    n = int(m.group(1))
    s = s.replace(m.group(0), f'width="{px}" height="{px}" viewBox="0 0 {n} {n}"', 1)
    return s.replace('<svg ', '<svg shape-rendering="crispEdges" role="img" aria-label="QR code linking to the SELAR prototype" ', 1)


BASE_CSS = font_face_css() + f"""
:root {{ --forest:{PAL['forest']}; --moss:{PAL['moss']}; --ink:{INK}; --slate:{PAL['slate']}; --mist:{PAL['mist']};
  --sky:{PAL['sky']}; --rust:{PAL['rust']}; --rust-ink:{RUST_INK}; --blush:{PAL['blush']}; --amber:{PAL['amber']}; --sun:{PAL['sun']}; --paper:{PAL['paper']}; }}
* {{ box-sizing:border-box; margin:0; padding:0; }}
html,body {{ font-family:'Figtree',sans-serif; color:var(--ink); -webkit-font-smoothing:antialiased; }}
.serif {{ font-family:'Fraunces',serif; font-variation-settings:'SOFT' 100,'WONK' 0,'opsz' 72; }}
.kicker {{ font-family:'Figtree'; font-weight:700; letter-spacing:.08em; text-transform:uppercase; }}
.pill {{ display:inline-flex; align-items:center; gap:.5em; border-radius:999px; font-weight:700; }}
.dot {{ width:.6em; height:.6em; border-radius:50%; background:var(--rust); display:inline-block; }}
"""


def render(name, body, w, h, css="", pdf=False, scale=1, outdir=None):
    outdir = outdir or OUT
    os.makedirs(outdir, exist_ok=True)
    page_css = f"@page {{ size:{w}px {h}px; margin:0; }}" if not pdf else ""
    doc = f"<!doctype html><html><head><meta charset='utf-8'><style>{BASE_CSS}{page_css} html,body{{width:{w}px;height:{h}px;overflow:hidden;}} {css}</style></head><body>{body}</body></html>"
    hp = os.path.join(BUILD, f"{name}.html")
    open(hp, "w").write(doc)
    prof = tempfile.mkdtemp(prefix="chr-")
    target = os.path.join(outdir, f"{name}.pdf" if pdf else f"{name}.png")
    args = [CHROME, "--headless=new", "--disable-gpu", "--hide-scrollbars", f"--user-data-dir={prof}", "--no-first-run",
            "--allow-file-access-from-files", f"--force-device-scale-factor={scale}", "--virtual-time-budget=6000",
            "--default-background-color=00000000"]
    args += [f"--print-to-pdf={target}", "--no-pdf-header-footer"] if pdf else [f"--window-size={w},{h+200}", f"--screenshot={target}"]
    args.append("file://" + hp)
    if os.path.exists(target):
        os.remove(target)
    args[1:1] = ["--disable-extensions", "--disable-component-update", "--disable-background-networking",
                 "--no-default-browser-check", "--disable-sync", "--mute-audio"]
    import time
    p = subprocess.Popen(args, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    t0 = time.time(); last = -1
    while time.time() - t0 < 120:
        if p.poll() is not None:
            break
        if os.path.exists(target):
            sz = os.path.getsize(target)
            if sz > 0 and sz == last:
                break
            last = sz
        time.sleep(1.0)
    if p.poll() is None:
        p.kill(); p.wait()
    shutil.rmtree(prof, ignore_errors=True)
    if not os.path.exists(target):
        raise RuntimeError("render failed: " + target)
    if not pdf:
        from PIL import Image
        im = Image.open(target)
        im.crop((0, 0, int(w * scale), int(h * scale))).save(target)
    return target


# ------------------------------------------------------------------ comparison board
def comparison():
    dirs = [
        ("A", "Linny the linnet", logos.linny_icon, True,
         "A round, friendly bird carrying a page in its beak: bringing back something you read earlier.",
         ["Most personality; also our mascot and empty-state guide", "Easy to animate into poses (reading, linking, thinking)", "Holds up at 32 px with a simplified favicon cut"],
         ["Needs a dedicated 16 px drawing", "Mascot must stay modest in academic settings"]),
        ("B", "Bridge between pages", logos.bridge_icon, False,
         "Two pages side by side. A highlighted line on each is joined by one rust thread across the gutter.",
         ["Literal and instantly explainable to a panel", "Very sober; fits reports and slides", "Mirrors the side-by-side passages in the reader"],
         ["Little warmth; reads as a generic notes app", "Thread gets thin at favicon sizes"]),
        ("C", "Knot monogram", logos.knot_icon, False,
         "An S drawn as a single thread, looped once, with a node at each end.",
         ["Ownable letterform; strong app-icon silhouette", "Works in one colour without changes", "Easy to stamp, stitch or emboss"],
         ["Abstract: the link idea needs explaining", "Loop can read as script or a ‘9’"]),
    ]
    cols = []
    for key, name, fn, rec, desc, pros, cons in dirs:
        badge = '<div class="rec pill"><span class="dot" style="background:var(--sun)"></span>Recommended</div>' if rec else '<div class="alt">Alternative</div>'
        extra = ""
        fav = f'<div class="tile fav"><div>{icon(fn, "color", 32)}</div><div>{icon(fn, "color", 16)}</div></div>'
        if fn is logos.linny_icon:
            s32 = logos.wrap(logos.linny_small(), 100, 100, "favicon").replace('width="100" height="100"', 'width="32" height="32"')
            s16 = logos.wrap(logos.linny_tiny(), 100, 100, "favicon").replace('width="100" height="100"', 'width="16" height="16"')
            fav = '<div class="tile fav"><div>' + s32 + '</div><div>' + s16 + '</div></div>'
        if True:
            extra = f'<div class="poses mono"><div class="tile dark">{icon(fn, "mono-light", 76)}</div><div class="tile">{icon(fn, "mono-dark", 76)}</div>{fav}</div>'
            if fn is logos.linny_icon:
                extra += '<div class="poses row3">' + "".join(f'<div>{pose(p, 84)}</div>' for p in ["reading", "linking", "celebrating", "questioning", "empty"]) + '</div>'
            else:
                extra += f'<div class="poses row3 darkstrip">{lockup_svg("mono-light", "#FFFFFF", h=52, fn=fn)}</div>'
        cols.append(f"""
<section class="col {'is-rec' if rec else ''}">
  <div class="head"><span class="letter serif">{key}</span>{badge}</div>
  <h2 class="serif">{esc(name)}</h2>
  <p class="desc">{esc(desc)}</p>
  <div class="hero">{icon(fn, 'color', 190)}</div>
  <div class="lock">{lockup_svg(fn=fn, h=64)}</div>
  {extra}
  <div class="pc"><h3>Strengths</h3><ul>{''.join(f'<li>{esc(p)}</li>' for p in pros)}</ul>
  <h3>Watch-outs</h3><ul class="cons">{''.join(f'<li>{esc(c)}</li>' for c in cons)}</ul></div>
</section>""")
    body = f"""
<header><div>{lockup_svg(h=46)}</div><div class="t"><div class="kicker">Brand exploration · 3 directions</div>
<h1 class="serif">Which mark should carry SELAR?</h1></div></header>
<main>{''.join(cols)}</main>
<footer><b>Recommendation: A, Linny.</b> The product asks students to do the thinking, so the brand should feel like a curious study companion rather than a machine.
A mascot that fetches an earlier page and then waits for your verdict acts out “AI suggests, you decide”. B and C stay in the kit as sober alternates. <span class="rp">Research prototype · UCSC IS4101 Group 04</span></footer>"""
    css = """
body { background:var(--mist); padding:44px 52px; }
header { display:flex; align-items:center; gap:40px; margin-bottom:28px; }
header .t .kicker { color:var(--slate); font-size:18px; }
header h1 { font-size:46px; font-weight:700; letter-spacing:-.01em; }
main { display:grid; grid-template-columns:1fr 1fr 1fr; gap:24px; align-items:stretch; }
.col { background:#fff; border-radius:24px; padding:28px 30px; border:2px solid #E2E8F0; display:flex; flex-direction:column; }
.col.is-rec { border:3px solid var(--forest); }
.head { display:flex; justify-content:space-between; align-items:center; }
.letter { font-size:40px; font-weight:800; color:var(--forest); }
.rec { background:var(--forest); color:#fff; padding:8px 16px; font-size:17px; }
.alt { color:var(--slate); font-weight:600; font-size:17px; }
h2 { font-size:32px; margin:6px 0 6px; }
.desc { color:var(--slate); font-size:18px; line-height:1.4; min-height:76px; text-wrap:pretty; }
.hero { display:flex; justify-content:center; padding:24px 0; background:var(--sky); border-radius:18px; margin:14px 0; }
.lock { margin:6px 0 12px; }
.poses { display:flex; gap:6px; justify-content:space-between; margin-bottom:10px; }
.poses.mono { justify-content:flex-start; gap:14px; }
.tile { width:96px; height:96px; border-radius:16px; background:var(--mist); display:flex; align-items:center; justify-content:center; }
.tile.dark { background:var(--ink); }
.tile.fav { background:#fff; border:2px dashed #CBD5E1; gap:10px; }
.row3 { height:96px; align-items:center; }
.darkstrip { background:var(--ink); border-radius:16px; justify-content:center; }
.pc { margin-top:auto; }
.pc h3 { font-size:15px; text-transform:uppercase; letter-spacing:.08em; color:var(--forest); margin:10px 0 4px; }
.pc ul { padding-left:20px; font-size:17px; line-height:1.4; }
.pc ul.cons li::marker { color:var(--rust); }
footer { margin-top:22px; font-size:19px; line-height:1.45; background:#fff; border-radius:18px; padding:18px 24px; border:2px solid #E2E8F0; }
footer .rp { display:block; color:var(--slate); font-size:15px; margin-top:6px; }
"""
    return render("brand-exploration-comparison", body, 1800, 1330, css)


# ------------------------------------------------------------------ OG card
def og():
    body = f"""
<div class="wrap">
  <div class="l">
    <div class="lk">{lockup_svg(h=78)}</div>
    <h1 class="serif">AI suggests.<br>Sources show.<br><span>You decide.</span></h1>
    <p>A PDF reader that spots possible links to your earlier reading and asks you to explain them.</p>
    <div class="pill rp"><span class="dot"></span>Research prototype · UCSC</div>
  </div>
  <div class="r">{pose('linking', 470)}</div>
</div>"""
    css = """
body { background:var(--mist); }
.wrap { display:grid; grid-template-columns:1fr 470px; height:630px; padding:54px 40px 54px 70px; align-items:center; background:
  radial-gradient(circle at 88% 45%, var(--sky) 0 250px, transparent 251px); }
.lk { margin-bottom:26px; }
h1 { font-size:66px; line-height:1.02; font-weight:700; letter-spacing:-.015em; }
h1 span { color:var(--forest); }
p { font-size:25px; line-height:1.35; color:var(--slate); margin:20px 0 22px; max-width:560px; }
.rp { background:#fff; border:2px solid #CBD5E1; padding:8px 18px; font-size:20px; color:var(--ink); }
"""
    return render("og-image", body, 1200, 630, css)


# ------------------------------------------------------------------ flyer
def flyer(pdf=False):
    steps = "".join(f'<li><b class="n serif">{i+1}</b><div><b>{esc(t)}</b><span>{esc(d)}</span></div></li>' for i, (t, d) in enumerate(C.STEPS))
    survey = "".join(f'<div class="stat"><b class="serif">{esc(n)}</b><span>{esc(d)}</span></div>' for n, d in C.SURVEY)
    body = f"""
<div class="page">
  <div class="top">
    <div>{lockup_svg(h=64)}</div>
    <div class="pill tag"><span class="dot"></span>Research prototype</div>
  </div>
  <div class="hero">
    <div class="ht">
      <div class="kicker">Pilot testers wanted</div>
      <h1 class="serif">Every new page has an <em>old friend</em>.</h1>
      <p class="lede">SELAR is a PDF reader we are building at UCSC. While you read, it suggests a possible link to something you read earlier, shows you both passages, and asks you to explain and decide.</p>
    </div>
    <div class="hm">{pose('linking', 300)}</div>
  </div>
  <div class="principle serif">AI suggests, sources show, <span>you explain and decide.</span></div>
  <div class="grid">
    <section><h2>How it works</h2><ol class="steps">{steps}</ol></section>
    <section class="why"><h2>Why we built it this way</h2>
      <p>From our formative needs survey (38 eligible respondents; 36 answered these items):</p>{survey}
      <p class="note">A needs survey, not an evaluation of SELAR.</p>
      <p class="ground">{esc(C.GROUNDING)}</p>
    </section>
  </div>
  <div class="cta">
    <div class="qr">{qr_svg(C.APP_URL, 150)}</div>
    <div class="ct"><h2 class="serif">Register your interest</h2>
      <p>Scan to open the prototype at <b style="white-space:nowrap">{C.APP_URL_SHORT}</b> or tell any of us you would like to try it. It is an early prototype, so rough edges are expected and honest feedback is the most useful thing you can give.</p></div>
    <div class="cm">{pose('hello', 130)}</div>
  </div>
  <footer><b>{esc(C.TEAM)}</b>, University of Colombo School of Computing<br>{esc(', '.join(C.MEMBERS))} · Supervisor: {esc(C.SUPERVISOR)}</footer>
</div>"""
    css = """
body { background:#fff; }
.page { width:794px; height:1123px; padding:36px 46px 34px; display:flex; flex-direction:column; background:var(--paper); position:relative; overflow:hidden; }
.page::before { content:''; position:absolute; right:-110px; top:120px; width:420px; height:420px; border-radius:50%; background:var(--sky); z-index:0; }
.page > * { position:relative; z-index:1; }
.top { display:flex; justify-content:space-between; align-items:center; }
.tag { background:var(--blush); color:var(--rust-ink); padding:7px 14px; font-size:14px; }
.hero { display:grid; grid-template-columns:1fr 300px; align-items:center; margin-top:14px; }
.kicker { color:var(--forest); font-size:15px; }
h1 { font-size:50px; line-height:1.02; font-weight:700; margin:8px 0 12px; letter-spacing:-.015em; }
h1 em { font-style:normal; color:var(--forest); background:linear-gradient(transparent 62%, var(--sun) 62% 90%, transparent 90%); }
.lede { font-size:17px; line-height:1.45; color:#334155; }
.principle { font-size:25px; font-weight:650; padding:14px 22px; border-radius:16px; background:var(--forest); color:#fff; margin:6px 0 18px; }
.principle span { color:var(--sun); }
.grid { display:grid; grid-template-columns:1fr 1fr; gap:26px; flex:1; }
h2 { font-size:19px; font-weight:800; color:var(--forest); margin-bottom:10px; }
.steps { list-style:none; display:flex; flex-direction:column; gap:14px; }
.steps li { display:flex; gap:12px; align-items:flex-start; }
.steps .n { flex:0 0 34px; height:34px; border-radius:50%; background:var(--sky); color:var(--forest); display:flex; align-items:center; justify-content:center; font-size:19px; }
.steps b { display:block; font-size:16px; }
.steps span { font-size:15px; color:var(--slate); line-height:1.35; }
.why p { font-size:15px; color:#334155; line-height:1.4; }
.stat { display:flex; gap:12px; align-items:baseline; padding:7px 0; border-bottom:1.5px dashed #CBD5E1; }
.stat b { font-size:24px; color:var(--rust-ink); flex:0 0 92px; }
.stat span { font-size:15px; line-height:1.3; }
.why .note { font-size:13px; margin-top:6px; }
.why .ground { margin-top:10px; font-size:14.5px; color:var(--ink); }
.cta { display:grid; grid-template-columns:150px 1fr 130px; gap:20px; align-items:center; background:#fff; border:2.5px solid var(--ink); border-radius:20px; padding:16px 18px; margin-top:14px; box-shadow:6px 6px 0 var(--sky); }
.qr svg { display:block; }
.ct h2 { font-size:26px; color:var(--ink); margin-bottom:6px; }
.ct p { font-size:14.5px; line-height:1.4; color:var(--slate); }
footer { font-size:12px; color:#334155; margin-top:12px; line-height:1.45; }
"""
    if pdf:
        css += "@page { size:A4; margin:0; } html,body{width:210mm;height:297mm;} .page{width:210mm;height:297mm;}"
        return render("selar-flyer-a4", body, 794, 1123, css, pdf=True)
    return render("selar-flyer-a4", body, 794, 1123, css, scale=2.5)


# ------------------------------------------------------------------ social
SOCIAL_CSS = """
body { background:var(--paper); }
.s { width:100%; height:100%; padding:80px 84px; display:flex; flex-direction:column; position:relative; overflow:hidden; }
.s .kicker { font-size:26px; color:var(--forest); }
.s h1 { font-size:84px; line-height:1.03; font-weight:700; letter-spacing:-.015em; margin:22px 0 26px; }
.s h1 em { font-style:normal; color:var(--forest); }
.s p.b { font-size:38px; line-height:1.4; color:#334155; text-wrap:pretty; }
.s .mid { flex:1; display:flex; flex-direction:column; justify-content:flex-start; }
.s .foot { margin-top:auto; display:flex; justify-content:space-between; align-items:center; }
.s .rp { background:var(--blush); color:var(--rust-ink); padding:12px 24px; font-size:26px; }
.s .pg { font-size:26px; color:var(--slate); font-weight:700; }
.blob { position:absolute; border-radius:50%; background:var(--sky); z-index:0; }
.s > *:not(.blob) { position:relative; z-index:1; }
.steps { list-style:none; display:flex; flex-direction:column; gap:30px; margin-top:12px; }
.steps li { display:flex; gap:24px; align-items:flex-start; }
.steps .n { flex:0 0 64px; height:64px; border-radius:50%; background:var(--forest); color:#fff; display:flex; align-items:center; justify-content:center; font-size:34px; }
.steps b { display:block; font-size:42px; }
.steps span { font-size:32px; color:#334155; line-height:1.32; }
.stat { display:flex; gap:26px; align-items:baseline; padding:34px 0; border-bottom:3px dashed #CBD5E1; }
.stat b { font-size:58px; color:var(--rust-ink); flex:0 0 230px; }
.stat span { font-size:36px; line-height:1.3; text-wrap:balance; }
.note { font-size:26px; color:#334155; margin-top:30px; line-height:1.4; }
"""


def carousel():
    outs = []
    for i, sl in enumerate(C.CAROUSEL, 1):
        if sl.get("steps"):
            inner = '<ol class="steps">' + "".join(f'<li><b class="n serif">{k+1}</b><div><b>{esc(t)}</b><span>{esc(d)}</span></div></li>' for k, (t, d) in enumerate(C.STEPS)) + '</ol>'
            title = 'AI suggests. Sources show. <em>You decide.</em>'
        elif sl.get("survey"):
            inner = "".join(f'<div class="stat"><b class="serif">{esc(n)}</b><span>{esc(d)}</span></div>' for n, d in C.SURVEY)
            inner += f'<p class="note">From our formative needs survey: 38 eligible respondents, 36 answered these items. It is not an evaluation of SELAR.</p>'
            title = 'You asked for links, <em>and</em> for honesty about AI.'
        else:
            inner = f'<p class="b">{esc(sl["body"])}</p>'
            title = 'Ever read something and thought, <em>“wait, I’ve seen this before”?</em>'
        if sl["pose"] == "thinking":
            art = f'<div class="blob" style="width:700px;height:700px;right:-170px;bottom:120px"></div><div style="position:absolute;right:50px;bottom:170px;z-index:1">{pose("thinking", 500)}</div>'
            hstyle, istyle, kick = "max-width:900px", "max-width:470px", "Hello from SELAR"
        else:
            art = f'<div class="blob" style="width:420px;height:420px;right:-90px;top:-70px"></div><div style="position:absolute;right:34px;top:44px;z-index:1">{pose(sl["pose"], 270)}</div>'
            hstyle, istyle, kick = "max-width:690px", "", sl["kicker"]
        body = f"""<div class="s">{art}
<div>{lockup_svg(h=70)}</div>
<div class="kicker" style="margin-top:64px">{esc(kick)}</div>
<h1 class="serif" style="{hstyle}">{title}</h1>
<div class="mid" style="{istyle}">{inner}</div>
<div class="foot"><span class="pill rp"><span class="dot"></span>Research prototype · feedback welcome</span><span class="pg">{i} / 3</span></div>
</div>"""
        outs.append(render(f"carousel-{i:02d}", body, 1080, 1350, SOCIAL_CSS, outdir=os.path.join(OUT, "social")))
    return outs


def squares():
    outs = []
    b1 = f"""<div class="s" style="background:var(--forest);color:#fff">
<div class="blob" style="width:620px;height:620px;right:-200px;bottom:-220px;background:var(--sky)"></div>
<div>{lockup_svg('dark', '#FFFFFF', h=70)}</div>
<div class="kicker" style="margin-top:70px;color:var(--sun)">Our one design rule</div>
<h1 class="serif" style="font-size:96px;color:#fff;margin-top:20px">AI suggests.<br>Sources show.<br><span style="color:var(--sun)">You explain<br>and decide.</span></h1>
<div style="position:absolute;right:56px;bottom:120px;z-index:1">{pose('celebrating', 300)}</div>
<div class="foot"><span class="pill rp" style="background:#fff;color:var(--forest)"><span class="dot"></span>Research prototype</span></div>
</div>"""
    outs.append(render("square-01-principle", b1, 1080, 1080, SOCIAL_CSS, outdir=os.path.join(OUT, "social")))
    b2 = f"""<div class="s">
<div class="blob" style="width:640px;height:640px;right:-200px;top:250px"></div>
<div>{lockup_svg(h=64)}</div>
<div class="kicker" style="margin-top:46px">Meet Linny</div>
<h1 class="serif" style="font-size:76px;max-width:540px">Linny points out a link. <em>You</em> decide if it holds.</h1>
<p class="b" style="max-width:520px;font-size:32px">Rejecting a weak link counts as good thinking too.</p>
<div style="position:absolute;right:30px;top:300px;z-index:1">{pose('questioning', 470)}</div>
<div class="foot"><span class="pill rp"><span class="dot"></span>Research prototype · feedback welcome</span></div>
</div>"""
    outs.append(render("square-02-linny", b2, 1080, 1080, SOCIAL_CSS, outdir=os.path.join(OUT, "social")))
    return outs


if __name__ == "__main__":
    what = sys.argv[1:] or ["comparison", "og", "flyer", "carousel", "squares"]
    for wname in what:
        if wname == "flyer":
            print(flyer()); print(flyer(pdf=True))
        else:
            print(globals()[wname]())
