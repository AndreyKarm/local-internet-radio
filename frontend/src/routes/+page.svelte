<script lang="ts" module>
	export type TrackData = {
		album: string;
		artist: string;
		cover: string;
		title: string;
		track: string;
		duration: number;
		started_at: number;
	};
</script>

<script lang="ts">
	import { onMount } from 'svelte';
	import { enhance } from '$app/forms';
	import { settings } from '$lib/store/settings';
	import type { SubmitFunction } from './$types';
	import { Pause, Play, Volume2, VolumeX } from '@lucide/svelte';
	import { RADIO_URL } from '$lib';

	// State
	let trackData: TrackData | undefined = $state();
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
				const data = JSON.parse(event.data);
				if (trackData?.title !== data.title) {
					timestamp = Date.now();
				}
				trackData = data;
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
			if (trackData && trackData.duration && trackData.started_at) {
				const now = Date.now();
				let currentElapsed = Math.floor((now - trackData.started_at) / 1000);

				if (currentElapsed > trackData.duration) {
					currentElapsed = trackData.duration;
				}

				elapsed = currentElapsed;
				remaining = trackData.duration - elapsed;
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
			src={trackData ? `${RADIO_URL}${trackData?.cover}?t=${timestamp}` : '/boykisser.png'}
			alt="cover"
		/>

		{#if trackData}
			<h2>{trackData.title} - {trackData.artist}</h2>
			{#if trackData.duration}
				<div class="time-display">
					<span class="time">{formatTime(elapsed)}</span>
					<div class="progress-bar">
						<div class="progress-fill" style="width: {(elapsed / trackData.duration) * 100}%"></div>
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

		<button onclick={togglePlay} class="play">
			{#if $settings.playing}
				<Pause />
			{:else}
				<Play />
			{/if}
		</button>
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
