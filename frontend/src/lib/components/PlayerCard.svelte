<script lang="ts">
	import CoverImage from './CoverImage.svelte';
	import ProgressBar from './ProgressBar.svelte';
	import VolumeControl from './VolumeControl.svelte';
	import PlayerControls from './PlayerControls.svelte';
	import type { TSongData } from '$lib/types';
	import { RADIO_URL } from '$lib';
	import { songTitle } from '$lib/utils';

	let {
		data,
		timestamp,
		elapsed,
		playing,
		volume,
		onTogglePlay,
		onVolumeChange,
		onPlay,
		onPause,
		audioRef
	}: {
		data: TSongData | undefined;
		timestamp: number;
		elapsed: number;
		playing: boolean;
		volume: number;
		onTogglePlay: () => void;
		onVolumeChange: (v: number) => void;
		onPlay: () => void;
		onPause: () => void;
		audioRef: (el: HTMLAudioElement) => void;
	} = $props();

	let coverSrc = $derived(data ? `${RADIO_URL}${data.cover}?t=${timestamp}` : '/boykisser.png');

	let audioEl: HTMLAudioElement;

	$effect(() => {
		if (audioEl) audioRef(audioEl);
	});

	$effect(() => {
		if (audioEl) audioEl.volume = volume;
	});
</script>

<div class="card">
	<CoverImage src={coverSrc} />

	{#if data && data.title}
		<h2 title={songTitle(data)}>
			{songTitle(data)}
		</h2>

		{#if data.duration}
			<ProgressBar {elapsed} duration={data.duration} />
		{/if}
	{:else}
		<h2>Loading...</h2>
	{/if}

	<audio bind:this={audioEl} onplay={onPlay} onpause={onPause}></audio>

	<VolumeControl {volume} {onVolumeChange} />

	<PlayerControls {playing} {onTogglePlay} looping={data?.loop ?? false} />
</div>

<style>
	.card {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		background: var(--secondary);
		gap: 0.5rem;
		padding: 2rem;
		border-radius: 2rem;
		text-align: center;
		transition: width 0.2s ease;
	}

	.card h2 {
		width: 30rem;
		text-overflow: ellipsis;
	}
</style>
