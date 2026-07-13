<script lang="ts">
	import { onMount } from 'svelte';
	import { settings } from '$lib/store/settings';
	import { PlayerState } from '$lib/player.svelte';
	import PlayerCard from '$lib/components/PlayerCard.svelte';
	import UploadSection from '$lib/components/UploadSection.svelte';
	import QueueList from '$lib/components/QueueList.svelte';
	import Listeners from '$lib/components/Listeners.svelte';

	const player = new PlayerState();

	onMount(() => {
		player.init();
		return () => player.destroy();
	});
</script>

<main class="container">
	<Listeners listeners={player.data?.listeners} />

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
