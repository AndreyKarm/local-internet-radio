<script lang="ts">
	import { RADIO_NAME } from '$lib';
	import favicon from '$lib/assets/favicon.svg';
	import './app.css';

	let { children } = $props();
</script>

<svelte:head>
	<link rel="icon" href={favicon} />
	<title>{RADIO_NAME}</title>
</svelte:head>

{@render children()}
