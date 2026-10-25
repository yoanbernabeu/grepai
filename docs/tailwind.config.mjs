/** @type {import('tailwindcss').Config} */

// Colors are CSS variables (RGB channels) defined in src/styles/global.css,
// so every utility follows the light/dark theme without a `dark:` variant.
const token = (name) => `rgb(var(--${name}) / <alpha-value>)`;

export default {
  content: ['./src/**/*.{astro,html,js,jsx,md,mdx,svelte,ts,tsx,vue}'],
  theme: {
    extend: {
      colors: {
        paper: {
          DEFAULT: token('paper'),
          raised: token('paper-raised'),
          sunk: token('paper-sunk'),
        },
        rule: token('rule'),
        ink: {
          DEFAULT: token('ink'),
          muted: token('ink-muted'),
        },
        gold: {
          DEFAULT: token('gold'),
          ink: token('gold-ink'),
          on: token('on-gold'),
        },
        code: {
          bg: token('code-bg'),
          fg: token('code-fg'),
          muted: token('code-muted'),
        },
      },
      fontFamily: {
        display: ['"Young Serif"', 'Georgia', 'serif'],
        sans: ['"Public Sans"', 'system-ui', '-apple-system', 'Segoe UI', 'sans-serif'],
        mono: ['"IBM Plex Mono"', 'ui-monospace', 'SFMono-Regular', 'Menlo', 'monospace'],
      },
      borderRadius: {
        // Two radii only: controls (3px) and framed blocks (6px).
        sm: '3px',
        DEFAULT: '3px',
        md: '6px',
      },
      maxWidth: {
        prose: '68ch',
      },
      typography: {
        DEFAULT: {
          css: {
            '--tw-prose-body': 'rgb(var(--ink))',
            '--tw-prose-headings': 'rgb(var(--ink))',
            '--tw-prose-lead': 'rgb(var(--ink-muted))',
            '--tw-prose-links': 'rgb(var(--gold-ink))',
            '--tw-prose-bold': 'rgb(var(--ink))',
            '--tw-prose-counters': 'rgb(var(--ink-muted))',
            '--tw-prose-bullets': 'rgb(var(--ink-muted))',
            '--tw-prose-hr': 'rgb(var(--rule))',
            '--tw-prose-quotes': 'rgb(var(--ink))',
            '--tw-prose-quote-borders': 'rgb(var(--rule))',
            '--tw-prose-captions': 'rgb(var(--ink-muted))',
            '--tw-prose-code': 'rgb(var(--ink))',
            '--tw-prose-pre-code': 'rgb(var(--code-fg))',
            '--tw-prose-pre-bg': 'rgb(var(--code-bg))',
            '--tw-prose-th-borders': 'rgb(var(--rule))',
            '--tw-prose-td-borders': 'rgb(var(--rule))',
            maxWidth: 'none',
            lineHeight: '1.7',
            a: {
              fontWeight: '500',
              textDecorationLine: 'underline',
              textDecorationColor: 'rgb(var(--gold) / 0.6)',
              textDecorationThickness: '1px',
              textUnderlineOffset: '3px',
              '&:hover': {
                textDecorationColor: 'rgb(var(--gold))',
                textDecorationThickness: '2px',
              },
            },
            'h1, h2': {
              fontFamily: '"Young Serif", Georgia, serif',
              fontWeight: '400',
              letterSpacing: '-0.01em',
            },
            h2: {
              marginTop: '2.2em',
              paddingTop: '0.4em',
            },
            h3: {
              fontWeight: '600',
            },
            code: {
              backgroundColor: 'rgb(var(--paper-sunk))',
              padding: '0.15em 0.35em',
              borderRadius: '3px',
              fontWeight: '400',
              fontSize: '0.875em',
            },
            'code::before': { content: '""' },
            'code::after': { content: '""' },
            'a code': { color: 'inherit' },
            pre: {
              borderRadius: '6px',
              fontSize: '0.85em',
              lineHeight: '1.65',
            },
            'pre code': {
              backgroundColor: 'transparent',
              padding: '0',
            },
            blockquote: {
              fontStyle: 'normal',
              fontWeight: '400',
              backgroundColor: 'rgb(var(--paper-sunk) / 0.6)',
              borderLeftWidth: '2px',
              padding: '0.6em 1em',
              borderRadius: '0 6px 6px 0',
            },
            'blockquote p:first-of-type::before': { content: 'none' },
            'blockquote p:last-of-type::after': { content: 'none' },
            table: {
              fontSize: '0.9em',
            },
            'thead th': {
              fontWeight: '600',
              backgroundColor: 'rgb(var(--paper-sunk) / 0.7)',
              padding: '0.6em 0.8em',
            },
            'tbody td, tfoot td': {
              padding: '0.6em 0.8em',
            },
          },
        },
      },
    },
  },
  plugins: [
    require('@tailwindcss/typography'),
  ],
};
