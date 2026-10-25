import type { APIRoute, GetStaticPaths } from 'astro';
import { getCollection } from 'astro:content';

// Markdown version of every docs page (/grepai/<slug>.md), for LLMs and tools
// that read raw text. Linked from llms.txt and from each page's <head>.
export const getStaticPaths: GetStaticPaths = async () => {
  const docs = await getCollection('docs');
  return docs.map((entry) => ({
    params: { slug: entry.id.replace(/\.(md|mdx)$/, '') },
    props: { title: entry.data.title, description: entry.data.description, body: entry.body ?? '' },
  }));
};

export const GET: APIRoute = ({ props }) => {
  const { title, description, body } = props as { title: string; description?: string; body: string };
  const text = `# ${title}\n\n${description ? `> ${description}\n\n` : ''}${body.trim()}\n`;
  return new Response(text, { headers: { 'Content-Type': 'text/markdown; charset=utf-8' } });
};
