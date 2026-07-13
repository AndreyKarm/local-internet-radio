<script lang="ts">
	import CoverImage from './CoverImage.svelte';
	import type { TQueueSong } from '$lib/types';
	import { RADIO_URL } from '$lib';

	let { data, currently_playing }: { data: TQueueSong; currently_playing: boolean } = $props();
</script>

<div class="queue-item" class:currently-playing={currently_playing}>
	<CoverImage src={RADIO_URL + data.cover_url} class="queue-item-image" />
	<p title={data.title.replace('.mp3', '')}>
		{data.title.replace('.mp3', '')}
		{#if !data.title.includes(data.artist) && data.artist != 'Unknown Artist'}
			- {data.artist}
		{/if}
	</p>
</div>

<style>
	.queue-item {
		display: flex;
		flex-direction: row;
		/* align-items: center; */
		background-color: var(--secondary);
		padding: 1rem;
		gap: 1rem;
		max-width: 20rem;
		border-radius: 0.5rem 0 0 0.5rem;
	}

	.queue-item p {
		text-overflow: ellipsis;
	}

	.currently-playing {
		background-color: var(--primary);
	}

	.currently-playing ::selection {
		color: var(--primary);
		background: var(--text);
	}
</style>
