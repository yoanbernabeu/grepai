import type { ImageMetadata } from 'astro';

// Blog covers follow a naming convention: src/assets/blog/<post-slug>.jpg.
// Generate one with `npm run illustrate -- <post-slug> "<scene>"` (see docs/ILLUSTRATIONS.md).
const covers = import.meta.glob<{ default: ImageMetadata }>('../assets/blog/*.jpg', { eager: true });

export function blogCover(slug: string): { image: ImageMetadata; file: string } | undefined {
  const entry = Object.entries(covers).find(([path]) => path.endsWith(`/${slug}.jpg`));
  return entry ? { image: entry[1].default, file: `blog/${slug}.jpg` } : undefined;
}
