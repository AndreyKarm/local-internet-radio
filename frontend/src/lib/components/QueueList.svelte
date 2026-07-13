<script lang="ts">
	import QueueSong from '$lib/components/QueueSong.svelte';
	import type { TQueueSong } from '$lib/types';

	let {
		queue,
		currentIndex
	}: { queue: TQueueSong[] | undefined; currentIndex: number | undefined } = $props();
</script>

<div class="queue-container">
	{#if queue}
		<div>
			{#each queue as song, i (i)}
				<QueueSong data={song} currently_playing={currentIndex === i} />
			{/each}
		</div>
	{/if}
</div>

<style>
	.queue-container {
		position: absolute;
		right: 0;
		height: 100vh;
		overflow-y: scroll;
	}

	.queue-container div {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}
</style>
