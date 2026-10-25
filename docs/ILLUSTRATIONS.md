# Illustrations

All illustrations on the site share one world, "Le Cartographe": a lone explorer finding a path through a vast labyrinth city that stands for a codebase. The gold thread is the path from a question to the code that answers it, and it is the only saturated color.

## Generate a new one

```bash
cd docs
GEMINI_API_KEY=your-key npm run illustrate -- <name> "<scene>"
```

- `<name>`: file name in kebab-case. The image is written to `src/assets/blog/<name>.jpg` (2000 px wide).
- `--aspect 16:9` (default) or `21:9`, `4:5`, `1:1`.
- `--dir illustrations` writes to `src/assets/illustrations/` instead of `src/assets/blog/`.
- `GEMINI_IMAGE_MODEL` overrides the model (default `gemini-3-pro-image`).

The script sends the home page hero (`src/assets/illustrations/hero.jpg`) as a style reference and appends the shared style prompt (`STYLE` in `scripts/illustrate.mjs`), so you only describe the scene. Keep the key in your environment; never commit it.

## Blog covers

A post gets a cover when `src/assets/blog/<post-slug>.jpg` exists, where `<post-slug>` is the Markdown file name without `.md`. Nothing to add to the frontmatter. The cover is used:

- at the top of the post,
- as the thumbnail on `/blog/`,
- as the background of the post's Open Graph image (`/og/blog/<post-slug>.jpg`).

Example for a new post `src/content/blog/faster-indexing.md`:

```bash
npm run illustrate -- faster-indexing "The explorer rides a small cable car across the labyrinth city, a golden thread unspooling behind it."
```

Then look at the result and regenerate if needed: the model sometimes leaves an empty band on one side, adds text, or changes the explorer. Asking for "a continuous full-bleed landscape, painted edge to edge" fixes the empty band.

## Writing a scene

- Describe one moment that maps to the post's idea (two routes for a benchmark, several plateaus linked by bridges for workspaces).
- Keep the cast: the explorer in a long dark coat, a small lantern, the labyrinth city, sometimes small floating paper lantern spirits as helpers (used for AI agents).
- Light: dawn or dusk, warm sky, ink-teal shadows. One gold thread or glow.
- Leave the left third calm if the image will carry text (Open Graph images put the title there).
- Avoid text, numbers, logos, screens and UI in the image; the style prompt already asks for this.

## Where the existing images are used

| File | Used on |
| --- | --- |
| `illustrations/hero.jpg`, `hero-mobile.jpg` | Home page hero (desktop, mobile crop) |
| `illustrations/closing.jpg` | Home page closing section, fallback blog OG background |
| `illustrations/lost.jpg` | 404 page |
| `illustrations/section-*.jpg` | Banner on guide pages and OG background, per sidebar section (see `src/utils/sections.ts`) |
| `blog/<post-slug>.jpg` | Blog covers |

`public/og-image.png` is the Open Graph image of the home page and blog index.
