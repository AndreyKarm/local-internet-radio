<script lang="ts">
	import { onMount } from 'svelte';
	import { settings } from '$lib/store/settings';
	import { PlayerState } from '$lib/player.svelte';
	import PlayerCard from '$lib/components/PlayerCard.svelte';
	import UploadSection from '$lib/components/UploadSection.svelte';
	import QueueList from '$lib/components/QueueList.svelte';

	const player = new PlayerState();

	function handleWheel(e: WheelEvent) {
		const power = 0.01;
		if (e.deltaY < 0) {
			$settings.volume = Math.min(1, $settings.volume + power);
		} else if (e.deltaY > 0) {
			$settings.volume = Math.max(0, $settings.volume - power);
		}
	}

	onMount(() => {
		player.init();
		return () => player.destroy();
	});
</script>

<svelte:window onwheel={handleWheel} />

<main class="container">
	<PlayerCard
		data={player.data}
		timestamp={player.timestamp}
		elapsed={player.elapsed}
		playing={$settings.playing}
		volume={$settings.volume}
		onTogglePlay={() => player.togglePlay()}
		onVolumeChange={(v) => ($settings.volume = v)}
		onPlay={() => ($settings.playing = true)}
		onPause={() => ($settings.playing = false)}
		audioRef={(el) => player.setAudioElement(el)}
	/>

	<UploadSection />

	<QueueList queue={player.data?.queue} currentIndex={player.data?.queue_index} />
</main>

<style>
	.container {
		position: relative;
		display: flex;
		justify-content: center;
		align-items: center;
		flex-direction: column;
		height: 100vh;
	}
</style>
