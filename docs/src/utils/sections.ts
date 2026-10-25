import type { ImageMetadata } from 'astro';
import start from '../assets/illustrations/section-start.jpg';
import search from '../assets/illustrations/section-search.jpg';
import graph from '../assets/illustrations/section-graph.jpg';
import indexing from '../assets/illustrations/section-indexing.jpg';
import agents from '../assets/illustrations/section-agents.jpg';
import config from '../assets/illustrations/section-config.jpg';

export interface SectionArt {
  image: ImageMetadata;
  /** Path under src/assets, for the build-time Open Graph renderer */
  file: string;
  /** object-position that keeps the subject in a wide crop */
  focus: string;
}

// One illustration per navigation section, all from the same world as the hero.
const art: Record<string, SectionArt> = {
  'Getting Started': { image: start, file: 'illustrations/section-start.jpg', focus: '70% 55%' },
  Search: { image: search, file: 'illustrations/section-search.jpg', focus: '55% 50%' },
  'Code Graph': { image: graph, file: 'illustrations/section-graph.jpg', focus: '50% 55%' },
  Indexing: { image: indexing, file: 'illustrations/section-indexing.jpg', focus: '70% 50%' },
  'AI Agents': { image: agents, file: 'illustrations/section-agents.jpg', focus: '40% 60%' },
  Configuration: { image: config, file: 'illustrations/section-config.jpg', focus: '75% 60%' },
  Community: { image: start, file: 'illustrations/section-start.jpg', focus: '70% 55%' },
};

/** Illustration for a navigation section; command reference pages get none. */
export function sectionArt(section?: string): SectionArt | undefined {
  return section ? art[section] : undefined;
}
