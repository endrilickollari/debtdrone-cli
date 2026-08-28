// @ts-check
import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';

const base = process.env.DOCS_BASE;

export default defineConfig({
  site: process.env.DOCS_SITE ?? 'https://cli.debtdrone.net',
  ...(base ? { base } : {}),
  outDir: './docs-dist',
  integrations: [
    starlight({
      title: 'DebtDrone CLI',
      description: 'Scan technical debt locally and enforce quality gates in CI.',
      favicon: '/debtdrone-logo.svg',
      logo: {
        src: './src/assets/debtdrone-logo.svg',
        alt: 'DebtDrone',
      },
      customCss: [
        '@fontsource-variable/geist',
        '@fontsource-variable/geist-mono',
        './src/styles/custom.css',
      ],
      components: {
        ThemeProvider: './src/components/ThemeProvider.astro',
        ThemeSelect: './src/components/ThemeSelect.astro',
      },
      editLink: {
        base: 'https://github.com/endrilickollari/debtdrone-cli/edit/main/src/content/docs/',
      },
      lastUpdated: true,
      social: [
        {
          icon: 'github',
          label: 'DebtDrone CLI on GitHub',
          href: 'https://github.com/endrilickollari/debtdrone-cli',
        },
      ],
      sidebar: [
        { label: 'Overview', slug: 'index' },
        {
          label: 'Getting started',
          items: [
            { label: 'Installation', slug: 'installation' },
            { label: 'Configuration', slug: 'configuration' },
          ],
        },
        {
          label: 'User guides',
          items: [
            { label: 'Interactive TUI', slug: 'tui-usage' },
            { label: 'CI/CD and headless CLI', slug: 'headless-usage' },
            { label: 'Coverage for Go consumers', slug: 'scanner-coverage' },
          ],
        },
        {
          label: 'Reference',
          items: [
            { label: 'System architecture', slug: 'architecture' },
            { label: 'Scanner ownership', slug: 'ownership' },
            { label: 'Versioning and releases', slug: 'versioning' },
          ],
        },
      ],
    }),
  ],
});
