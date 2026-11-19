// @ts-check
import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';

// Served from the repository's GitHub Pages address. If the site moves to a
// custom domain, set `site` to that domain and drop `base`.
export default defineConfig({
	site: 'https://gizawnahom.github.io',
	base: '/ledgerops',
	integrations: [
		starlight({
			title: 'ledgerops',
			description:
				'A double-entry ledger service with no overdrafts, safe retries, append-only history and a one-call check that the books balance.',
			customCss: ['./src/styles/fonts.css', './src/styles/custom.css'],
			components: {
				Hero: './src/components/Hero.astro',
				ThemeProvider: './src/components/ThemeProvider.astro',
				ThemeSelect: './src/components/ThemeSelect.astro',
			},
			expressiveCode: {
				themes: ['github-dark'],
				styleOverrides: {
					borderRadius: '0px',
					codeBackground: '#0a0a0a',
					borderColor: '#2a2a2a',
					codeFontFamily: "'IBM Plex Mono', ui-monospace, Menlo, monospace",
					frames: { shadowColor: 'transparent' },
				},
			},
			social: [
				{ icon: 'github', label: 'GitHub', href: 'https://github.com/gizawNahom/ledgerops' },
			],
			sidebar: [
				{ label: 'Quickstart', slug: 'quickstart' },
				{ label: 'Guarantees', slug: 'guarantees' },
				{ label: 'Compare', slug: 'compare' },
				{ label: 'Status, limits and operating', slug: 'status' },
			],
		}),
	],
});
