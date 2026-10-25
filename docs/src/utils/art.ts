import { getImage } from 'astro:assets';
import type { ImageMetadata } from 'astro';

/**
 * Builds AVIF, WebP and JPEG srcsets for an illustration, so pages can use
 * art-directed <picture> elements (a different crop on mobile).
 */
export async function responsiveArt(src: ImageMetadata, widths: number[]) {
  const usable = widths.filter((w) => w <= src.width);
  const [avif, webp, jpeg] = await Promise.all(
    (['avif', 'webp', 'jpeg'] as const).map((format) =>
      getImage({ src, widths: usable, format, quality: format === 'avif' ? 60 : 78 })
    )
  );

  return {
    avif: avif.srcSet.attribute,
    webp: webp.srcSet.attribute,
    jpeg: jpeg.srcSet.attribute,
    fallback: jpeg.src,
    width: src.width,
    height: src.height,
  };
}
