import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';

export default defineConfig({
  site: 'https://yoanbernabeu.github.io',
  base: '/grepai',
  integrations: [
    starlight({
      title: 'grepai',
      description: 'Privacy-first semantic code search CLI',
      social: {
        github: 'https://github.com/yoanbernabeu/grepai',
      },
      sidebar: [
        {
          label: 'Getting Started',
          items: [
            { label: 'Introduction', link: '/getting-started/' },
            { label: 'Installation', link: '/installation/' },
            { label: 'Quick Start', link: '/quickstart/' },
          ],
        },
        {
          label: 'Features',
          items: [
            { label: 'Semantic Search', link: '/search-guide/' },
            { label: 'File Watching', link: '/watch-guide/' },
            { label: 'Call Graph Analysis', link: '/trace/' },
            { label: 'MCP Integration', link: '/mcp/' },
            { label: 'Claude Code Subagent', link: '/subagent/' },
            { label: 'Search Boost', link: '/search-boost/' },
            { label: 'Hybrid Search', link: '/hybrid-search/' },
          ],
        },
        {
          label: 'Configuration',
          items: [
            { label: 'Config File', link: '/configuration/' },
          ],
        },
        {
          label: 'Commands',
          autogenerate: { directory: 'commands' },
        },
        {
          label: 'Backends',
          items: [
            { label: 'Embedders', link: '/backends/embedders/' },
            { label: 'Stores', link: '/backends/stores/' },
          ],
        },
        {
          label: 'Contributing',
          items: [
            { label: 'How to Contribute', link: '/contributing/' },
          ],
        },
      ],
      editLink: {
        baseUrl: 'https://github.com/yoanbernabeu/grepai/edit/main/docs/',
      },
    }),
  ],
});
