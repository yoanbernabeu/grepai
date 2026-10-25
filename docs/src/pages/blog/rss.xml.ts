import type { APIRoute } from 'astro';
import { getCollection } from 'astro:content';

const escape = (text: string) =>
  text.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');

export const GET: APIRoute = async ({ site }) => {
  const posts = (await getCollection('blog'))
    .filter((post) => !post.data.draft)
    .sort((a, b) => b.data.pubDate.valueOf() - a.data.pubDate.valueOf());
  const base = new URL('/grepai/', site).toString();

  const items = posts
    .map((post) => {
      const url = `${base}blog/${post.id.replace(/\.(md|mdx)$/, '')}/`;
      return `    <item>
      <title>${escape(post.data.title)}</title>
      <link>${url}</link>
      <guid>${url}</guid>
      <description>${escape(post.data.description)}</description>
      <pubDate>${post.data.pubDate.toUTCString()}</pubDate>
    </item>`;
    })
    .join('\n');

  const xml = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:atom="http://www.w3.org/2005/Atom">
  <channel>
    <title>grepai blog</title>
    <link>${base}blog/</link>
    <description>Releases, guides and benchmarks for grepai, semantic code search for AI agents.</description>
    <language>en</language>
    <atom:link href="${base}blog/rss.xml" rel="self" type="application/rss+xml" />
${items}
  </channel>
</rss>
`;
  return new Response(xml, { headers: { 'Content-Type': 'application/rss+xml; charset=utf-8' } });
};
