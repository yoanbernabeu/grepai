export interface NavItem {
  label: string;
  href: string;
  order?: number;
}

export interface NavSection {
  title: string;
  items: NavItem[];
}

export const navigation: NavSection[] = [
  {
    title: 'Getting Started',
    items: [
      { label: 'Introduction', href: '/grepai/getting-started/', order: 1 },
      { label: 'Installation', href: '/grepai/installation/', order: 2 },
      { label: 'Quick Start', href: '/grepai/quickstart/', order: 3 },
    ],
  },
  {
    title: 'Features',
    items: [
      { label: 'Semantic Search', href: '/grepai/search-guide/', order: 1 },
      { label: 'File Watching', href: '/grepai/watch-guide/', order: 2 },
      { label: 'Call Graph Analysis', href: '/grepai/trace/', order: 3 },
      { label: 'Repository Planning Graph', href: '/grepai/rpg/', order: 4 },
      { label: 'MCP Integration', href: '/grepai/mcp/', order: 5 },
      { label: 'Claude Code Subagent', href: '/grepai/subagent/', order: 6 },
      { label: 'AI Agent Skills', href: '/grepai/skills/', order: 7 },
      { label: 'Search Boost', href: '/grepai/search-boost/', order: 8 },
      { label: 'Hybrid Search', href: '/grepai/hybrid-search/', order: 9 },
      { label: 'Git Worktrees', href: '/grepai/git-worktrees/', order: 10 },
      { label: 'Workspace Management', href: '/grepai/workspace/', order: 11 },
    ],
  },
  {
    title: 'Configuration',
    items: [
      { label: 'Config File', href: '/grepai/configuration/', order: 1 },
    ],
  },
  {
    title: 'Commands',
    items: [
      { label: 'grepai', href: '/grepai/commands/grepai/', order: 1 },
      { label: 'grepai init', href: '/grepai/commands/grepai_init/', order: 2 },
      { label: 'grepai index', href: '/grepai/commands/grepai_index/', order: 3 },
      { label: 'grepai watch', href: '/grepai/commands/grepai_watch/', order: 4 },
      { label: 'grepai search', href: '/grepai/commands/grepai_search/', order: 5 },
      { label: 'grepai trace', href: '/grepai/commands/grepai_trace/', order: 6 },
      { label: 'grepai trace callers', href: '/grepai/commands/grepai_trace_callers/', order: 7 },
      { label: 'grepai trace callees', href: '/grepai/commands/grepai_trace_callees/', order: 8 },
      { label: 'grepai trace graph', href: '/grepai/commands/grepai_trace_graph/', order: 9 },
      { label: 'grepai refs', href: '/grepai/commands/grepai_refs/', order: 10 },
      { label: 'grepai refs readers', href: '/grepai/commands/grepai_refs_readers/', order: 11 },
      { label: 'grepai refs writers', href: '/grepai/commands/grepai_refs_writers/', order: 12 },
      { label: 'grepai refs graph', href: '/grepai/commands/grepai_refs_graph/', order: 13 },
      { label: 'grepai rpg', href: '/grepai/commands/grepai_rpg/', order: 14 },
      { label: 'grepai rpg search', href: '/grepai/commands/grepai_rpg_search/', order: 15 },
      { label: 'grepai rpg fetch', href: '/grepai/commands/grepai_rpg_fetch/', order: 16 },
      { label: 'grepai rpg explore', href: '/grepai/commands/grepai_rpg_explore/', order: 17 },
      { label: 'grepai agent-setup', href: '/grepai/commands/grepai_agent-setup/', order: 18 },
      { label: 'grepai mcp-serve', href: '/grepai/commands/grepai_mcp-serve/', order: 19 },
      { label: 'grepai status', href: '/grepai/commands/grepai_status/', order: 20 },
      { label: 'grepai update', href: '/grepai/commands/grepai_update/', order: 21 },
      { label: 'grepai version', href: '/grepai/commands/grepai_version/', order: 22 },
      { label: 'grepai workspace', href: '/grepai/commands/grepai_workspace/', order: 23 },
      { label: 'grepai workspace list', href: '/grepai/commands/grepai_workspace_list/', order: 24 },
      { label: 'grepai workspace show', href: '/grepai/commands/grepai_workspace_show/', order: 25 },
      { label: 'grepai workspace status', href: '/grepai/commands/grepai_workspace_status/', order: 26 },
      { label: 'grepai workspace create', href: '/grepai/commands/grepai_workspace_create/', order: 27 },
      { label: 'grepai workspace add', href: '/grepai/commands/grepai_workspace_add/', order: 28 },
      { label: 'grepai workspace remove', href: '/grepai/commands/grepai_workspace_remove/', order: 29 },
      { label: 'grepai workspace delete', href: '/grepai/commands/grepai_workspace_delete/', order: 30 },
    ],
  },
  {
    title: 'Backends',
    items: [
      { label: 'Embedders', href: '/grepai/backends/embedders/', order: 1 },
      { label: 'Stores', href: '/grepai/backends/stores/', order: 2 },
    ],
  },
  {
    title: 'Community',
    items: [
      { label: 'Community Tools', href: '/grepai/community-tools/', order: 1 },
      { label: 'How to Contribute', href: '/grepai/contributing/', order: 2 },
    ],
  },
];

export function getAllPages(): NavItem[] {
  return navigation.flatMap((section) => section.items);
}

// Normalize slug for comparison: remove extensions and leading/trailing slashes
function normalizeSlug(slug: string): string {
  return slug.replace(/\.(md|mdx)$/, '').replace(/^\/|\/$/g, '');
}

// Normalize href for comparison: remove base path and leading/trailing slashes
function normalizeHref(href: string): string {
  return href.replace(/^\/grepai\//, '').replace(/^\/|\/$/g, '');
}

// Check if href matches slug (exact match after normalization)
function hrefMatchesSlug(href: string, slug: string): boolean {
  return normalizeHref(href) === normalizeSlug(slug);
}

export function findCurrentSection(slug: string): string | undefined {
  for (const section of navigation) {
    if (section.items.some((item) => hrefMatchesSlug(item.href, slug))) {
      return section.title;
    }
  }
  return undefined;
}

export function findPrevNext(currentSlug: string): { prev?: NavItem; next?: NavItem } {
  const allPages = getAllPages();
  const currentIndex = allPages.findIndex((page) => hrefMatchesSlug(page.href, currentSlug));

  if (currentIndex === -1) {
    return {};
  }

  return {
    prev: currentIndex > 0 ? allPages[currentIndex - 1] : undefined,
    next: currentIndex < allPages.length - 1 ? allPages[currentIndex + 1] : undefined,
  };
}
