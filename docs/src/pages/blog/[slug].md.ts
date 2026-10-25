import type { APIRoute, GetStaticPaths } from 'astro';
import { getCollection } from 'astro:content';

// Markdown version of every blog post (/grepai/blog/<slug>.md).
export const getStaticPaths: GetStaticPaths = async () => {
  const posts = (await getCollection('blog')).filter((post) => !post.data.draft);
  return posts.map((post) => ({
    params: { slug: post.id.replace(/\.(md|mdx)$/, '') },
    props: { data: post.data, body: post.body ?? '' },
  }));
};

export const GET: APIRoute = ({ props }) => {
  const { data, body } = props as { data: { title: string; description: string; pubDate: Date; author: string }; body: string };
  const date = data.pubDate.toISOString().slice(0, 10);
  const text = `# ${data.title}\n\n> ${data.description}\n\nPublished ${date} by ${data.author}.\n\n${body.trim()}\n`;
  return new Response(text, { headers: { 'Content-Type': 'text/markdown; charset=utf-8' } });
};
