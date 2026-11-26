// interlinea.dev: the landing page (src/pages/index.astro) and the docs and
// help pages under /docs, built with Starlight from Markdown in git.
import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';

export default defineConfig({
  site: 'https://interlinea.dev',
  trailingSlash: 'ignore',
  integrations: [
    starlight({
      title: 'Interlinea',
      description: 'Documentation and help for Interlinea, the open-source CAT tool for translators.',
      logo: { src: './src/assets/logo.svg', alt: 'Interlinea' },
      favicon: '/favicon.svg',
      social: [{ icon: 'github', label: 'GitHub', href: 'https://github.com/4nonX/interlinea' }],
      editLink: { baseUrl: 'https://github.com/4nonX/interlinea/edit/main/site/' },
      lastUpdated: false,
      customCss: ['./src/styles/docs.css'],
      sidebar: [
        { label: 'Overview', link: '/docs/' },
        {
          label: 'Help',
          items: [
            'docs/help/install',
            'docs/help/first-project',
            'docs/help/check-before-delivery',
            'docs/help/your-data',
            'docs/help/team-server-quickstart',
            'docs/help/questions',
            'docs/help/report-a-problem',
          ],
        },
        {
          label: 'Manual',
          items: [
            { label: 'Using Interlinea', slug: 'docs/manual/guide' },
            { label: 'File formats', slug: 'docs/manual/formats' },
            { label: 'Matches, terms and checks', slug: 'docs/manual/matching' },
            { label: 'Languages', slug: 'docs/manual/languages' },
            { label: 'Command line', slug: 'docs/manual/cli' },
          ],
        },
        {
          label: 'Team server',
          items: [
            { label: 'Running a team server', slug: 'docs/manual/running' },
            { label: 'Sign-in and roles', slug: 'docs/manual/server' },
            { label: 'Translation services and assistants', slug: 'docs/manual/integration' },
            { label: 'API reference', slug: 'docs/manual/api' },
          ],
        },
        {
          label: 'Contributing',
          items: [
            { label: 'Development', slug: 'docs/manual/development' },
            { label: 'Desktop app internals', slug: 'docs/manual/desktop' },
          ],
        },
      ],
    }),
  ],
});
