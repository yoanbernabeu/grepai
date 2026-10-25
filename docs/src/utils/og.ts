import sharp from 'sharp';
import opentype from 'opentype.js';
import { resolve } from 'node:path';

// Build-time Open Graph images (1200x630): the section illustration under a
// parchment veil, the grepai mark, and the page title set in Young Serif.
// Text is converted to SVG paths from the bundled OFL font files, so the
// output does not depend on fonts installed on the build machine.

const WIDTH = 1200;
const HEIGHT = 630;
const LEFT = 72;
const TEXT_WIDTH = 620;
const root = process.cwd();

// opentype.js 1.x ships no types; a font is only used through getPath/getAdvanceWidth here.
type Font = ReturnType<typeof opentype.loadSync>;
let fonts: { serif: Font; sans: Font } | undefined;
function loadFonts() {
  fonts ??= {
    serif: opentype.loadSync(resolve(root, 'src/assets/fonts/YoungSerif-Regular.ttf')),
    sans: opentype.loadSync(resolve(root, 'src/assets/fonts/PublicSans-SemiBold.ttf')),
  };
  return fonts;
}

/** Greedy word wrap measured with the font's real advance widths. */
function wrap(font: Font, text: string, size: number, maxWidth: number): string[] {
  const lines: string[] = [];
  let line = '';
  for (const word of text.split(/\s+/)) {
    const candidate = line ? `${line} ${word}` : word;
    if (line && font.getAdvanceWidth(candidate, size) > maxWidth) {
      lines.push(line);
      line = word;
    } else {
      line = candidate;
    }
  }
  if (line) lines.push(line);
  return lines;
}

function textPaths(font: Font, lines: string[], size: number, top: number, lineHeight: number, fill: string) {
  return lines
    .map((line, i) => {
      const baseline = top + size * 0.8 + i * lineHeight;
      return `<path d="${font.getPath(line, LEFT, baseline, size).toPathData(2)}" fill="${fill}"/>`;
    })
    .join('');
}

export async function renderOgImage(options: { title: string; kicker?: string; art: string }) {
  const { serif, sans } = loadFonts();
  const { title, kicker, art } = options;

  // Largest size that keeps the title within three lines
  let size = 78;
  let lines = wrap(serif, title, size, TEXT_WIDTH);
  while (lines.length > 3 && size > 44) {
    size -= 6;
    lines = wrap(serif, title, size, TEXT_WIDTH);
  }
  const lineHeight = Math.round(size * 1.08);
  const kickerSize = 26;
  const kickerGap = kicker ? kickerSize + 22 : 0;
  const blockHeight = kickerGap + lineHeight * (lines.length - 1) + size;
  const top = Math.max(170, Math.round((HEIGHT - blockHeight) / 2) + 36);

  const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="${WIDTH}" height="${HEIGHT}">
  <defs>
    <linearGradient id="v" x1="0" y1="0" x2="1" y2="0">
      <stop offset="0" stop-color="#F4E8CC" stop-opacity="0.96"/>
      <stop offset="0.48" stop-color="#F4E8CC" stop-opacity="0.88"/>
      <stop offset="0.74" stop-color="#F4E8CC" stop-opacity="0"/>
    </linearGradient>
  </defs>
  <rect width="${WIDTH}" height="${HEIGHT}" fill="url(#v)"/>
  <rect y="${HEIGHT - 8}" width="${WIDTH}" height="8" fill="#E3A52B"/>
  <g transform="translate(${LEFT} 60) scale(1.85)" fill="none" stroke-linecap="round" stroke-linejoin="round">
    <circle cx="10.5" cy="10.5" r="7.5" stroke="#172629" stroke-width="1.7"/>
    <path d="m16 16 5 5" stroke="#172629" stroke-width="2.2"/>
    <path d="M5.6 12.6c1.9 0 1.9-4.4 4.4-4.4s1.6 3.2 3.6 3.2" stroke="#E3A52B" stroke-width="1.7"/>
    <circle cx="14" cy="11.4" r="1.1" fill="#E3A52B"/>
  </g>
  <path d="${serif.getPath('grepai', LEFT + 56, 96, 36).toPathData(2)}" fill="#172629"/>
  ${kicker ? textPaths(sans, [kicker], kickerSize, top, kickerSize, '#6B4A0C') : ''}
  ${textPaths(serif, lines, size, top + kickerGap, lineHeight, '#172629')}
</svg>`;

  return sharp(resolve(root, 'src/assets', art))
    .resize(WIDTH, HEIGHT, { fit: 'cover', position: 'centre' })
    .composite([{ input: Buffer.from(svg), top: 0, left: 0 }])
    .jpeg({ quality: 82, mozjpeg: true })
    .toBuffer();
}
