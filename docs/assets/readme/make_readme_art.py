"""Draw the README illustrations for SELAR with Pillow (programmatic, not AI-generated).

Outputs (1600 px wide, opaque cream background so they read in light and dark
GitHub themes) next to this script:

    hero.png           wordmark, descriptor line, Linny, research-prototype pill
    learning-loop.png  the current reading workflow (illustration, not a screenshot)
    architecture.png   console -> API -> PostgreSQL/pgvector, worker -> Gemini

Inputs reused from the brand kit in this repository:
    brand/exports/logo/png/selar-wordmark-ink.png
    brand/exports/mascot/png/linny-<pose>-1024.png

Fonts: Fraunces and Figtree variable TTFs (SIL Open Font License 1.1; licence
texts in brand/fonts/). They are not vendored, matching brand/README.md. Put
`Fraunces[SOFT,WONK,opsz,wght].ttf` and `Figtree[wght].ttf` in brand/fonts/ or
point SELAR_BRAND_FONTS at a directory that holds them.

    pip install pillow
    python docs/assets/readme/make_readme_art.py
"""
from __future__ import annotations

import math
import os
from pathlib import Path

from PIL import Image, ImageDraw, ImageFilter, ImageFont

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[2]
BRAND = ROOT / "brand"
FONTS = Path(os.environ.get("SELAR_BRAND_FONTS", BRAND / "fonts"))
MASCOT = BRAND / "exports/mascot/png"
LOGO = BRAND / "exports/logo/png"
MAX_BYTES = 400 * 1024

# Brand palette (brand/DESIGN.md)
FOREST = "#1F5135"
MOSS = "#5B8A6F"
INK = "#0F172A"
SLATE = "#475569"
LINE = "#E2E8F0"
PAPER = "#FFFDF8"
SKY = "#E1EDFB"
RUST = "#B4532A"
RUST_INK = "#9A4322"
BLUSH = "#FBEDE6"
SUN = "#F6C453"
WHITE = "#FFFFFF"

W = 1600
FRAUNCES = ""
FIGTREE = ""


def _font_file(*names: str) -> str:
    for name in names:
        p = FONTS / name
        if p.exists():
            return str(p)
    raise SystemExit(f"Missing font {names[0]} in {FONTS}; see the module docstring.")


def fraunces(size: int, wght: int = 680) -> ImageFont.FreeTypeFont:
    f = ImageFont.truetype(FRAUNCES, size)
    f.set_variation_by_axes([72 if size > 40 else 24, wght, 100, 0])  # opsz, wght, SOFT, WONK
    return f


def fit_fraunces(d, text: str, size: int, width: float) -> ImageFont.FreeTypeFont:
    """Largest Fraunces size <= size whose rendering of text fits width."""
    while d.textlength(text, font=fraunces(size)) > width:
        size -= 1
    return fraunces(size)


def figtree(size: int, weight: str = "Regular") -> ImageFont.FreeTypeFont:
    f = ImageFont.truetype(FIGTREE, size)
    f.set_variation_by_name(weight)
    return f


def linny(pose: str, size: int) -> Image.Image:
    im = Image.open(MASCOT / f"linny-{pose}-1024.png").convert("RGBA")
    im = im.crop(im.getbbox())
    im.thumbnail((size, size), Image.Resampling.LANCZOS)
    return im


def paste_center(canvas: Image.Image, im: Image.Image, cx: float, cy: float) -> None:
    canvas.alpha_composite(im, (int(cx - im.width / 2), int(cy - im.height / 2)))


def blob(d, cx, cy, r, color) -> None:
    d.ellipse((cx - r, cy - r, cx + r, cy + r), fill=color)


def pill(d, x, y, text, font, bg, fg, padx=22, pady=10, outline=None):
    w = d.textlength(text, font=font)
    asc, desc = font.getmetrics()
    h = asc + desc
    box = (x, y, x + w + 2 * padx, y + h + 2 * pady)
    d.rounded_rectangle(box, radius=(h + 2 * pady) // 2, fill=bg, outline=outline, width=2 if outline else 0)
    d.text((x + padx, y + pady), text, font=font, fill=fg)
    return box


def wrap(d, text, font, width):
    lines, cur = [], ""
    for word in text.split():
        t = (cur + " " + word).strip()
        if d.textlength(t, font=font) <= width:
            cur = t
        else:
            lines.append(cur)
            cur = word
    if cur:
        lines.append(cur)
    return lines


def para(d, x, y, text, font, fill, width, gap=1.34):
    lh = int(font.size * gap)
    for line in wrap(d, text, font, width):
        assert d.textlength(line, font=font) <= width + 1, line
        d.text((x, y), line, font=font, fill=fill)
        y += lh
    return y


def shadow_card(canvas, box, radius=28, fill=WHITE, outline=LINE):
    x0, y0, x1, y1 = box
    sh = Image.new("RGBA", canvas.size, (0, 0, 0, 0))
    ImageDraw.Draw(sh).rounded_rectangle((x0, y0 + 8, x1, y1 + 8), radius=radius, fill=(15, 23, 42, 24))
    canvas.alpha_composite(sh.filter(ImageFilter.GaussianBlur(12)))
    ImageDraw.Draw(canvas).rounded_rectangle(box, radius=radius, fill=fill, outline=outline, width=2 if outline else 0)


def arrow(d, p0, p1, color=RUST, width=6, head=16):
    (x0, y0), (x1, y1) = p0, p1
    ang = math.atan2(y1 - y0, x1 - x0)
    bx, by = x1 - head * math.cos(ang), y1 - head * math.sin(ang)
    d.line((x0, y0, bx, by), fill=color, width=width)
    left = (bx + head * 0.7 * math.cos(ang + math.pi / 2), by + head * 0.7 * math.sin(ang + math.pi / 2))
    right = (bx + head * 0.7 * math.cos(ang - math.pi / 2), by + head * 0.7 * math.sin(ang - math.pi / 2))
    d.polygon([(x1, y1), left, right], fill=color)


def frame(canvas):
    """Thin border so the panel has a defined edge on dark GitHub themes."""
    ImageDraw.Draw(canvas).rectangle((0, 0, canvas.width - 1, canvas.height - 1), outline="#E7E0D0", width=2)


def save(canvas: Image.Image, name: str) -> Path:
    frame(canvas)
    out = HERE / name
    rgb = canvas.convert("RGB")
    rgb.save(out, optimize=True)
    if out.stat().st_size > MAX_BYTES:
        rgb.quantize(colors=256, method=Image.Quantize.MEDIANCUT, dither=Image.Dither.NONE).save(out, optimize=True)
    size = out.stat().st_size
    assert size <= MAX_BYTES, f"{name} is {size} bytes"
    print(f"saved {out.relative_to(ROOT)} {canvas.size} {size // 1024} KB")
    return out


# ── 1. Hero ────────────────────────────────────────────────────────────────
def hero():
    c = Image.new("RGBA", (W, 600), PAPER)
    d = ImageDraw.Draw(c)
    blob(d, 1250, 300, 240, SKY)
    blob(d, 1462, 112, 40, "#FBE7B5")
    paste_center(c, linny("linking", 410), 1250, 305)
    d = ImageDraw.Draw(c)

    mark = Image.open(LOGO / "selar-wordmark-ink.png").convert("RGBA")
    mark = mark.crop(mark.getbbox())
    mark.thumbnail((330, 126), Image.Resampling.LANCZOS)
    c.alpha_composite(mark, (80, 58))
    y = 58 + mark.height + 26
    d.text((82, y), "Semantic Linking", font=fraunces(60), fill=INK)
    d.text((82, y + 70), "for Active Retention", font=fraunces(60), fill=FOREST)
    y += 168
    y = para(d, 84, y, "Read across papers and notes, compare suggested links against their source passages, and practise recall.",
             figtree(28, "Medium"), SLATE, 860)
    box = pill(d, 82, y + 18, "Research prototype", figtree(26, "Bold"), RUST, WHITE)
    assert box[3] <= c.height - 40, box
    return save(c, "hero.png")


# ── 2. Learning loop ───────────────────────────────────────────────────────
STEPS = [
    ("Add readings", "PDF, Markdown, TXT, DOCX, web articles or pasted text.", "reading", SKY),
    ("Before-reading warm-up", "Optional recall questions before you open the source.", "thinking", BLUSH),
    ("Read and compare", "Reflection prompts put linked passages side by side.", "linking", SKY),
    ("End-reading check", "Answer from recall, then compare the feedback with a quoted source.", "questioning", BLUSH),
    ("Daily review and streak", "Due questions come back on a simple schedule. Streaks count UTC days.", "hello", SKY),
    ("Progress", "Observed practice records. Not a mastery score.", None, BLUSH),
]


def progress_icon(c, cx, cy):
    d = ImageDraw.Draw(c)
    x0, base = cx - 78, cy + 50
    for i, h in enumerate([40, 64, 52, 86, 106]):
        x = x0 + i * 34
        d.rounded_rectangle((x, base - h, x + 24, base), radius=7, fill=[MOSS, FOREST][i % 2])
    d.line((x0 - 8, base + 4, x0 + 168, base + 4), fill=INK, width=4)


def learning_loop():
    H = 1170
    c = Image.new("RGBA", (W, H), PAPER)
    d = ImageDraw.Draw(c)
    d.text((60, 42), "THE LEARNING LOOP", font=figtree(24, "Bold"), fill=FOREST)
    d.text((60, 78), "Read, compare, recall, review.", font=fraunces(56), fill=INK)
    d.text((62, 154), "How a reading session flows in the current prototype.", font=figtree(27), fill=SLATE)

    cw, ch, gap = 440, 380, 80
    left = (W - (3 * cw + 2 * gap)) // 2
    tops = [236, 716]
    boxes = []
    for i, (title, sub, pose, tint) in enumerate(STEPS):
        row, col = divmod(i, 3)
        x0 = left + col * (cw + gap)
        y0 = tops[row]
        boxes.append((x0, y0, x0 + cw, y0 + ch))
        shadow_card(c, (x0, y0, x0 + cw, y0 + ch))
        d = ImageDraw.Draw(c)
        cx, cy = x0 + 112, y0 + 108
        blob(d, cx, cy, 78, tint)
        if pose:
            paste_center(c, linny(pose, 132), cx, cy)
        else:
            progress_icon(c, cx, cy)
        d = ImageDraw.Draw(c)
        blob(d, x0 + cw - 60, y0 + 58, 30, RUST)
        d.text((x0 + cw - 60, y0 + 58), str(i + 1), font=fraunces(34), fill=WHITE, anchor="mm")
        ty = y0 + 206
        tf = fit_fraunces(d, title, 36, cw - 64)
        assert tf.size >= 30, title
        d.text((x0 + 32, ty + 36), title, font=tf, fill=INK, anchor="ls")
        end = para(d, x0 + 32, ty + 52, sub, figtree(24), SLATE, cw - 64)
        assert end <= y0 + ch - 20, (title, end)

    d = ImageDraw.Draw(c)
    # UI labels on the compare card (labels taken from the reader UI)
    x0, y0, x1, y1 = boxes[2]
    pill(d, x0 + 214, y0 + 100, "Compare passages", figtree(19, "Bold"), WHITE, FOREST, padx=14, pady=7, outline=FOREST)
    pill(d, x0 + 214, y0 + 150, "This link is wrong", figtree(19, "Bold"), BLUSH, RUST_INK, padx=14, pady=7, outline=RUST)
    # row arrows
    for a, b in [(0, 1), (1, 2), (3, 4), (4, 5)]:
        ya = boxes[a][1] + 108
        arrow(d, (boxes[a][2] + 14, ya), (boxes[b][0] - 14, ya))
    # 3 -> 4: down from card 3, left along the gutter, down into card 4
    gy = (tops[0] + ch + tops[1]) // 2
    sx = (boxes[2][0] + boxes[2][2]) // 2
    ex = (boxes[3][0] + boxes[3][2]) // 2
    d.line((sx, boxes[2][3] + 12, sx, gy), fill=RUST, width=6)
    d.line((ex - 3, gy, sx + 3, gy), fill=RUST, width=6)
    arrow(d, (ex, gy - 3), (ex, boxes[3][1] - 12))
    d.text((60, H - 50), "Illustration, not a screenshot. It shows the workflow, not measured learning gains.",
           font=figtree(22, "Medium"), fill=SLATE)
    return save(c, "learning-loop.png")


# ── 3. Architecture ────────────────────────────────────────────────────────
def node(c, box, title, tech, detail, accent):
    shadow_card(c, box, radius=24)
    d = ImageDraw.Draw(c)
    x0, y0, x1, y1 = box
    d.rounded_rectangle((x0, y0, x0 + 16, y1), radius=8, fill=accent)
    d.rectangle((x0 + 8, y0, x0 + 16, y1), fill=accent)
    d.text((x0 + 40, y0 + 26), tech.upper(), font=figtree(19, "Bold"), fill=accent)
    assert d.textlength(tech.upper(), font=figtree(19, "Bold")) <= x1 - x0 - 60, tech
    d.text((x0 + 40, y0 + 54), title, font=fraunces(36), fill=INK)
    assert d.textlength(title, font=fraunces(36)) <= x1 - x0 - 60, title
    para(d, x0 + 40, y0 + 108, detail, figtree(22), SLATE, x1 - x0 - 70)


def label(d, x, y, text, anchor="mm"):
    f = figtree(20, "SemiBold")
    tw = d.textlength(text, font=f)
    if anchor == "mm":
        box = (x - tw / 2 - 12, y - 18, x + tw / 2 + 12, y + 18)
    else:  # starts at x
        box = (x, y - 18, x + tw + 24, y + 18)
    d.rounded_rectangle(box, radius=18, fill=PAPER, outline=LINE, width=2)
    d.text(((box[0] + box[2]) / 2, y), text, font=f, fill=RUST_INK, anchor="mm")
    return box


def architecture():
    H = 880
    c = Image.new("RGBA", (W, H), PAPER)
    d = ImageDraw.Draw(c)
    d.text((60, 42), "ARCHITECTURE", font=figtree(24, "Bold"), fill=FOREST)
    d.text((60, 78), "How the services fit together", font=fraunces(56), fill=INK)
    d.text((62, 154), "Model-backed processing runs in the worker and calls Google Gemini in the cloud.", font=figtree(27), fill=SLATE)

    nw, nh = 420, 240
    y1, y2 = 260, 580
    console = (60, y1, 60 + nw, y1 + nh)
    api = (590, y1, 590 + nw, y1 + nh)
    db = (1120, y1, 1120 + nw, y1 + nh)
    worker = (590, y2, 590 + nw, y2 + nh)
    gemini = (1120, y2, 1120 + nw, y2 + nh)

    node(c, console, "Console", "Next.js · selar-console", "Library, Reader, Review, Progress, Chat and Graph in the browser.", FOREST)
    node(c, api, "API", "Go · selar-api", "Accounts, documents, practice, review schedule and chat routing.", FOREST)
    node(c, db, "PostgreSQL", "with pgvector", "Documents, chunks, embeddings, practice records and the processing queue.", MOSS)
    node(c, worker, "Worker", "Python · selar-worker", "Ingestion, embeddings, reflection prompts, practice and cited chat.", RUST)
    node(c, gemini, "Google Gemini", "Cloud model provider", "Embeddings and generation. Requests can use quota or incur charges.", RUST)

    d = ImageDraw.Draw(c)
    my = y1 + nh // 2
    arrow(d, (console[2] + 10, my), (api[0] - 10, my))
    label(d, (console[2] + api[0]) / 2, my - 40, "HTTP")
    arrow(d, (api[2] + 10, my), (db[0] - 10, my))
    label(d, (api[2] + db[0]) / 2, my - 40, "SQL")
    ax = (api[0] + api[2]) // 2
    arrow(d, (ax, api[3] + 12), (ax, worker[1] - 12))
    label(d, ax + 20, (api[3] + worker[1]) / 2, "HTTP", anchor="lm")
    wy = y2 + nh // 2
    arrow(d, (worker[2] + 10, wy), (gemini[0] - 10, wy))
    label(d, (worker[2] + gemini[0]) / 2, wy - 40, "HTTPS")
    # worker -> database: elbow through the column gap and the row gutter
    gx = (worker[2] + gemini[0]) // 2
    gy = (db[3] + worker[1]) // 2
    tx = db[0] + 60
    d.line((worker[2] + 10, worker[1] + 44, gx + 3, worker[1] + 44), fill=RUST, width=6)
    d.line((gx, worker[1] + 47, gx, gy - 3), fill=RUST, width=6)
    d.line((gx - 3, gy, tx + 3, gy), fill=RUST, width=6)
    arrow(d, (tx, gy + 3), (tx, db[3] + 10))
    label(d, tx + 24, gy, "SQL (worker reads the queue)", anchor="lm")

    # Linny in the free lower-left space
    blob(d, 270, 700, 118, SKY)
    paste_center(c, linny("reading", 180), 270, 700)
    d = ImageDraw.Draw(c)
    d.text((60, H - 46), "Simplified illustration of the repository's services. See ARCHITECTURE.md for detail.",
           font=figtree(22, "Medium"), fill=SLATE)
    return save(c, "architecture.png")


def main():
    global FRAUNCES, FIGTREE
    FRAUNCES = _font_file("Fraunces[SOFT,WONK,opsz,wght].ttf", "Fraunces.ttf")
    FIGTREE = _font_file("Figtree[wght].ttf", "Figtree.ttf")
    hero()
    learning_loop()
    architecture()


if __name__ == "__main__":
    main()
