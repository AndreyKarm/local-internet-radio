<script lang="ts">
	let { listeners }: { listeners: number | undefined } = $props();
</script>

<div class="listeners-container">
	Listeners: {listeners}
</div>

<style>
	.listeners-container {
		position: absolute;
		background-color: var(--secondary);
		padding: 0.5rem;
		margin: 1rem;
		border-radius: 0.5rem;
		top: 0;
		left: 0;
	}
</style>
