// @ts-check
import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';

// https://astro.build/config
export default defineConfig({
	integrations: [
		starlight({
			title: 'FlightStrips Docs',
			customCss: ['./src/tailwind.css'],
			social: [{ icon: 'github', label: 'GitHub', href: 'https://github.com/flightstrips' }],
			sidebar: [
				{
					label: 'Getting Started',
					items: [
						{ label: 'Start here', slug: 'getting-started/intro' },
						{ label: 'How the system fits together', slug: 'getting-started/features' },
						{ label: 'Connect EuroScope', slug: 'getting-started/es-plugin' },
						{ label: 'First-session checklist', slug: 'getting-started/first-session' },
					],
				},
				{
					label: 'Concepts',
					autogenerate: { directory: 'concepts' }
				},
				{
					label: 'Procedures',
					autogenerate: { directory: 'procedures' }
				},
				{
					label: 'Kastrup',
					autogenerate: { directory: 'ekch' }
				},
				{
					label: 'Troubleshooting',
					autogenerate: { directory: 'troubleshooting' }
				},
				{
					label: 'Development',
					autogenerate: { directory: 'dev' }
				},
				{
					label: 'Reference',
					autogenerate: { directory: 'reference' },
				},
			],
		}),
	],
});
