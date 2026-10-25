import type { APIRoute } from 'astro';
import { getCollection } from 'astro:content';
import { intro, docsInNavOrder, mdUrl, blogMdUrl } from '../utils/llms';

// llms.txt (https://llmstxt.org): what grepai is, then every page as Markdown.
export const GET: APIRoute = async () => {
  const sections = await docsInNavOrder();
  const posts = (await getCollection('blog'))
    .filter((post) => !post.data.draft)
    .sort((a, b) => b.data.pubDate.valueOf() - a.data.pubDate.valueOf());

  const lines = [intro()];
  for (const section of sections) {
    if (section.entries.length === 0) continue;
    lines.push(`## ${section.title}\n`);
    for (const doc of section.entries) {
      const desc = doc.data.description ? `: ${doc.data.description}` : '';
      lines.push(`- [${doc.data.title}](${mdUrl(doc)})${desc}`);
    }
    lines.push('');
  }
  lines.push('## Optional\n');
  for (const post of posts) {
    lines.push(`- [${post.data.title}](${blogMdUrl(post)}): ${post.data.description}`);
  }
  lines.push('- [Full documentation in one file](https://yoanbernabeu.github.io/grepai/llms-full.txt)');

  return new Response(lines.join('\n') + '\n', { headers: { 'Content-Type': 'text/plain; charset=utf-8' } });
};
