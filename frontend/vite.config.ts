import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

export default defineConfig({
	plugins: [
		sveltekit(),
	],
	server: {
		allowedHosts: ["decode-handbag-overrate.ngrok-free.dev", "fielv.store"]
	}
});
