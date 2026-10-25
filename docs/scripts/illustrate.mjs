#!/usr/bin/env node
// Generates an illustration in the docs art direction ("Le Cartographe") with
// Google's Gemini image model, using the home page hero as the style reference.
// See docs/ILLUSTRATIONS.md for the style guide and prompt tips.
//
// Usage (from ./docs):
//   GEMINI_API_KEY=... npm run illustrate -- <name> "<scene description>" [--aspect 16:9] [--dir blog]
//
// Writes src/assets/<dir>/<name>.jpg (2000px wide). The key is read from the
// environment only; never commit it.

import { readFile, mkdir } from 'node:fs/promises';
import { resolve, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import sharp from 'sharp';

const MODEL = process.env.GEMINI_IMAGE_MODEL || 'gemini-3-pro-image';
const docsRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const reference = resolve(docsRoot, 'src/assets/illustrations/hero.jpg');

// The constant part of every prompt. Keep it in sync with ILLUSTRATIONS.md.
export const STYLE = [
  'Match the reference image exactly in style and world: painterly matte painting, visible brush texture,',
  'the same world of a vast labyrinth city of terraced stone, and when a person appears it is the same lone explorer',
  'in a long dark coat, often with a small lantern.',
  'Palette: deep ink teal and slate shadows, warm parchment and ochre light, one thin molten gold thread or glow',
  'as the only saturated accent.',
  'Wide composition with calm, low-detail areas on the left third for text overlays.',
  'No text, no letters, no numbers, no logos, no UI, no watermark.',
].join(' ');

function parseArgs(argv) {
  const args = { aspect: '16:9', dir: 'blog' };
  const rest = [];
  for (let i = 0; i < argv.length; i++) {
    if (argv[i] === '--aspect') args.aspect = argv[++i];
    else if (argv[i] === '--dir') args.dir = argv[++i];
    else rest.push(argv[i]);
  }
  [args.name, args.scene] = rest;
  return args;
}

async function main() {
  const { name, scene, aspect, dir } = parseArgs(process.argv.slice(2));
  if (!name || !scene || !/^[a-z0-9-]+$/.test(name)) {
    console.error('Usage: npm run illustrate -- <name-in-kebab-case> "<scene description>" [--aspect 16:9] [--dir blog]');
    process.exit(1);
  }
  const key = process.env.GEMINI_API_KEY;
  if (!key) {
    console.error('GEMINI_API_KEY is not set.');
    process.exit(1);
  }

  const refImage = await sharp(await readFile(reference)).resize(1600).png().toBuffer();
  const res = await fetch(`https://generativelanguage.googleapis.com/v1beta/models/${MODEL}:generateContent`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'x-goog-api-key': key },
    body: JSON.stringify({
      contents: [{
        parts: [
          { inlineData: { mimeType: 'image/png', data: refImage.toString('base64') } },
          { text: `${scene.trim()} ${STYLE}` },
        ],
      }],
      generationConfig: { responseModalities: ['IMAGE'], imageConfig: { aspectRatio: aspect, imageSize: '2K' } },
    }),
  });
  if (!res.ok) {
    console.error(`Gemini API error ${res.status}: ${(await res.text()).slice(0, 500)}`);
    process.exit(1);
  }

  const body = await res.json();
  const part = body.candidates?.flatMap((c) => c.content?.parts ?? []).find((p) => p.inlineData);
  if (!part) {
    console.error('No image in the response:', JSON.stringify(body).slice(0, 500));
    process.exit(1);
  }

  const out = resolve(docsRoot, 'src/assets', dir, `${name}.jpg`);
  await mkdir(dirname(out), { recursive: true });
  await sharp(Buffer.from(part.inlineData.data, 'base64'))
    .resize({ width: 2000, withoutEnlargement: true })
    .jpeg({ quality: 84, mozjpeg: true })
    .toFile(out);
  console.log(`Wrote ${out}`);
}

main();
