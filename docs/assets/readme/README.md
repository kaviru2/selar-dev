# README illustrations

| File | Shows |
| --- | --- |
| `hero.png` | Wordmark, "Semantic Linking for Active Retention", Linny and a "Research prototype" label. |
| `learning-loop.png` | The current reading workflow: add readings → before-reading warm-up → read and compare → end-reading check → daily review and streak → progress. |
| `architecture.png` | Next.js console → Go API → PostgreSQL/pgvector, with the Python worker → Google Gemini. |

These are **programmatic illustrations** drawn by [`make_readme_art.py`](make_readme_art.py) with Pillow. They are not AI-generated images, not screenshots of the app, and not evidence of learning gains. They reuse the brand kit's Linny mascot and wordmark PNGs (`brand/exports/`) and palette (`brand/DESIGN.md`). Each image has an opaque cream background so it reads in both light and dark GitHub themes.

## Rebuild

```bash
pip install pillow
# Fraunces[SOFT,WONK,opsz,wght].ttf and Figtree[wght].ttf in brand/fonts/,
# or point SELAR_BRAND_FONTS at a folder holding them
python docs/assets/readme/make_readme_art.py
```

The script checks that text fits its cards and that each PNG stays under 400 KB.

## Fonts

Fraunces and Figtree are licensed under the SIL Open Font License 1.1. The licence texts are in [`brand/fonts/`](../../../brand/fonts/). The font files are not vendored in this repository. In the PNGs, the fonts appear only as rendered pixels.
