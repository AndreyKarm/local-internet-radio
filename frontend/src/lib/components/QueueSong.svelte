<script lang="ts">
	import CoverImage from './CoverImage.svelte';
	import type { TQueueSong } from '$lib/types';
	import { RADIO_URL } from '$lib';
	import { enhance } from '$app/forms';

	let {
		data,
		currently_playing,
		index
	}: { data: TQueueSong; currently_playing: boolean; index: number } = $props();
</script>

<form method="POST" action="?/play" use:enhance class="play-form">
	<input type="hidden" name="index" value={index} />

	<button type="submit" class="queue-item-trigger">
		<div class="queue-item" class:currently-playing={currently_playing}>
			<CoverImage src={RADIO_URL + data.cover_url} class="queue-item-image" />
			<p title={data.title.replace('.mp3', '')}>
				{data.title.replace('.mp3', '')}
				{#if !data.title.includes(data.artist) && data.artist != 'Unknown Artist'}
					- {data.artist}
				{/if}
			</p>
		</div>
	</button>
</form>

<style>
	.play-form {
		display: contents;
	}

	.queue-item-trigger {
		background: none;
		border: none;
		padding: 0;
		margin: 0;
		font: inherit;
		cursor: pointer;
		width: 100%;
		text-align: left;
	}

	.queue-item {
		display: flex;
		flex-direction: row;
		background: var(--secondary);
		padding: 1rem;
		gap: 1rem;
		max-width: 20rem;
		border-radius: 0.5rem 0 0 0.5rem;
		transition: background-color 0.2s ease;
	}

	.queue-item p {
		text-overflow: ellipsis;
	}

	.currently-playing {
		background: var(--primary) !important;
	}

	.currently-playing ::selection {
		color: var(--primary);
		background: var(--text);
	}

	.queue-item-trigger:hover .queue-item {
		filter: brightness(1.2);
	}
</style>
