"""Shared helpers for the SELAR brand build: fonts -> outlined SVG paths, rendering."""
import os, subprocess, functools, tempfile, shutil
from fontTools.ttLib import TTFont
from fontTools.varLib import instancer
from fontTools.pens.svgPathPen import SVGPathPen
from fontTools.pens.transformPen import TransformPen
from fontTools.pens.boundsPen import BoundsPen

ROOT = os.environ.get("SELAR_BRAND_ROOT", os.path.abspath(os.path.join(os.path.dirname(__file__), "..", "build-out")))
FONTS = os.environ.get("SELAR_BRAND_FONTS", os.path.join(os.path.dirname(__file__), "..", "fonts"))
CHROME = os.environ.get("CHROME", "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome")

PAL = {
    "forest": "#1F5135",   # deep green (primary)
    "moss": "#5B8A6F",     # mid green
    "ink": "#0F172A",
    "slate": "#475569",
    "mist": "#F1F5F9",     # light
    "sky": "#E1EDFB",      # sky tint
    "rust": "#B4532A",     # thread / accent
    "blush": "#FBEDE6",    # rust tint
    "amber": "#D97706",
    "sun": "#F6C453",      # playful accent (decorative only, never text on light)
    "paper": "#FFFDF8",    # warm page white
    "white": "#FFFFFF",
}

FONT_SPECS = {
    "fraunces": ("Fraunces[SOFT,WONK,opsz,wght].ttf", {"wght": 650, "opsz": 72, "SOFT": 100, "WONK": 0}),
    "fraunces-bold": ("Fraunces[SOFT,WONK,opsz,wght].ttf", {"wght": 760, "opsz": 72, "SOFT": 100, "WONK": 0}),
    "figtree": ("Figtree[wght].ttf", {"wght": 500}),
    "figtree-bold": ("Figtree[wght].ttf", {"wght": 700}),
}


@functools.lru_cache(None)
def font(key):
    fn, loc = FONT_SPECS[key]
    f = TTFont(os.path.join(FONTS, fn))
    return instancer.instantiateVariableFont(f, loc)


def text_path(text, key="fraunces", size=100, x=0, y=0, tracking=0.0):
    """Return (svg path d, advance width) for text with baseline at (x, y)."""
    f = font(key)
    upm = f["head"].unitsPerEm
    cmap = f.getBestCmap()
    gs = f.getGlyphSet()
    hmtx = f["hmtx"]
    s = size / upm
    pen = SVGPathPen(gs)
    cx = 0.0
    for ch in text:
        gname = cmap[ord(ch)]
        tp = TransformPen(pen, (s, 0, 0, -s, x + cx * s, y))
        gs[gname].draw(tp)
        cx += hmtx[gname][0] + tracking * upm
    return pen.getCommands(), cx * s - tracking * upm * s


def svg_to_png(svg_path, png_path, width=None, height=None):
    import resvg_py
    kw = {"svg_path": svg_path}
    if width: kw["width"] = int(width)
    if height: kw["height"] = int(height)
    data = resvg_py.svg_to_bytes(**kw)
    with open(png_path, "wb") as fh:
        fh.write(bytes(data))


def chrome(html_path, out, w=1200, h=630, pdf=False):
    prof = tempfile.mkdtemp(prefix="chr-")
    args = [CHROME, "--headless=new", "--disable-gpu", "--hide-scrollbars", f"--user-data-dir={prof}",
            "--no-first-run", "--allow-file-access-from-files", "--force-device-scale-factor=1",
            "--virtual-time-budget=5000", "--run-all-compositor-stages-before-draw"]
    if pdf:
        args += [f"--print-to-pdf={out}", "--no-pdf-header-footer"]
    else:
        args += [f"--window-size={w},{h}", f"--screenshot={out}"]
    args.append("file://" + os.path.abspath(html_path))
    subprocess.run(args, check=True, capture_output=True, timeout=180)
    shutil.rmtree(prof, ignore_errors=True)


def font_face_css():
    return f"""
@font-face {{ font-family: 'Fraunces'; src: url('file://{FONTS}/Fraunces[SOFT,WONK,opsz,wght].ttf'); font-weight: 100 900; }}
@font-face {{ font-family: 'Figtree'; src: url('file://{FONTS}/Figtree[wght].ttf'); font-weight: 300 900; }}
@font-face {{ font-family: 'JetBrains Mono'; src: url('file://{FONTS}/JetBrainsMono[wght].ttf'); font-weight: 100 800; }}
"""
