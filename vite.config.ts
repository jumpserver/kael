import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

import { viteStaticCopy } from 'vite-plugin-static-copy';

// @ts-ignore
export default defineConfig({
	plugins: [
		sveltekit(),
		viteStaticCopy({
			targets: [
				{
					src: 'node_modules/onnxruntime-web/dist/*.jsep.*',

					dest: 'wasm'
				}
			]
		})
	],
	define: {
		APP_VERSION: JSON.stringify(process.env.npm_package_version),
		APP_BUILD_HASH: JSON.stringify(process.env.APP_BUILD_HASH || 'dev-build')
	},
	build: {
		sourcemap: true
	},
	worker: {
		format: 'es'
	},
	server: {
		port: 5173,
		watch: {
			// Exclude backend files from triggering HMR reloads
			ignored: [
				'**/backend/**',
				'**/backend/open_webui/jms/data/**',
				'**/*.cast'
			]
		},
		proxy: {
			'^/kael/api/': {
				target: 'http://localhost:8083',
				changeOrigin: true
				// 如果后端实际上是 /api 而不是 /kael/api，添加：
				// rewrite: p => p.replace(/^\/kael\/api\//, '/api/')
			},
			'/kael/openai': {
				target: 'http://localhost:8083',
				changeOrigin: true
			},
			'/kael/ollama': {
				target: 'http://localhost:8083',
				changeOrigin: true
			},
			'/kael/ws': {
				target: 'http://localhost:8083',
				ws: true,
				changeOrigin: true
			}
		}
	},
	esbuild: {
		pure: process.env.ENV === 'dev' ? [] : ['console.log', 'console.debug', 'console.error']
	}
});
