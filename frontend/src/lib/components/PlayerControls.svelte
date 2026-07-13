<script lang="ts">
	import { enhance } from '$app/forms';
	import { ArrowLeft, ArrowRight, Pause, Play, Repeat, Repeat1, Shuffle } from '@lucide/svelte';

	let {
		playing,
		onTogglePlay,
		looping = false
	}: { playing: boolean; onTogglePlay: () => void; looping?: boolean } = $props();
</script>

<div class="controls">
	<form method="POST" action="?/shuffle" use:enhance>
		<button type="submit" title="Shuffle">
			<Shuffle />
		</button>
	</form>

	<form method="POST" action="?/previous" use:enhance>
		<button type="submit" title="Previous Song">
			<ArrowLeft />
		</button>
	</form>

	<button onclick={onTogglePlay} class="play" title="Play">
		{#if playing}
			<Pause />
		{:else}
			<Play />
		{/if}
	</button>

	<form method="POST" action="?/skip" use:enhance>
		<button type="submit" title="Skip">
			<ArrowRight />
		</button>
	</form>

	<form method="POST" action="?/loop" use:enhance>
		<button type="submit" title="Loop">
			{#if looping}
				<Repeat1 />
			{:else}
				<Repeat />
			{/if}
		</button>
	</form>
</div>

<style>
	.controls {
		display: flex;
		flex-direction: row;
		align-items: center;
		justify-content: center;
		gap: 1rem;
	}

	.play {
		width: 4rem;
		height: 4rem;
	}
</style>
