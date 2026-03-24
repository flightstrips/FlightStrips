// @ts-check
import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';

// https://astro.build/config
export default defineConfig({
	integrations: [
		starlight({
			title: 'FlightStrips Docs',
			social: [{ icon: 'github', label: 'GitHub', href: 'https://github.com/flightstrips' }],
			sidebar: [
				{
					label: 'Getting Started',
					items: [
						// Each item here is one entry in the navigation menu.
						{ label: 'Introduction', slug: 'getting-started/intro' },
						{ label: 'Features', slug: 'getting-started/features' },
						{ label: 'EuroScope plugin', slug: 'getting-started/es-plugion' },
					],
				},
				{
					label: 'Kastrup',
					autogenerate: { directory: 'ekch' }
				},
				{
					label: 'Reference',
					autogenerate: { directory: 'reference' },
				},
			],
		}),
	],
});
