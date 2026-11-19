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
