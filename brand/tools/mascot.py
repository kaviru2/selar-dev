"""Linny the linnet: SELAR's mascot, drawn as plain SVG primitives (240x240 frame).

Poses: hello, reading, linking, thinking, celebrating, questioning, empty.
All shapes are original geometry (ellipses, paths); no external artwork.
"""
import math
from common import PAL

CREAM = "#FFF6E6"
CHEEK = "#EE9A78"
WING = PAL["moss"]
BODY = PAL["forest"]
BEAK = PAL["amber"]
THREAD = PAL["rust"]
INK = PAL["ink"]
SW = 3.5


def page(x, y, w=46, h=58, rot=0, fill="#FFFFFF", lines=3, dot=None, dashed=False):
    f = 12
    dash = ' stroke-dasharray="6 6"' if dashed else ""
    op = ' opacity=".75"' if dashed else ""
    s = [f'<g transform="translate({x} {y}) rotate({rot})"{op}>',
         f'<path d="M{-w/2} {-h/2} H{w/2-f} L{w/2} {-h/2+f} V{h/2} H{-w/2} Z" fill="{fill}" stroke="{INK}" stroke-width="3" stroke-linejoin="round"{dash}/>',
         f'<path d="M{w/2-f} {-h/2} V{-h/2+f} H{w/2}" fill="none" stroke="{INK}" stroke-width="3" stroke-linejoin="round"/>']
    for i in range(lines):
        yy = -h/2 + 18 + i * 10
        ln = w - 20 - (8 if i == lines - 1 else 0)
        s.append(f'<line x1="{-w/2+9}" y1="{yy}" x2="{-w/2+9+ln}" y2="{yy}" stroke="{PAL["slate"]}" stroke-width="3" stroke-linecap="round" opacity=".55"/>')
    if dot:
        s.append(f'<circle cx="{dot[0]}" cy="{dot[1]}" r="6" fill="{THREAD}" stroke="{INK}" stroke-width="2.5"/>')
    s.append('</g>')
    return "".join(s)


def eyes(kind="open", look=(0, 0)):
    lx, ly = look
    if kind == "open":
        out = []
        for cx in (101, 139):
            out.append(f'<circle cx="{cx+lx}" cy="{116+ly}" r="9.5" fill="{INK}"/>')
            out.append(f'<circle cx="{cx+lx+3}" cy="{112+ly}" r="3.2" fill="#FFFFFF"/>')
        return "".join(out)
    if kind == "happy":
        return "".join(f'<path d="M{cx-9} 120 Q{cx} 106 {cx+9} 120" fill="none" stroke="{INK}" stroke-width="5" stroke-linecap="round"/>' for cx in (101, 139))
    if kind == "closed":
        return "".join(f'<path d="M{cx-9} 116 Q{cx} 124 {cx+9} 116" fill="none" stroke="{INK}" stroke-width="4.5" stroke-linecap="round"/>' for cx in (101, 139))
    if kind == "squint":
        return (f'<circle cx="{101+lx}" cy="{117+ly}" r="9.5" fill="{INK}"/><circle cx="{104+lx}" cy="{113+ly}" r="3.2" fill="#fff"/>'
                f'<circle cx="{139+lx}" cy="{117+ly}" r="9.5" fill="{INK}"/><circle cx="{142+lx}" cy="{113+ly}" r="3.2" fill="#fff"/>'
                f'<path d="M129 99 Q140 91 151 97" fill="none" stroke="{INK}" stroke-width="4.5" stroke-linecap="round"/>')
    raise ValueError(kind)


def beak(open_=False):
    if open_:
        return (f'<path d="M110 131 L130 131 L120 140 Z" fill="{BEAK}" stroke="{BEAK}" stroke-width="4" stroke-linejoin="round"/>'
                f'<path d="M114 142 L126 142 L120 148 Z" fill="#B45309" stroke="#B45309" stroke-width="3" stroke-linejoin="round"/>')
    return f'<path d="M110 131 L130 131 L120 143 Z" fill="{BEAK}" stroke="{BEAK}" stroke-width="4" stroke-linejoin="round"/>'


def _wing_th(side, angle):
    return -side * (18 + angle)


def wing(side, angle=0):
    """side -1 left, +1 right. angle > 0 lifts the wing outward/up."""
    cx = 120 + side * 64
    return (f'<g transform="rotate({_wing_th(side, angle)} {120 + side*52} 128)">'
            f'<ellipse cx="{cx}" cy="152" rx="17" ry="30" fill="{WING}" stroke="{INK}" stroke-width="{SW}"/></g>')


def wing_tip(side, angle=0):
    th = math.radians(_wing_th(side, angle))
    px, py = 120 + side * 52, 128
    dx, dy = side * 12, 52
    return (px + dx * math.cos(th) - dy * math.sin(th), py + dx * math.sin(th) + dy * math.cos(th))


def body(eye_kind="open", look=(0, 0), beak_open=False, wings=(0, 0), cheeks=True, tilt=0, front_wings=False):
    s = [f'<g transform="rotate({tilt} 120 150)">']
    s.append(f'<path d="M106 196 v12 M100 210 h12 M134 196 v12 M128 210 h12" stroke="{BEAK}" stroke-width="5" stroke-linecap="round"/>')
    if not front_wings:
        for sd, a in ((-1, wings[0]), (1, wings[1])):
            if a != -200:
                s.append(wing(sd, a))
    s.append(f'<path d="M116 70 C106 50 118 38 130 44 C121 49 121 58 127 68 Z" fill="{BODY}" stroke="{INK}" stroke-width="{SW}" stroke-linejoin="round"/>')
    s.append(f'<ellipse cx="120" cy="134" rx="68" ry="66" fill="{BODY}" stroke="{INK}" stroke-width="{SW}"/>')
    s.append(f'<path d="M80 160 C82 132 158 132 160 160 C158 188 82 188 80 160 Z" fill="{CREAM}"/>')
    if cheeks:
        s.append(f'<circle cx="86" cy="134" r="8.5" fill="{CHEEK}" opacity=".75"/><circle cx="154" cy="134" r="8.5" fill="{CHEEK}" opacity=".75"/>')
    s.append(eyes(eye_kind, look))
    s.append(beak(beak_open))
    s.append('</g>')
    return "".join(s)


def scaled(inner, k=0.8, cx=120, cy=134, dx=0, dy=0):
    return f'<g transform="translate({cx+dx} {cy+dy}) scale({k}) translate({-cx} {-cy})">{inner}</g>'


def tx(pt, k=0.8, cx=120, cy=134, dx=0, dy=0):
    return (cx + dx + (pt[0] - cx) * k, cy + dy + (pt[1] - cy) * k)


def thread(d, w=5, dash=None):
    da = f' stroke-dasharray="{dash}"' if dash else ""
    return f'<path d="{d}" fill="none" stroke="{THREAD}" stroke-width="{w}" stroke-linecap="round" stroke-linejoin="round"{da}/>'


def spark(x, y, r, c):
    return (f'<path d="M{x} {y-r} L{x+r*0.3} {y-r*0.3} L{x+r} {y} L{x+r*0.3} {y+r*0.3} L{x} {y+r} '
            f'L{x-r*0.3} {y+r*0.3} L{x-r} {y} L{x-r*0.3} {y-r*0.3} Z" fill="{c}"/>')


K = 0.8
DY = 10


def B(**kw):
    return scaled(body(**kw), K, dy=DY)


def T(pt):
    return tx(pt, K, dy=DY)


def motion(x, y, side=1):
    return (f'<path d="M{x} {y} q{side*8} -8 {side*4} -18 M{x+side*12} {y+4} q{side*10} -10 {side*6} -24" fill="none" '
            f'stroke="{PAL["moss"]}" stroke-width="3.5" stroke-linecap="round"/>')


def raised_wing(side, a=40, length=64):
    """A wing raised outward/upward from the side of the body. a = degrees from vertical."""
    px, py = 120 + side * 58, 142
    r = math.radians(a)
    dx, dy = side * math.sin(r), -math.cos(r)
    cx, cy = px + dx * length * 0.5, py + dy * length * 0.5
    return (f'<ellipse cx="{cx:.1f}" cy="{cy:.1f}" rx="16" ry="{length/2+6:.1f}" transform="rotate({side*a} {cx:.1f} {cy:.1f})" '
            f'fill="{WING}" stroke="{INK}" stroke-width="{SW}"/>'), (px + dx * (length + 4), py + dy * (length + 4))


def B(inner_pre="", inner_post="", **kw):
    return scaled(inner_pre + body(**kw) + inner_post, K, dy=DY)


def T(pt):
    return tx(pt, K, dy=DY)


def pose(name):
    if name == "hello":
        w, tip = raised_wing(1, 35)
        tip = T(tip)
        return B(w, wings=(0, -200)) + motion(tip[0] + 6, tip[1] + 10)
    if name == "reading":
        book = (f'<path d="M120 170 C104 160 84 160 70 166 V204 C84 198 104 198 120 208 Z" fill="#FFFFFF" stroke="{INK}" stroke-width="{SW}" stroke-linejoin="round"/>'
                f'<path d="M120 170 C136 160 156 160 170 166 V204 C156 198 136 198 120 208 Z" fill="{PAL["sky"]}" stroke="{INK}" stroke-width="{SW}" stroke-linejoin="round"/>'
                f'<path d="M80 178 q16 -4 30 1 M80 188 q16 -4 30 1 M130 179 q14 -5 30 -1 M130 189 q14 -5 30 -1" stroke="{PAL["slate"]}" stroke-width="2.5" fill="none" stroke-linecap="round" opacity=".6"/>'
                f'<ellipse cx="70" cy="186" rx="11" ry="16" fill="{WING}" stroke="{INK}" stroke-width="{SW}"/>'
                f'<ellipse cx="170" cy="186" rx="11" ry="16" fill="{WING}" stroke="{INK}" stroke-width="{SW}"/>')
        return B("", book, look=(-2, 9), front_wings=True)
    if name in ("linking", "questioning"):
        wl, tl = raised_wing(-1, 70, 56)
        wr, tr = raised_wing(1, 70, 56)
        tl, tr = T(tl), T(tr)
        lp, rp = (42, 198), (198, 198)
        left = page(*lp, rot=-6, dot=(0, -19), lines=2)
        lt = thread(f"M{lp[0]} {lp[1]-19} C{lp[0]-8} {lp[1]-60} {tl[0]-22} {tl[1]-10} {tl[0]} {tl[1]}")
        if name == "linking":
            right = page(*rp, rot=6, dot=(0, -19), lines=2)
            rt = thread(f"M{tr[0]} {tr[1]} C{tr[0]+22} {tr[1]-10} {rp[0]+8} {rp[1]-60} {rp[0]} {rp[1]-19}")
            return left + right + lt + rt + B(wl + wr, wings=(-200, -200), look=(0, 4))
        right = page(rp[0] + 4, rp[1] + 4, w=40, h=50, rot=8, dashed=True, lines=0)
        rt = thread(f"M{tr[0]} {tr[1]} C{tr[0]+8} {tr[1]-6} {tr[0]+12} {tr[1]+8} {tr[0]+8} {tr[1]+20}", w=4.5)
        e = (tr[0] + 8, tr[1] + 20)
        fray = f'<path d="M{e[0]} {e[1]} l-6 6 M{e[0]} {e[1]} l1 8 M{e[0]} {e[1]} l6 5" stroke="{THREAD}" stroke-width="2.5" stroke-linecap="round"/>'
        q = (f'<path d="M184 28 C184 12 210 12 210 28 C210 40 197 40 197 52" fill="none" stroke="{PAL["amber"]}" stroke-width="7" stroke-linecap="round"/>'
             f'<circle cx="197" cy="67" r="5" fill="{PAL["amber"]}"/>')
        return left + right + lt + rt + fray + B(wl + wr, wings=(-200, -200), eye_kind="squint") + q
    if name == "thinking":
        bub = (f'<circle cx="168" cy="72" r="5" fill="{PAL["moss"]}"/><circle cx="182" cy="56" r="7" fill="{PAL["moss"]}"/>'
               f'<circle cx="204" cy="34" r="18" fill="#FFFFFF" stroke="{INK}" stroke-width="3"/>'
               f'<path d="M196 34 h16" stroke="{THREAD}" stroke-width="4" stroke-linecap="round"/>'
               f'<circle cx="195" cy="34" r="4.5" fill="{PAL["forest"]}"/><circle cx="213" cy="34" r="4.5" fill="{PAL["forest"]}"/>')
        chin = f'<ellipse cx="140" cy="156" rx="11" ry="20" transform="rotate(-55 140 156)" fill="{WING}" stroke="{INK}" stroke-width="{SW}"/>'
        return B("", chin, look=(6, -7), wings=(0, -200)) + bub
    if name == "celebrating":
        wl, _ = raised_wing(-1, 38)
        wr, _ = raised_wing(1, 38)
        sp = "".join(spark(*a) for a in [(34, 70, 12, PAL["sun"]), (206, 66, 14, PAL["sun"]), (120, 22, 9, PAL["sun"]), (24, 150, 7, PAL["sun"]), (216, 140, 8, PAL["sun"])])
        badge = (f'<circle cx="196" cy="200" r="19" fill="{PAL["forest"]}" stroke="{INK}" stroke-width="{SW}"/>'
                 f'<path d="M187 200 l6 6 l12 -13" fill="none" stroke="#FFFFFF" stroke-width="5" stroke-linecap="round" stroke-linejoin="round"/>')
        return sp + B(wl + wr, eye_kind="happy", wings=(-200, -200), beak_open=True) + badge
    if name == "empty":
        pg = page(200, 198, w=40, h=50, rot=8, lines=0, fill="#FFFFFF", dashed=True)
        z = (f'<path d="M152 72 h10 l-10 11 h10" fill="none" stroke="{PAL["slate"]}" stroke-width="3.5" stroke-linecap="round" stroke-linejoin="round"/>'
             f'<path d="M170 44 h15 l-15 17 h15" fill="none" stroke="{PAL["slate"]}" stroke-width="4" stroke-linecap="round" stroke-linejoin="round"/>')
        return scaled(body(eye_kind="closed"), K, dx=-14, dy=DY) + pg + z
    raise ValueError(name)


POSES = ["hello", "reading", "linking", "thinking", "celebrating", "questioning", "empty"]
POSE_LABEL = {
    "hello": "Hello / onboarding",
    "reading": "Reading",
    "linking": "Linking two passages",
    "thinking": "Thinking / loading",
    "celebrating": "Kept a link",
    "questioning": "Questioning a weak link",
    "empty": "Nothing yet (empty state)",
}


def svg(name, size=240, bg=None, title=None):
    t = title or f"Linny the SELAR linnet: {POSE_LABEL[name].lower()}"
    bgr = f'<rect width="240" height="240" rx="28" fill="{bg}"/>' if bg else ""
    return (f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 240 240" width="{size}" height="{size}" role="img" aria-label="{t}">'
            f'<title>{t}</title>{bgr}{pose(name)}</svg>')
