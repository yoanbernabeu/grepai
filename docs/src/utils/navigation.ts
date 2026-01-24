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
      { label: 'MCP Integration', href: '/grepai/mcp/', order: 4 },
      { label: 'Claude Code Subagent', href: '/grepai/subagent/', order: 5 },
      { label: 'Search Boost', href: '/grepai/search-boost/', order: 6 },
      { label: 'Hybrid Search', href: '/grepai/hybrid-search/', order: 7 },
      { label: 'Workspace Management', href: '/grepai/workspace/', order: 8 },
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
      { label: 'grepai watch', href: '/grepai/commands/grepai_watch/', order: 3 },
      { label: 'grepai search', href: '/grepai/commands/grepai_search/', order: 4 },
      { label: 'grepai trace', href: '/grepai/commands/grepai_trace/', order: 5 },
      { label: 'grepai trace callers', href: '/grepai/commands/grepai_trace_callers/', order: 6 },
      { label: 'grepai trace callees', href: '/grepai/commands/grepai_trace_callees/', order: 7 },
      { label: 'grepai trace graph', href: '/grepai/commands/grepai_trace_graph/', order: 8 },
      { label: 'grepai agent-setup', href: '/grepai/commands/grepai_agent-setup/', order: 9 },
      { label: 'grepai mcp-serve', href: '/grepai/commands/grepai_mcp-serve/', order: 10 },
      { label: 'grepai status', href: '/grepai/commands/grepai_status/', order: 11 },
      { label: 'grepai update', href: '/grepai/commands/grepai_update/', order: 12 },
      { label: 'grepai version', href: '/grepai/commands/grepai_version/', order: 13 },
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
    title: 'Contributing',
    items: [
      { label: 'How to Contribute', href: '/grepai/contributing/', order: 1 },
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
