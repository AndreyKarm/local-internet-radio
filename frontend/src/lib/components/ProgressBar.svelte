<script lang="ts">
	let { elapsed, duration }: { elapsed: number; duration: number } = $props();

	function formatTime(seconds: number) {
		if (isNaN(seconds) || seconds < 0) return '0:00';
		const m = Math.floor(seconds / 60);
		const s = Math.floor(seconds % 60);
		return `${m}:${s.toString().padStart(2, '0')}`;
	}

	let remaining = $derived(duration - elapsed);
</script>

<div class="time-display">
	<span class="time">{formatTime(elapsed)}</span>
	<div class="progress-bar">
		<div
			class="progress-fill"
			style="width: {duration > 0 ? (elapsed / duration) * 100 : 0}%"
		></div>
	</div>
	<span class="time">-{formatTime(remaining)}</span>
</div>

<style>
	.time-display {
		display: flex;
		align-items: center;
		width: 100%;
		gap: 0.5rem;
		font-size: 0.9rem;
	}

	.time {
		min-width: 3rem;
	}

	.progress-bar {
		flex-grow: 1;
		height: 0.5rem;
		width: 25rem;
		background: var(--background);
		border-radius: 1rem;
		overflow: hidden;
	}

	.progress-fill {
		height: 100%;
		background: var(--primary);
		transition: width 0.2s linear;
	}
</style>
