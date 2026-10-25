import type { APIRoute, GetStaticPaths } from 'astro';
import { getCollection } from 'astro:content';
import { findCurrentSection } from '../../utils/navigation';
import { sectionArt } from '../../utils/sections';
import { renderOgImage } from '../../utils/og';
import { blogCover } from '../../utils/blogCovers';

// One Open Graph image per docs page and blog post, rendered at build time.
export const getStaticPaths: GetStaticPaths = async () => {
  const docs = (await getCollection('docs')).map((entry) => {
    const slug = entry.id.replace(/\.(md|mdx)$/, '');
    const section = findCurrentSection(entry.id);
    const isCommand = slug.startsWith('commands/');
    return {
      params: { slug },
      props: {
        title: entry.data.title,
        kicker: isCommand ? 'Command reference' : section ?? 'Documentation',
        art: (isCommand ? sectionArt('Configuration') : sectionArt(section) ?? sectionArt('Getting Started'))!.file,
      },
    };
  });

  const posts = (await getCollection('blog'))
    .filter((post) => !post.data.draft)
    .map((post) => {
      const slug = post.id.replace(/\.(md|mdx)$/, '');
      return {
        params: { slug: `blog/${slug}` },
        props: { title: post.data.title, kicker: 'grepai blog', art: blogCover(slug)?.file ?? 'illustrations/closing.jpg' },
      };
    });

  return [...docs, ...posts];
};

export const GET: APIRoute = async ({ props }) => {
  const jpg = await renderOgImage(props as { title: string; kicker: string; art: string });
  return new Response(jpg, { headers: { 'Content-Type': 'image/jpeg' } });
};
