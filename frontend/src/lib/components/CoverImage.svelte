<script lang="ts">
	let {
		src,
		alt = 'cover',
		fallback = '/boykisser.png',
		class: class_ = ''
	}: { src: string; alt?: string; fallback?: string; class?: string } = $props();

	function handleError(ev: Event) {
		const target = ev.currentTarget as HTMLImageElement;
		target.src = fallback;
	}
</script>

<img draggable="false" class={['cover', class_]} {src} {alt} onerror={handleError} />

<style>
	.cover {
		width: 20rem;
		height: 20rem;
		object-fit: cover;
		border-radius: 0.5rem;
	}

	.queue-item-image {
		height: 5rem !important;
		width: 5rem !important;
	}
</style>
