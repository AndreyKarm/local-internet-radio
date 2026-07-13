<script lang="ts">
	let { listeners }: { listeners: number | undefined } = $props();
</script>

<div class="listeners-container">
	<span class="live-dot"></span>
	<span class="text">
		{listeners ?? 0}
		{listeners === 1 ? 'Listener' : 'Listeners'}
	</span>
</div>

<style>
	.listeners-container {
		position: absolute;
		top: 1rem;
		left: 1rem;
		background-color: var(--secondary);
		padding: 0.5rem 1rem;
		border-radius: 9999px;
		display: flex;
		align-items: center;
		gap: 0.6rem;
		font-weight: 600;
		font-size: 0.9rem;
		user-select: none;
	}

	.live-dot {
		width: 10px;
		height: 10px;
		background-color: #ef4444;
		border-radius: 50%;
		animation: pulse 2s infinite;
	}

	@keyframes pulse {
		0% {
			box-shadow: 0 0 0 0 rgba(239, 68, 68, 0.6);
		}
		70% {
			box-shadow: 0 0 0 8px rgba(239, 68, 68, 0);
		}
		100% {
			box-shadow: 0 0 0 0 rgba(239, 68, 68, 0);
		}
	}
</style>
