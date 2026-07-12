<script lang="ts" module>
	export type TrackData = {
		album: string;
		artist: string;
		cover: string;
		title: string;
		track: string;
		duration: number;
		started_at: number;

		queue: Array<string>;
		queue_index: number;
	};
</script>

<script lang="ts">
	import { onMount } from 'svelte';
	import { enhance } from '$app/forms';
	import { settings } from '$lib/store/settings';
	import type { SubmitFunction } from './$types';
	import {
		// ArrowRight,
		Pause,
		Play,
		// Shuffle,
		Volume2,
		VolumeX
	} from '@lucide/svelte';
	import { RADIO_URL } from '$lib';

	// State
	let data: TrackData | undefined = $state();
	let timestamp = $state(Date.now());
	let elapsed = $state(0);
	let remaining = $state(0);

	// Audio
	let audio: HTMLAudioElement;
	let ws: WebSocket;
	let reconnectTimer: ReturnType<typeof setTimeout> | undefined;

	// Song Upload
	let fileInput: HTMLInputElement;
	let formElement: HTMLFormElement;
	let isUploading = $state(false);

	function connectWebSocket() {
		if (ws && (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING)) {
			return;
		}

		const wsUrl = RADIO_URL.replace(/^http/, 'ws') + '/ws/now-playing';
		ws = new WebSocket(wsUrl);

		ws.onmessage = (event) => {
			try {
				const response = JSON.parse(event.data);
				console.log('WebSocket message:', response);
				if (data?.title !== response.title) {
					timestamp = Date.now();
				}
				data = response;
			} catch (e) {
				console.error('Failed to parse WebSocket message', e);
			}
		};

		ws.onclose = () => {
			console.log('WebSocket disconnected. Reconnecting in 3s...');
			reconnectTimer = setTimeout(connectWebSocket, 3000);
		};

		ws.onerror = (err) => {
			console.error('WebSocket error:', err);
			ws.close();
		};
	}

	function togglePlay() {
		if ($settings.playing) {
			audio.pause();
			audio.removeAttribute('src');
			audio.load();
		} else {
			audio.src = `${RADIO_URL}/stream?t=${Date.now()}`;
			audio.load();
			audio.play().catch((err) => {
				console.error('Playback failed:', err);
				$settings.playing = false;
			});
		}
		$settings.playing = !$settings.playing;
	}

	function formatTime(seconds: number) {
		if (isNaN(seconds) || seconds < 0) return '0:00';
		const m = Math.floor(seconds / 60);
		const s = Math.floor(seconds % 60);
		return `${m}:${s.toString().padStart(2, '0')}`;
	}

	const uploadHandler: SubmitFunction = () => {
		isUploading = true;

		return async ({ result, update }) => {
			isUploading = false;

			if (result.type === 'success') {
				alert('Song uploaded successfully! It will play in the next rotation.');
			} else if (result.type === 'failure') {
				alert(`Upload failed: ${result.data?.message || 'Unknown error'}`);
			}

			update();
		};
	};

	onMount(() => {
		connectWebSocket();

		if ($settings.playing) {
			audio.src = `${RADIO_URL}/stream?t=${Date.now()}`;
			audio.load();
			audio.play().catch(() => {
				$settings.playing = false;
			});
		}

		const timer = setInterval(() => {
			if (data && data.duration && data.started_at) {
				const now = Date.now();
				let currentElapsed = Math.floor((now - data.started_at) / 1000);

				if (currentElapsed > data.duration) {
					currentElapsed = data.duration;
				}

				elapsed = currentElapsed;
				remaining = data.duration - elapsed;
			} else {
				elapsed = 0;
				remaining = 0;
			}
		}, 1000);

		return () => {
			clearInterval(timer);
			clearTimeout(reconnectTimer);
			if (ws) {
				ws.onclose = null;
				ws.close();
			}
		};
	});
</script>

<main class="container">
	<div class="card">
		<img
			draggable="false"
			class="cover"
			src={data ? `${RADIO_URL}${data?.cover}?t=${timestamp}` : '/boykisser.png'}
			alt="cover"
		/>

		{#if data}
			<h2>{data.title} - {data.artist}</h2>
			{#if data.duration}
				<div class="time-display">
					<span class="time">{formatTime(elapsed)}</span>
					<div class="progress-bar">
						<div class="progress-fill" style="width: {(elapsed / data.duration) * 100}%"></div>
					</div>
					<span class="time">-{formatTime(remaining)}</span>
				</div>
			{/if}
		{:else}
			<h2>Loading...</h2>
		{/if}

		<audio
			bind:this={audio}
			bind:volume={$settings.volume}
			onplay={() => ($settings.playing = true)}
			onpause={() => ($settings.playing = false)}
		></audio>

		<div class="volume-control">
			{#if $settings.volume === 0}
				<VolumeX />
			{:else}
				<Volume2 />
			{/if}

			<input
				type="range"
				min={0}
				max={1 / 10}
				step={1 / 1000}
				bind:value={$settings.volume}
				class="volume-slider"
				style="--progress: {$settings.volume * 1000}%"
			/>
		</div>

		<div class="controls">
			<!-- <form method="POST" action="?/shuffle" use:enhance>
				<button type="submit">
					<Shuffle />
				</button>
			</form> -->

			<button onclick={togglePlay} class="play">
				{#if $settings.playing}
					<Pause />
				{:else}
					<Play />
				{/if}
			</button>

			<!-- <form method="POST" action="?/skip" use:enhance>
				<button type="submit">
					<ArrowRight />
				</button>
			</form> -->
		</div>
	</div>

	<div class="upload-section">
		<form
			method="POST"
			action="?/upload"
			enctype="multipart/form-data"
			use:enhance={uploadHandler}
			bind:this={formElement}
		>
			<input
				type="file"
				name="track"
				accept="audio/*"
				bind:this={fileInput}
				onchange={() => formElement.requestSubmit()}
				style="display: none;"
			/>

			<button
				type="button"
				class="upload-btn"
				onclick={() => fileInput.click()}
				disabled={isUploading}
			>
				{#if isUploading}
					Uploading...
				{:else}
					Upload Song
				{/if}
			</button>
		</form>
	</div>

	<!-- {#if data}
		<div>
			{#each data.queue as item, i (i)}
				<p>{item}</p>
			{/each}
		</div>
	{/if} -->
</main>

<style>
	.container {
		display: flex;
		justify-content: center;
		align-items: center;
		flex-direction: column;
		height: 100vh;
	}

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

	.cover {
		width: 20rem;
		height: 20rem;
		object-fit: cover;
		border-radius: 0.5rem;
	}

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

	.time-display {
		display: flex;
		align-items: center;
		width: 100%;
		gap: 0.5rem;
		font-family: monospace;
		font-size: 0.9rem;
	}

	.time {
		min-width: 3rem;
	}

	/* Uploading */
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

	:global(.spin) {
		animation: spin 1s linear infinite;
	}
	@keyframes spin {
		from {
			transform: rotate(0deg);
		}
		to {
			transform: rotate(360deg);
		}
	}
</style>
