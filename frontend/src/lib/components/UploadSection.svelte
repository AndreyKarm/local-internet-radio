<script lang="ts">
	import { enhance } from '$app/forms';
	import type { SubmitFunction } from '@sveltejs/kit';

	let fileInput: HTMLInputElement;
	let formElement: HTMLFormElement;
	let isUploading = $state(false);

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
</script>

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

<style>
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
</style>
