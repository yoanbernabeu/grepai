import { getCollection, type CollectionEntry } from 'astro:content';
import { navigation } from './navigation';
import { latestVersion } from './project';

const SITE = 'https://yoanbernabeu.github.io/grepai';

export const intro = () => {
  const version = latestVersion();
  return `# grepai

> grepai is an open-source (MIT) command-line tool for semantic code search and call graph tracing. It indexes what code means with vector embeddings, so developers and AI coding agents (Claude Code, Cursor, Codex, Windsurf, Gemini CLI, OpenCode) can find code by describing it in natural language, then see who calls it.

- Single Go binary for macOS, Linux and Windows${version ? `, latest release v${version}` : ''}.
- Embeddings run locally with Ollama (default model nomic-embed-text) or LM Studio, or through OpenAI, OpenRouter, Synthetic and Requesty.
- Index storage: local GOB file (default), PostgreSQL with pgvector, or Qdrant.
- Main commands: \`grepai init\`, \`grepai watch\` (live index), \`grepai index\` (one-shot), \`grepai search\`, \`grepai trace callers|callees|graph\`, \`grepai refs readers|writers|graph\`, \`grepai rpg search|fetch|explore\`, \`grepai stats\`, \`grepai mcp-serve\` (MCP server), \`grepai workspace\` (multi-project).
- Source code: https://github.com/yoanbernabeu/grepai
`;
};

type Doc = CollectionEntry<'docs'>;

export async function docsInNavOrder() {
  const docs = await getCollection('docs');
  const bySlug = new Map<string, Doc>(docs.map((d) => [d.id.replace(/\.(md|mdx)$/, ''), d]));
  const sections = navigation.map((section) => ({
    title: section.title,
    entries: section.items
      .map((item) => bySlug.get(item.href.replace(/^\/grepai\//, '').replace(/\/$/, '')))
      .filter((d): d is Doc => Boolean(d)),
  }));
  const listed = new Set(sections.flatMap((s) => s.entries.map((e) => e.id)));
  const rest = docs.filter((d) => !listed.has(d.id));
  if (rest.length) sections.push({ title: 'Other pages', entries: rest });
  return sections;
}

export const mdUrl = (doc: Doc) => `${SITE}/${doc.id.replace(/\.(md|mdx)$/, '')}.md`;
export const blogMdUrl = (post: CollectionEntry<'blog'>) => `${SITE}/blog/${post.id.replace(/\.(md|mdx)$/, '')}.md`;
