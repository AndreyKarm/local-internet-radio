<script lang="ts">
	import { onMount } from 'svelte';
	import { settings } from '$lib/store/settings';
	import { PlayerState } from '$lib/player.svelte';
	import { songTitle } from '$lib/utils';
	import {
		ArrowLeft,
		ArrowRight,
		Pause,
		Play,
		Repeat,
		Repeat1,
		Search,
		Shuffle,
		Trash,
		Volume2,
		VolumeX,
		X
	} from '@lucide/svelte';
	import { deserialize, enhance } from '$app/forms';
	import CoverImage from '$lib/components/CoverImage.svelte';
	import ProgressBar from '$lib/components/ProgressBar.svelte';
	import Listeners from '$lib/components/Listeners.svelte';
	import { RADIO_URL, RADIO_NAME } from '$lib';
	import type { PageProps } from './$types';
	import { invalidateAll } from '$app/navigation';

	let { data }: PageProps = $props();

	const player = new PlayerState();
	const RadioName = RADIO_NAME;
	const placeholderImage = '/boykisser.png';

	// Sync queue
	$effect(() => {
		player.setQueue(data.queue ?? []);
	});

	// Current track data
	let currentTrack = $derived(player.data);
	let currentIndex = $derived(player.queue.findIndex((song) => song.key === currentTrack?.track));

	// Search
	let searchQuery = $state('');

	let filteredQueue = $derived(
		player.queue
			.map((song, i) => ({ song, i })) // keep original index for play/delete
			.filter(({ song }) => {
				const q = searchQuery.trim().toLowerCase();
				if (!q) return true;
				return songTitle(song).toLowerCase().includes(q);
			})
	);

	// Change page title on song change
	$effect(() => {
		if (currentTrack && currentTrack.title && player.data) {
			document.title = songTitle(player.data);
		} else {
			document.title = RadioName;
		}
	});

	// Current track cover
	let coverSrc = $derived(
		currentTrack?.cover
			? `${RADIO_URL}${currentTrack.cover}?t=${player.timestamp}`
			: placeholderImage
	);

	// Audio element
	let audioEl: HTMLAudioElement;

	// Update the audio element when the player changes
	$effect(() => {
		if (audioEl) player.setAudioElement(audioEl);
	});

	// Update the volume when the settings change
	$effect(() => {
		if (audioEl) audioEl.volume = $settings.volume;
	});

	// Initialize the player
	onMount(() => {
		player.init();
		return () => player.destroy();
	});

	// Handle volume changes
	function onVolumeChange(v: number) {
		settings.update((s) => ({ ...s, volume: v }));
	}

	// Handle input events on the volume slider
	function handleInput(e: Event) {
		const target = e.currentTarget as HTMLInputElement;
		onVolumeChange(parseFloat(target.value));
	}

	// Handle wheel events on the volume slider
	function handleWheel(e: WheelEvent) {
		e.preventDefault();
		const power = 0.0005;
		const current = $settings.volume;
		if (e.deltaY < 0) {
			onVolumeChange(Math.min(0.1, current + power));
		} else if (e.deltaY > 0) {
			onVolumeChange(Math.max(0, current - power));
		}
	}

	// Delete song confirmation
	function confirmDelete(e: SubmitEvent) {
		if (!player.data) return;
		const ok = confirm(`Delete "${songTitle(player.data)}"? This cannot be undone.`);
		if (!ok) {
			e.preventDefault();
		}
	}

	// Scroll current track into view
	function scrollIntoViewIfCurrent(el: HTMLDivElement, isCurrent: boolean) {
		if (isCurrent) {
			el.scrollIntoView({ behavior: 'smooth', block: 'center' });
		}

		return {
			update(isCurrent: boolean) {
				if (isCurrent) {
					el.scrollIntoView({ behavior: 'smooth', block: 'center' });
				}
			}
		};
	}

	// Uploading
	let fileInput: HTMLInputElement;
	let isUploading = $state(false);
	let uploadTotal = $state(0);
	let uploadCurrent = $state(0);
	let currentFileName = $state('');

	async function handleFilesSelected(e: Event) {
		const target = e.currentTarget as HTMLInputElement;
		const files = target.files;
		if (!files || files.length === 0) return;

		isUploading = true;
		uploadTotal = files.length;
		uploadCurrent = 0;
		const errors: string[] = [];

		for (const file of Array.from(files)) {
			uploadCurrent += 1;
			currentFileName = file.name;

			const formData = new FormData();
			formData.append('track', file);

			try {
				const res = await fetch('?/upload', {
					method: 'POST',
					body: formData
				});
				const result = deserialize(await res.text());

				if (result.type === 'failure') {
					errors.push(`${file.name}: ${result.data?.message ?? 'Unknown error'}`);
				} else if (result.type === 'error') {
					errors.push(`${file.name}: ${result.error?.message ?? 'Unknown error'}`);
				}
			} catch (err) {
				console.error('Upload failed:', err);
				errors.push(`${file.name}: Network error`);
			}
		}

		isUploading = false;
		currentFileName = '';
		target.value = ''; // reset so selecting the same file(s) again re-fires onchange

		if (errors.length > 0) {
			alert(`Some uploads failed:\n\n${errors.join('\n')}`);
		} else {
			alert(
				`Successfully uploaded ${uploadTotal} song${uploadTotal > 1 ? 's' : ''}! ` +
					`They'll play in the next rotation.`
			);
		}

		await invalidateAll(); // refresh queue after uploads
	}
</script>

<main class="container">
	<Listeners listeners={player.data?.listeners} />

	<div class="card">
		<CoverImage src={coverSrc} />

		{#if player.data && player.data.title}
			<h2 title={songTitle(player.data)}>
				{songTitle(player.data)}
			</h2>

			{#if player.data.duration}
				<ProgressBar elapsed={player.elapsed} duration={player.data.duration} />
			{/if}

			<div class="volume-control" onwheel={handleWheel}>
				{#if $settings.volume === 0}
					<VolumeX />
				{:else}
					<Volume2 />
				{/if}

				<input
					type="range"
					min={0}
					max={1}
					step={0.01}
					value={$settings.volume}
					oninput={handleInput}
					class="volume-slider"
					style="--progress: {$settings.volume * 100}%"
				/>
			</div>
		{:else}
			<h2>Loading...</h2>
		{/if}

		<audio
			bind:this={audioEl}
			onplay={() => ($settings.playing = true)}
			onpause={() => ($settings.playing = false)}
		></audio>

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

			<button onclick={() => player.togglePlay()} class="play" title="Play">
				{#if $settings.playing}
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
					{#if player.data?.loop ?? false}
						<Repeat1 />
					{:else}
						<Repeat />
					{/if}
				</button>
			</form>
		</div>
	</div>

	<div class="upload-section">
		<input
			type="file"
			name="track"
			accept="audio/*"
			multiple
			bind:this={fileInput}
			onchange={handleFilesSelected}
			style="display: none;"
		/>

		<button
			type="button"
			class="upload-btn"
			onclick={() => fileInput.click()}
			disabled={isUploading}
		>
			{#if isUploading}
				<div>
					<p>
						Uploading {uploadCurrent}/{uploadTotal}
					</p>
					<p>
						{currentFileName ? currentFileName : ''}
					</p>
				</div>
			{:else}
				Upload Songs
			{/if}
		</button>
	</div>

	<div class="queue-container">
		<div class="search-container">
			<div class="search-bar">
				<Search size={18} />
				<input
					type="text"
					placeholder="Search songs..."
					bind:value={searchQuery}
					class="search-input"
				/>
				{#if searchQuery}
					<button
						type="button"
						class="search-clear"
						onclick={() => (searchQuery = '')}
						aria-label="Clear search"
					>
						<X size={16} />
					</button>
				{/if}
			</div>
		</div>

		{#if searchQuery && filteredQueue.length === 0}
			<p class="no-results">No songs match "{searchQuery}"</p>
		{/if}

		{#if player.queue}
			{#each filteredQueue as { song, i } (i)}
				<div
					title={songTitle(song)}
					class="queue-item-wrapper"
					use:scrollIntoViewIfCurrent={currentIndex === i}
				>
					<form method="POST" action="?/play" use:enhance class="play-form">
						<input type="hidden" name="index" value={i} />

						<button type="submit" class="queue-item-trigger">
							<div class="queue-item" class:currently-playing={currentIndex === i}>
								<CoverImage src={RADIO_URL + song.cover_url} class="queue-item-image" />
								<p title={songTitle(song)}>
									{songTitle(song)}
								</p>
							</div>
						</button>
					</form>

					<form
						method="POST"
						action="?/delete"
						use:enhance
						class="delete-form"
						onsubmit={confirmDelete}
					>
						<input type="hidden" name="key" value={song.key} />
						<button type="submit" class="delete-button" aria-label={`Delete ${songTitle(song)}`}>
							<Trash />
						</button>
					</form>
				</div>
			{/each}
		{/if}
	</div>
</main>

<style>
	.container {
		position: relative;
		display: flex;
		justify-content: center;
		align-items: center;
		flex-direction: column;
		min-height: 100vh;
		padding: 4rem 1rem;
	}

	/* Volume control */
	.volume-control {
		display: flex;
		align-items: center;
		width: 100%;
		gap: 1rem;
	}

	.volume-slider {
		-webkit-appearance: none;
		appearance: none;

		width: 100%;
		max-width: 30rem;

		height: 0.5rem;
		background: linear-gradient(
			to right,
			var(--primary) var(--progress),
			var(--background) var(--progress)
		);
		border-radius: 1rem;
		outline: none;
		cursor: pointer;
	}

	.volume-slider::-webkit-slider-thumb {
		-webkit-appearance: none;
		appearance: none;
		width: 1.2rem;
		height: 1.2rem;
		border-radius: 50%;
		background: var(--text);
		cursor: pointer;
		transition:
			transform 0.1s,
			background 0.2s;
	}

	.volume-slider::-webkit-slider-thumb:hover {
		transform: scale(1.2);
		background: var(--accent);
	}

	.volume-slider::-moz-range-thumb {
		width: 1.2rem;
		height: 1.2rem;
		border-radius: 50%;
		background: var(--text);
		border: none;
		cursor: pointer;
		transition: all 0.2s;
	}

	.volume-slider::-moz-range-thumb:hover {
		transform: scale(1.2);
		background: var(--accent);
	}

	/* Player Card */
	.card {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		width: 100%;
		max-width: 35rem;
		background: var(--secondary);
		gap: 0.5rem;
		padding: 2rem;
		border-radius: 2rem;
		text-align: center;
		transition: width 0.2s ease;
	}

	.card h2 {
		width: 100%;
		max-width: 30rem;
		text-overflow: ellipsis;
		overflow: hidden;
	}

	/* Player Controls */
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

	/* Upload section */
	.upload-section {
		margin-top: 1rem;
		width: 20rem;
	}

	.upload-btn {
		gap: 0.5rem;
		width: 100%;
		font-weight: bold;
	}

	.upload-btn:disabled {
		opacity: 0.7;
		cursor: not-allowed;
	}

	/* Queue list */
	.queue-container {
		position: absolute;
		right: 0;
		height: 100vh;
		overflow-y: scroll;
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
		/* max-width: 100%; */
	}

	.search-container {
		z-index: 1;
		position: sticky;
		top: 0;
		padding: 0.5rem 0;
		display: flex;
		align-items: center;
		gap: 0.5rem;
		background-color: var(--background);
		width: 100%;
	}

	.search-bar {
		background-color: var(--secondary);
		border-radius: 0.5rem;
		display: flex;
		align-items: center;
		gap: 0.5rem;
		width: 100%;
		padding: 0.5rem 0.75rem;
	}

	.search-input {
		flex: 1;
		background: none;
		border: none;
		outline: none;
		color: var(--text);
		font-size: 1rem;
		min-width: 0;
	}

	.search-clear {
		background: none;
		padding: 0;
		display: flex;
		align-items: center;
		opacity: 0.7;
	}

	.search-clear:hover {
		opacity: 1;
	}

	.no-results {
		text-align: center;
		opacity: 0.6;
		padding: 1rem 0;
		width: 100%;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	/* Queue Item */
	.queue-item-wrapper {
		display: flex;
		flex-direction: row;
		align-items: stretch;
		width: 100%;
		max-width: 25rem;
		min-width: 0;
	}

	.play-form {
		display: contents;
	}

	.queue-item-trigger {
		background: none;
		padding: 0;
		flex: 1 1 auto;
		min-width: 0;
		text-align: left;
	}

	.queue-item {
		display: flex;
		flex-direction: row;
		align-items: center;
		background: var(--secondary);
		padding: 1rem;
		gap: 1rem;
		width: 100%;
		min-width: 0;
		border-radius: 0.5rem 0 0 0.5rem;
		transition: all 0.2s ease;
	}

	.queue-item p {
		flex: 1 1 auto;
		min-width: 0;
		white-space: nowrap;
		text-overflow: ellipsis;
		overflow: hidden;
	}

	.currently-playing {
		background: var(--primary) !important;
	}

	.currently-playing ::selection {
		color: var(--primary);
		background: var(--text);
	}

	.queue-item-trigger:hover .queue-item {
		background-color: var(--accent);
	}

	.delete-form {
		display: flex;
		flex-shrink: 0;
	}

	.delete-button {
		flex-shrink: 0;
		background: var(--secondary);
		border-left: 1px solid var(--primary);
		border-radius: 0 0.5rem 0.5rem 0;
		padding: 0 0.75rem;
		width: 3rem;
		transition:
			background-color 0.2s ease,
			color 0.2s ease;
	}

	.delete-button:hover {
		background: var(--danger);
		color: var(--text);
	}

	@media (max-width: 768px) {
		.container {
			padding-bottom: 0.25rem;
		}

		.controls {
			gap: 0.2rem;
		}

		.queue-container {
			position: relative;
			right: auto;
			height: auto;
			max-height: 60vh;
			width: 100%;
			margin-top: 2rem;
		}

		.queue-item-wrapper {
			max-width: none;
		}

		.search-bar {
			max-width: 100%;
		}

		.no-results {
			max-width: 100%;
		}

		.search-container {
			max-width: 100%;
		}
	}
</style>
