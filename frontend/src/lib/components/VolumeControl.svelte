<script lang="ts">
	import { Volume2, VolumeX } from '@lucide/svelte';

	let { volume, onVolumeChange }: { volume: number; onVolumeChange: (value: number) => void } =
		$props();

	function handleInput(e: Event) {
		const target = e.currentTarget as HTMLInputElement;
		onVolumeChange(parseFloat(target.value));
	}

	function handleWheel(e: WheelEvent) {
		e.preventDefault();
		const power = 0.01;
		if (e.deltaY < 0) {
			onVolumeChange(Math.min(1, volume + power));
		} else if (e.deltaY > 0) {
			onVolumeChange(Math.max(0, volume - power));
		}
	}
</script>

<div class="volume-control" onwheel={handleWheel}>
	{#if volume === 0}
		<VolumeX />
	{:else}
		<Volume2 />
	{/if}

	<input
		type="range"
		min={0}
		max={1}
		step={0.005}
		value={volume}
		oninput={handleInput}
		class="volume-slider"
		style="--progress: {volume * 100}%"
	/>
</div>

<style>
	.volume-control {
		display: flex;
		align-items: center;
		width: 100%;
		gap: 1rem;
	}

	.volume-slider {
		-webkit-appearance: none;
		appearance: none;

		width: 30rem;

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
</style>
