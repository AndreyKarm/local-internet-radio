<script lang="ts">
	import { enhance } from '$app/forms';
	import { ArrowLeft, ArrowRight, Pause, Play, Repeat, Shuffle } from '@lucide/svelte';

	let { playing, onTogglePlay }: { playing: boolean; onTogglePlay: () => void } = $props();
</script>

<div class="controls">
	<form method="POST" action="?/shuffle" use:enhance>
		<button type="submit"><Shuffle /></button>
	</form>

	<form method="POST" action="?/return" use:enhance>
		<button type="submit"><ArrowLeft /></button>
	</form>

	<button onclick={onTogglePlay} class="play">
		{#if playing}
			<Pause />
		{:else}
			<Play />
		{/if}
	</button>

	<form method="POST" action="?/skip" use:enhance>
		<button type="submit"><ArrowRight /></button>
	</form>

	<form method="POST" action="?/shuffle" use:enhance>
		<button type="submit"><Repeat /></button>
	</form>
</div>

<style>
	.controls {
		display: flex;
		flex-direction: row;
		align-items: center;
		justify-content: center;
		gap: 2rem;
	}

	.play {
		width: 4rem;
		height: 4rem;
	}
</style>
