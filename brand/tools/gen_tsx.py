"""Generate React (TSX) brand components from the Python SVG sources."""
import os, re, sys
sys.path.insert(0, os.path.dirname(__file__))
import logos, mascot
from common import PAL

ATTR = {"stroke-width": "strokeWidth", "stroke-linecap": "strokeLinecap", "stroke-linejoin": "strokeLinejoin",
        "stroke-dasharray": "strokeDasharray", "fill-rule": "fillRule", "clip-rule": "clipRule"}


def jsx(svg_inner):
    s = svg_inner
    for k, v in ATTR.items():
        s = s.replace(f'{k}=', f'{v}=')
    s = re.sub(r'<title>.*?</title>', '', s)
    s = s.replace('<text x="0" y="0" style="display:none"/>', '')
    # self-close is already used everywhere; ensure no stray 'style=' strings
    assert 'style=' not in s, s[:200]
    return s


def build(repo_console):
    out_dir = os.path.join(repo_console, "components", "brand")
    os.makedirs(out_dir, exist_ok=True)
    marks = {v: jsx(logos.linny_icon(v)) for v in ["color", "dark", "mono-dark", "mono-light"]}
    small = jsx(logos.linny_small("color"))
    wm_inner, wm_w, wm_h = logos.wordmark_only("currentColor")
    word_d, word_w = logos.text_path("selar", logos.WORD_KEY, logos.WORD_SIZE, 0, 0, -0.005)
    wm_tr = re.search(r'transform="([^"]+)"', wm_inner).group(1)
    xh, cap = logos.word_metrics()
    lines = []
    lines.append('// Logo.tsx: SELAR brand marks (generated from brand/tools; edit the sources, not this file).')
    lines.append('// Linny the linnet holding a page is the primary mark. The wordmark is Fraunces (OFL) outlined to paths.')
    lines.append('// Usage: <Logo /> for the horizontal lockup, <LogoMark /> for the icon, <Wordmark /> for text only.')
    lines.append('')
    lines.append('import type { SVGProps } from "react";')
    lines.append('')
    lines.append('export type LogoVariant = "color" | "dark" | "mono" | "mono-white";')
    lines.append('')
    lines.append('const MARKS: Record<LogoVariant, React.ReactNode> = {')
    for key, v in [("color", "color"), ("dark", "dark"), ("mono", "mono-dark"), ("mono-white", "mono-light")]:
        lines.append(f'  "{key}": (\n    <>\n      {marks[v]}\n    </>\n  ),')
    lines.append('};')
    lines.append('')
    lines.append('type MarkProps = Omit<SVGProps<SVGSVGElement>, "children"> & {')
    lines.append('  /** Rendered height in px (width follows the aspect ratio). */')
    lines.append('  size?: number;')
    lines.append('  variant?: LogoVariant;')
    lines.append('  /** Accessible name. Pass an empty string when the mark sits next to visible "SELAR" text. */')
    lines.append('  title?: string;')
    lines.append('};')
    lines.append('')
    lines.append('function a11y(title: string | undefined) {')
    lines.append('  return title === "" ? { "aria-hidden": true as const } : { role: "img" as const, "aria-label": title ?? "SELAR" };')
    lines.append('}')
    lines.append('')
    lines.append('/** Icon-only mark (Linny holding a page). Minimum size: 24 px; below that use the favicon. */')
    lines.append('export function LogoMark({ size = 32, variant = "color", title, ...rest }: MarkProps) {')
    lines.append('  return (')
    lines.append('    <svg viewBox="0 0 100 100" width={size} height={size} {...a11y(title)} {...rest}>')
    lines.append('      {MARKS[variant]}')
    lines.append('    </svg>')
    lines.append('  );')
    lines.append('}')
    lines.append('')
    lines.append('const WORDMARK_PATH = "' + word_d + '";')
    lines.append('const WORDMARK_TRANSFORM = "' + wm_tr + '";')
    lines.append(f'const WORDMARK_W = {wm_w:.1f};')
    lines.append(f'const WORDMARK_H = {wm_h:.1f};')
    lines.append('')
    lines.append('/** Text-only wordmark. Inherits colour from CSS `color` unless `color` is passed. */')
    lines.append('export function Wordmark({ size = 24, title, color = "currentColor", ...rest }: Omit<MarkProps, "variant"> & { color?: string }) {')
    lines.append('  return (')
    lines.append('    <svg viewBox={`0 0 ${WORDMARK_W} ${WORDMARK_H}`} height={size} width={(size * WORDMARK_W) / WORDMARK_H} {...a11y(title)} {...rest}>')
    lines.append('      <path transform={WORDMARK_TRANSFORM} d={WORDMARK_PATH} fill={color} />')
    lines.append('    </svg>')
    lines.append('  );')
    lines.append('}')
    lines.append('')
    # lockup: icon 100 + gap 22 + word at baseline
    base = 54 + xh / 2
    lw = 100 + 22 + word_w + 6
    lines.append(f'const LOCKUP_W = {lw:.1f};')
    lines.append(f'const LOCKUP_BASELINE = {base:.1f};')
    lines.append('')
    lines.append('/** Horizontal lockup: mark + wordmark. `size` is the overall height in px. */')
    lines.append('export function Logo({ size = 32, variant = "color", title, ...rest }: MarkProps) {')
    lines.append('  const word = variant === "dark" || variant === "mono-white" ? "#FFFFFF" : "#0F172A";')
    lines.append('  return (')
    lines.append('    <svg viewBox={`0 0 ${LOCKUP_W} 100`} height={size} width={(size * LOCKUP_W) / 100} {...a11y(title)} {...rest}>')
    lines.append('      {MARKS[variant]}')
    lines.append('      <path transform={`translate(122 ${LOCKUP_BASELINE})`} d={WORDMARK_PATH} fill={word} />')
    lines.append('    </svg>')
    lines.append('  );')
    lines.append('}')
    lines.append('')
    content = "\n".join(lines) + "\n"
    open(os.path.join(out_dir, "Logo.tsx"), "w").write(content)

    # Linny mascot
    L = []
    L.append('// Linny.tsx: SELAR mascot poses (generated from brand/tools; edit the sources, not this file).')
    L.append('// Original artwork. Use for onboarding, empty, loading and feedback states; one Linny per view.')
    L.append('')
    L.append('import type { SVGProps } from "react";')
    L.append('')
    L.append('export type LinnyPose = ' + " | ".join(f'"{p}"' for p in mascot.POSES) + ';')
    L.append('')
    L.append('export const LINNY_LABELS: Record<LinnyPose, string> = {')
    for p in mascot.POSES:
        L.append(f'  {p}: "Linny the linnet: {mascot.POSE_LABEL[p].lower()}",')
    L.append('};')
    L.append('')
    L.append('const POSES: Record<LinnyPose, React.ReactNode> = {')
    for p in mascot.POSES:
        L.append(f'  {p}: (\n    <>\n      {jsx(mascot.pose(p))}\n    </>\n  ),')
    L.append('};')
    L.append('')
    L.append('type LinnyProps = Omit<SVGProps<SVGSVGElement>, "children"> & {')
    L.append('  pose?: LinnyPose;')
    L.append('  size?: number;')
    L.append('  /** Accessible name; pass "" when decorative (e.g. next to a heading that says the same thing). */')
    L.append('  title?: string;')
    L.append('};')
    L.append('')
    L.append('export function Linny({ pose = "hello", size = 160, title, ...rest }: LinnyProps) {')
    L.append('  const label = title ?? LINNY_LABELS[pose];')
    L.append('  const a11y = label === "" ? { "aria-hidden": true as const } : { role: "img" as const, "aria-label": label };')
    L.append('  return (')
    L.append('    <svg viewBox="0 0 240 240" width={size} height={size} data-pose={pose} {...a11y} {...rest}>')
    L.append('      {POSES[pose]}')
    L.append('    </svg>')
    L.append('  );')
    L.append('}')
    L.append('')
    open(os.path.join(out_dir, "Linny.tsx"), "w").write("\n".join(L))
    open(os.path.join(out_dir, "index.ts"), "w").write(
        '// Brand components: SELAR logo and the Linny mascot. See brand/DESIGN.md for usage rules.\n'
        'export { Logo, LogoMark, Wordmark } from "./Logo";\nexport type { LogoVariant } from "./Logo";\n'
        'export { Linny, LINNY_LABELS } from "./Linny";\nexport type { LinnyPose } from "./Linny";\n')
    print("wrote", out_dir)


if __name__ == "__main__":
    build(sys.argv[1])
