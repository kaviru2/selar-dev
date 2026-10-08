"""SELAR logo system: three directions + final lockups. Everything is outlined SVG paths."""
from common import PAL, text_path, font

INK = PAL["ink"]
CREAM = "#FFF6E6"


# ---------- Direction A: Linny (recommended) ----------
# Linny the linnet fetching an earlier page: a round bird holding a page in its beak.
# The page carries a rust "linked passage" line, the same thread used across the system.

def _page(fill, stroke, hl, line, sw=3):
    # small page whose top edge is held under the beak (beak is drawn on top)
    return (f'<g transform="translate(50 56) scale(.86) translate(-50 -56) rotate(-10 50 58)">'
            f'<path d="M38 56 H56 L64 64 V84 H38 Z" fill="{fill}" stroke="{stroke}" stroke-width="{sw}" stroke-linejoin="round"/>'
            f'<path d="M56 56 V64 H64" fill="none" stroke="{stroke}" stroke-width="{sw*0.85:.1f}" stroke-linejoin="round"/>'
            f'<line x1="43" y1="70" x2="58" y2="70" stroke="{hl}" stroke-width="4.5" stroke-linecap="round"/>'
            f'<line x1="43" y1="77" x2="53" y2="77" stroke="{line}" stroke-width="3" stroke-linecap="round"/></g>')


def linny_icon(variant="color", page=True):
    """100x100 mark. variant: color | dark | mono-dark | mono-light"""
    s = []
    if variant in ("color", "dark"):
        if variant == "color":
            body, wing, belly, eye, beak, stroke = PAL["forest"], PAL["moss"], CREAM, INK, PAL["amber"], INK
            pfill, hl, ln = "#FFFFFF", PAL["rust"], "#94A3B8"
        else:
            body, wing, belly, eye, beak, stroke = PAL["moss"], "#8FB89F", CREAM, INK, "#F59E0B", CREAM
            pfill, hl, ln = "#FFFFFF", PAL["rust"], "#94A3B8"
        s.append(f'<path d="M41 18 C35 5 44 -1 53 3 C46 7 46 13 50 17 Z" fill="{body}" stroke="{stroke}" stroke-width="3.5" stroke-linejoin="round"/>')
        s.append(f'<path d="M44 92 v6 M56 92 v6" stroke="{beak}" stroke-width="4" stroke-linecap="round"/>')
        s.append(f'<circle cx="46" cy="52" r="38" fill="{body}" stroke="{stroke}" stroke-width="3.5"/>')
        s.append(f'<path d="M18 72 C24 60 68 60 74 72 C68 86 24 86 18 72 Z" fill="{belly}"/>')
        s.append(f'<circle cx="33" cy="42" r="6.5" fill="{eye}"/><circle cx="59" cy="42" r="6.5" fill="{eye}"/>')
        s.append('<circle cx="35.5" cy="39.5" r="2.2" fill="#FFFFFF"/><circle cx="61.5" cy="39.5" r="2.2" fill="#FFFFFF"/>')
        if page:
            s.append(_page(pfill, stroke, hl, ln))
        s.append(f'<path d="M37 51 L55 51 L46 62 Z" fill="{beak}" stroke="{stroke if variant=="color" else beak}" stroke-width="2.5" stroke-linejoin="round"/>')
        return "".join(s)
    fg = INK if variant == "mono-dark" else "#FFFFFF"
    ko = "#FFFFFF" if variant == "mono-dark" else INK
    s.append(f'<path d="M41 18 C35 5 44 -1 53 3 C46 7 46 13 50 17 Z" fill="{fg}"/>')
    s.append(f'<path d="M44 92 v6 M56 92 v6" stroke="{fg}" stroke-width="4" stroke-linecap="round"/>')
    s.append(f'<circle cx="46" cy="52" r="39.5" fill="{fg}"/>')
    s.append(f'<circle cx="33" cy="42" r="7.5" fill="{ko}"/><circle cx="59" cy="42" r="7.5" fill="{ko}"/>')
    s.append(f'<circle cx="34" cy="43" r="3.8" fill="{fg}"/><circle cx="60" cy="43" r="3.8" fill="{fg}"/>')
    if page:
        s.append(_page(ko, fg, fg, fg, sw=3.5))
    s.append(f'<path d="M37 51 L55 51 L46 63 Z" fill="{fg}" stroke="{ko}" stroke-width="3" stroke-linejoin="round"/>')
    return "".join(s)


def linny_tiny():
    """16 px cut: silhouette, big eyes, beak, plain page with one rust bar."""
    body = PAL["forest"]
    return (f'<path d="M38 20 C32 2 46 -4 58 2 C48 8 48 14 52 19 Z" fill="{body}"/>'
            f'<circle cx="46" cy="52" r="44" fill="{body}"/>'
            f'<circle cx="28" cy="40" r="13" fill="#FFFFFF"/><circle cx="64" cy="40" r="13" fill="#FFFFFF"/>'
            f'<circle cx="30" cy="42" r="7" fill="{INK}"/><circle cx="66" cy="42" r="7" fill="{INK}"/>'
            f'<rect x="40" y="62" width="44" height="36" fill="#FFFFFF"/>'
            f'<rect x="46" y="74" width="32" height="10" fill="{PAL["rust"]}"/>'
            f'<path d="M32 54 L60 54 L46 70 Z" fill="{PAL["amber"]}"/>')


def linny_small(variant="color"):
    """Favicon cut (16-48 px): no outline detail, chunkier page, same silhouette."""
    body = PAL["forest"] if variant == "color" else INK
    return (f'<path d="M41 18 C35 3 46 -3 56 2 C47 7 47 13 51 17 Z" fill="{body}"/>'
            f'<circle cx="46" cy="52" r="42" fill="{body}"/>'
            f'<circle cx="31" cy="42" r="9" fill="#FFFFFF"/><circle cx="61" cy="42" r="9" fill="#FFFFFF"/>'
            f'<circle cx="32" cy="43" r="5" fill="{INK}"/><circle cx="62" cy="43" r="5" fill="{INK}"/>'
            f'<g transform="rotate(-10 50 60)"><path d="M32 58 H60 L70 68 V92 H32 Z" fill="#FFFFFF" stroke="{INK}" stroke-width="4" stroke-linejoin="round"/>'
            f'<line x1="40" y1="78" x2="62" y2="78" stroke="{PAL["rust"]}" stroke-width="8" stroke-linecap="round"/></g>'
            f'<path d="M35 52 L57 52 L46 66 Z" fill="{PAL["amber"]}" stroke="{INK}" stroke-width="3" stroke-linejoin="round"/>')


# ---------- Direction B: Bridge between pages ----------
def bridge_icon(variant="color"):
    """Two pages side by side; a highlighted passage on each, bridged by one thread across the gutter."""
    mono = variant.startswith("mono")
    fg = INK if variant == "mono-dark" else "#FFFFFF"
    g1 = PAL["forest"] if not mono else fg
    g2 = PAL["moss"] if not mono else fg
    hl = PAL["rust"] if not mono else fg
    ln = "#FFFFFF" if variant != "mono-light" else INK
    s = [f'<rect x="4" y="14" width="40" height="72" rx="7" fill="{g1}"/>',
         f'<rect x="56" y="14" width="40" height="72" rx="7" fill="{g2}"/>']
    for x0 in (12, 64):
        for i, y in enumerate((30, 42, 54, 66)):
            s.append(f'<line x1="{x0}" y1="{y}" x2="{x0 + (24 if i % 2 else 20)}" y2="{y}" stroke="{ln}" stroke-width="4" stroke-linecap="round" opacity=".55"/>')
    # highlighted passages
    s.append(f'<line x1="12" y1="42" x2="36" y2="42" stroke="{ln}" stroke-width="6" stroke-linecap="round"/>')
    s.append(f'<line x1="64" y1="62" x2="88" y2="62" stroke="{ln}" stroke-width="6" stroke-linecap="round"/>')
    halo = "#FFFFFF" if variant != "mono-light" else INK
    d = "M36 42 C52 42 48 62 64 62"
    s.append(f'<path d="{d}" fill="none" stroke="{halo}" stroke-width="12" stroke-linecap="round"/>')
    s.append(f'<path d="{d}" fill="none" stroke="{hl}" stroke-width="6" stroke-linecap="round"/>')
    s.append(f'<circle cx="36" cy="42" r="6" fill="{hl}" stroke="{halo}" stroke-width="2.5"/><circle cx="64" cy="62" r="6" fill="{hl}" stroke="{halo}" stroke-width="2.5"/>')
    return "".join(s)


# ---------- Direction C: Knot / thread monogram ----------
KNOT_D = ("M76 20 C62 6 26 10 28 32 C30 50 64 44 70 60 C76 76 58 84 50 74 "
          "C44 66 54 58 62 64 C70 70 66 92 40 92 C30 92 24 88 20 82")


def knot_icon(variant="color"):
    mono = variant.startswith("mono")
    fg = INK if variant == "mono-dark" else "#FFFFFF"
    t = PAL["rust"] if not mono else fg
    n = PAL["forest"] if not mono else fg
    halo = "#FFFFFF" if variant != "mono-light" else INK
    return (f'<path d="{KNOT_D}" fill="none" stroke="{t}" stroke-width="11" stroke-linecap="round" stroke-linejoin="round"/>'
            f'<circle cx="76" cy="20" r="10" fill="{n}" stroke="{halo}" stroke-width="3"/>'
            f'<circle cx="20" cy="82" r="10" fill="{n}" stroke="{halo}" stroke-width="3"/>')


def wrap(inner, w, h, title, vb_x=0, vb_y=0):
    return (f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="{vb_x:.0f} {vb_y:.0f} {w:.0f} {h:.0f}" width="{w:.0f}" height="{h:.0f}" role="img" aria-label="{title}">'
            f'<title>{title}</title>{inner}</svg>')


WORD_SIZE = 100
WORD_KEY = "fraunces-bold"


def word_metrics():
    f = font(WORD_KEY)
    upm = f["head"].unitsPerEm
    os2 = f["OS/2"]
    return os2.sxHeight / upm * WORD_SIZE, os2.sCapHeight / upm * WORD_SIZE


def lockup(icon_fn, variant, word_color, sub=None, sub_color=None, stacked=False, text="selar"):
    """Returns (inner_svg, width, height). Icon is 100 units tall, optically centred on the x-height band."""
    xh, cap = word_metrics()
    word_d, ww = text_path(text, WORD_KEY, WORD_SIZE, 0, 0, -0.005)
    if stacked:
        ic = 190
        h = ic + 34 + cap + (60 if sub else 0) + 8
        width = max(ww, ic * 1.1) + 8
        bx = (width - ww) / 2
        parts = [f'<g transform="translate({(width-ic)/2 + 0.04*ic:.1f} 0) scale({ic/100})">{icon_fn(variant)}</g>']
        base = ic + 34 + cap
        parts.append(f'<path transform="translate({bx:.1f} {base:.1f})" d="{word_d}" fill="{word_color}"/>')
        if sub:
            sd, sw = text_path(sub, "figtree", 20, 0, 0, 0.02)
            parts.append(f'<path transform="translate({(width-sw)/2:.1f} {base+44:.1f})" d="{sd}" fill="{sub_color}"/>')
            width = max(width, sw + 8)
        return "".join(parts), width, h
    ic = 100
    gap = 22
    lines = [] if not sub else (sub if isinstance(sub, (list, tuple)) else [sub])
    sz, lh = 21, 26
    # text block: wordmark x-height band + optional subtitle lines
    base = cap + 4
    text_h = base + (8 + lh * len(lines) if lines else 0.22 * WORD_SIZE)
    h = max(ic, text_h)
    icon_y = (h - ic) / 2
    if not lines:
        base = icon_y + 54 + xh / 2  # x-height centred on Linny's head
    x0 = ic + gap
    parts = [f'<g transform="translate(0 {icon_y:.1f})">{icon_fn(variant)}</g>',
             f'<path transform="translate({x0} {base:.1f})" d="{word_d}" fill="{word_color}"/>']
    width = x0 + ww + 6
    y = base + 8 + lh - 4
    for ln in lines:
        sd, sw = text_path(ln, "figtree", sz, 0, 0, 0.01)
        parts.append(f'<path transform="translate({x0+3} {y:.1f})" d="{sd}" fill="{sub_color}"/>')
        width = max(width, x0 + sw + 8)
        y += lh
    return "".join(parts), width, h


def wordmark_only(color):
    xh, cap = word_metrics()
    d, ww = text_path("selar", WORD_KEY, WORD_SIZE, 0, 0, -0.005)
    asc = cap * 1.06
    return f'<path transform="translate(4 {asc+4:.1f})" d="{d}" fill="{color}"/>', ww + 8, asc + 4 + 0.24 * WORD_SIZE
