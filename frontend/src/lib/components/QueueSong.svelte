<script lang="ts">
	import { songTitle } from '$lib/utils';
	import CoverImage from './CoverImage.svelte';
	import type { TQueueSong } from '$lib/types';
	import { RADIO_URL } from '$lib';
	import { enhance } from '$app/forms';
	import { Trash } from '@lucide/svelte';

	let {
		data,
		currently_playing,
		index
	}: { data: TQueueSong; currently_playing: boolean; index: number } = $props();

	function confirmDelete(e: SubmitEvent) {
		const ok = confirm(`Delete "${songTitle(data)}"? This cannot be undone.`);
		if (!ok) {
			e.preventDefault();
		}
	}
</script>

<div class="queue-item-wrapper">
	<form method="POST" action="?/play" use:enhance class="play-form">
		<input type="hidden" name="index" value={index} />

		<button type="submit" class="queue-item-trigger">
			<div class="queue-item" class:currently-playing={currently_playing}>
				<CoverImage src={RADIO_URL + data.cover_url} class="queue-item-image" />
				<p title={songTitle(data)}>
					{songTitle(data)}
				</p>
			</div>
		</button>
	</form>

	<form method="POST" action="?/delete" use:enhance class="delete-form" onsubmit={confirmDelete}>
		<input type="hidden" name="key" value={data.key} />
		<button type="submit" class="delete-button" aria-label={`Delete ${songTitle(data)}`}>
			<Trash />
		</button>
	</form>
</div>

<style>
	.queue-item-wrapper {
		display: flex;
		flex-direction: row;
		max-width: 20rem;
	}

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

	.delete-form {
		display: flex;
	}

	.delete-button {
		background: var(--secondary);
		border-left: 1px solid var(--primary);
		border-radius: 0 0.5rem 0.5rem 0;
		padding: 0 0.75rem;
		transition:
			background-color 0.2s ease,
			color 0.2s ease;
	}

	.delete-button:hover {
		background: #c0392b;
		color: var(--text);
	}
</style>
