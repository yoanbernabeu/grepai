import type { APIRoute } from 'astro';
import { intro, docsInNavOrder, mdUrl } from '../utils/llms';

// Every docs page concatenated in navigation order, for tools that want one file.
export const GET: APIRoute = async () => {
  const sections = await docsInNavOrder();
  const parts = [intro()];
  for (const section of sections) {
    for (const doc of section.entries) {
      parts.push(
        `\n---\n\n# ${doc.data.title}\n\nSource: ${mdUrl(doc)}\n${doc.data.description ? `\n> ${doc.data.description}\n` : ''}\n${(doc.body ?? '').trim()}\n`
      );
    }
  }
  return new Response(parts.join(''), { headers: { 'Content-Type': 'text/plain; charset=utf-8' } });
};
