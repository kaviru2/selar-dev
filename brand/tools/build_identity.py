"""Build the SELAR identity exports into out/brand (logos, favicons, mascot, exploration)."""
import os, io, json, shutil, sys
sys.path.insert(0, os.path.dirname(__file__))
from PIL import Image
import logos, mascot, common
from common import PAL, svg_to_png

OUT = os.path.join(common.ROOT, "out", "brand")
INK = PAL["ink"]


def w(path, text):
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, "w") as fh:
        fh.write(text)


def png(svg_path, png_path, width):
    os.makedirs(os.path.dirname(png_path), exist_ok=True)
    svg_to_png(svg_path, png_path, width=width)


def build_logos():
    L = os.path.join(OUT, "logo")
    # icons
    for v, name in [("color", "color"), ("dark", "on-dark"), ("mono-dark", "mono-ink"), ("mono-light", "mono-white")]:
        p = f"{L}/svg/selar-icon-{name}.svg"
        w(p, logos.wrap(logos.linny_icon(v), 100, 100, "SELAR logo: Linny the linnet holding a page"))
        png(p, f"{L}/png/selar-icon-{name}-512.png", 512)
        png(p, f"{L}/png/selar-icon-{name}-1024.png", 1024)
    # lockups
    combos = [("color", INK, "light", PAL["slate"]), ("dark", "#FFFFFF", "on-dark", "#CBD5E1"),
              ("mono-dark", INK, "mono-ink", INK), ("mono-light", "#FFFFFF", "mono-white", "#FFFFFF")]
    for v, wc, name, sc in combos:
        inner, ww, hh = logos.lockup(logos.linny_icon, v, wc)
        p = f"{L}/svg/selar-lockup-{name}.svg"
        w(p, logos.wrap(inner, ww, hh, "SELAR"))
        png(p, f"{L}/png/selar-lockup-{name}.png", int(ww * 4))
        inner, ww, hh = logos.lockup(logos.linny_icon, v, wc, sub=["Semantic Linking", "for Active Retention"], sub_color=sc)
        p = f"{L}/svg/selar-lockup-descriptor-{name}.svg"
        w(p, logos.wrap(inner, ww, hh, "SELAR: Semantic Linking for Active Retention"))
        png(p, f"{L}/png/selar-lockup-descriptor-{name}.png", int(ww * 4))
        inner, ww, hh = logos.lockup(logos.linny_icon, v, wc, stacked=True)
        p = f"{L}/svg/selar-stacked-{name}.svg"
        w(p, logos.wrap(inner, ww, hh, "SELAR"))
        png(p, f"{L}/png/selar-stacked-{name}.png", int(ww * 4))
    for c, name in [(INK, "ink"), ("#FFFFFF", "white"), (PAL["forest"], "forest")]:
        inner, ww, hh = logos.wordmark_only(c)
        p = f"{L}/svg/selar-wordmark-{name}.svg"
        w(p, logos.wrap(inner, ww, hh, "SELAR"))
        png(p, f"{L}/png/selar-wordmark-{name}.png", int(ww * 4))


def build_favicons():
    F = os.path.join(OUT, "favicon")
    os.makedirs(F, exist_ok=True)
    small = logos.wrap(logos.linny_small("color"), 100, 100, "SELAR")
    w(f"{F}/favicon.svg", small)
    w(f"{F}/icon-detailed.svg", logos.wrap(logos.linny_icon("color"), 100, 100, "SELAR"))
    for px in (32, 48):
        png(f"{F}/favicon.svg", f"{F}/favicon-{px}x{px}.png", px)
    w(f"{F}/favicon-16.svg", logos.wrap(logos.linny_tiny(), 100, 100, "SELAR"))
    png(f"{F}/favicon-16.svg", f"{F}/favicon-16x16.png", 16)
    os.remove(f"{F}/favicon-16.svg")
    # larger sizes use the detailed mark on a soft tile
    def tile(size, pad, bg, name, rx=0.22):
        inner = (f'<rect width="100" height="100" rx="{100*rx}" fill="{bg}"/>'
                 f'<g transform="translate({pad + 3} {pad}) scale({(100-2*pad)/100})">{logos.linny_icon("color")}</g>')
        p = f"{F}/{name}.svg"
        w(p, logos.wrap(inner, 100, 100, "SELAR"))
        png(p, f"{F}/{name}.png", size)
        os.remove(p)
    tile(180, 10, PAL["sky"], "apple-touch-icon", rx=0)
    tile(192, 8, PAL["sky"], "icon-192")
    tile(512, 8, PAL["sky"], "icon-512")
    tile(512, 18, PAL["sky"], "icon-maskable-512", rx=0)
    ims = [Image.open(f"{F}/favicon-{px}x{px}.png").convert("RGBA") for px in (16, 32, 48)]
    ims[2].save(f"{F}/favicon.ico", format="ICO", sizes=[(16, 16), (32, 32), (48, 48)], append_images=ims[:2])
    manifest = {
        "name": "SELAR: Semantic Linking for Active Retention",
        "short_name": "SELAR",
        "description": "Research prototype: a PDF reader that suggests links to earlier reading and asks you to explain and decide.",
        "start_url": "/",
        "display": "standalone",
        "background_color": PAL["mist"],
        "theme_color": PAL["forest"],
        "icons": [
            {"src": "/icon-192.png", "sizes": "192x192", "type": "image/png"},
            {"src": "/icon-512.png", "sizes": "512x512", "type": "image/png"},
            {"src": "/icon-maskable-512.png", "sizes": "512x512", "type": "image/png", "purpose": "maskable"},
        ],
    }
    w(f"{F}/site.webmanifest", json.dumps(manifest, indent=2) + "\n")


def build_mascot():
    M = os.path.join(OUT, "mascot")
    for p in mascot.POSES:
        sp = f"{M}/svg/linny-{p}.svg"
        w(sp, mascot.svg(p))
        png(sp, f"{M}/png/linny-{p}-1024.png", 1024)
        png(sp, f"{M}/png/linny-{p}-256.png", 256)


def build_alternatives():
    A = os.path.join(OUT, "exploration")
    for fn, key in [(logos.linny_icon, "a-linny"), (logos.bridge_icon, "b-bridge"), (logos.knot_icon, "c-knot")]:
        for v, name in [("color", "color"), ("mono-dark", "mono-ink"), ("mono-light", "mono-white")]:
            p = f"{A}/{key}/icon-{name}.svg"
            w(p, logos.wrap(fn(v), 100, 100, f"SELAR direction {key}"))
            png(p, f"{A}/{key}/icon-{name}-512.png", 512)
        inner, ww, hh = logos.lockup(fn, "color", INK)
        p = f"{A}/{key}/lockup-light.svg"
        w(p, logos.wrap(inner, ww, hh, "SELAR"))
        png(p, f"{A}/{key}/lockup-light.png", int(ww * 4))
        inner, ww, hh = logos.lockup(fn, "mono-light", "#FFFFFF")
        p = f"{A}/{key}/lockup-on-dark.svg"
        w(p, logos.wrap(inner, ww, hh, "SELAR"))
        png(p, f"{A}/{key}/lockup-on-dark.png", int(ww * 4))


if __name__ == "__main__":
    os.makedirs(OUT, exist_ok=True)
    build_logos(); build_favicons(); build_mascot(); build_alternatives()
    print("built", OUT)
