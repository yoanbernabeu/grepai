import { defineConfig } from 'astro/config';
import mdx from '@astrojs/mdx';
import sitemap from '@astrojs/sitemap';
import tailwind from '@astrojs/tailwind';
import { execFileSync } from 'node:child_process';
import { existsSync } from 'node:fs';
import cartographe from './src/styles/shiki-cartographe.mjs';

// Sitemap lastmod: the last commit that touched the page's source file.
// Pages without history (generated command docs, shallow clones) get none.
function lastCommitDate(url) {
  const path = new URL(url).pathname.replace(/^\/grepai\/?/, '').replace(/\/$/, '');
  const candidates = path === ''
    ? ['src/pages/index.astro']
    : path.startsWith('blog/') && path !== 'blog'
      ? [`src/content/blog/${path.slice(5)}.md`]
      : [`src/content/docs/${path}.md`, `src/content/docs/${path}.mdx`, `src/pages/${path}/index.astro`, `src/pages/${path}.astro`];
  const file = candidates.find((candidate) => existsSync(candidate));
  if (!file) return undefined;
  try {
    const date = execFileSync('git', ['log', '-1', '--format=%cI', '--', file], { encoding: 'utf8' }).trim();
    return date || undefined;
  } catch {
    return undefined;
  }
}

// Some pages open with an H2 that repeats the page title, which the layout
// already renders as the H1. Drop that first heading when it is a duplicate.
function remarkDropTitleHeading() {
  const text = (node) => (node.value ?? '') + (node.children ?? []).map(text).join('');
  return (tree, file) => {
    const title = file.data?.astro?.frontmatter?.title;
    const first = tree.children.find((node) => node.type !== 'yaml');
    if (title && first?.type === 'heading' && first.depth <= 2 && text(first).trim() === String(title).trim()) {
      const start = tree.children.indexOf(first);
      tree.children.splice(start, 1);
      // Its sub-headings move up one level until the next section, so the
      // outline stays valid (h1, h2, h3...). Ids come from the text, so anchors don't change.
      for (let i = start; i < tree.children.length; i++) {
        const node = tree.children[i];
        if (node.type !== 'heading') continue;
        if (node.depth <= first.depth) break;
        node.depth -= 1;
      }
    }
  };
}

export default defineConfig({
  site: 'https://yoanbernabeu.github.io',
  base: '/grepai',
  integrations: [
    tailwind(),
    mdx(),
    sitemap({
      serialize(item) {
        const lastmod = lastCommitDate(item.url);
        return lastmod ? { ...item, lastmod } : item;
      },
    }),
  ],
  markdown: {
    remarkPlugins: [remarkDropTitleHeading],
    shikiConfig: {
      theme: cartographe,
      wrap: true,
    },
  },
});
