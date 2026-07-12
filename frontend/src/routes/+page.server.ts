import { fail } from '@sveltejs/kit';
import type { Actions } from './$types';
import { env } from '$env/dynamic/private';

const RADIO_INTERNAL_URL = env.RADIO_INTERNAL_URL ?? 'http://localhost:8080';

export const actions = {
  upload: async ({ request, fetch }) => {
    console.log('Uploading track');
    const data = await request.formData();
    const file = data.get('track') as File | null;

    if (!file || file.size === 0) {
      return fail(400, { message: 'No file selected' });
    }

    if (!file.type.startsWith('audio/')) {
      return fail(400, { message: 'Must be an audio file' });
    }

    const backendFormData = new FormData();
    backendFormData.append('track', file);

    try {
      const res = await fetch(`${RADIO_INTERNAL_URL}/upload`, {
        method: 'POST',
        body: backendFormData
      });

      if (!res.ok) {
        const errorText = await res.text();
        return fail(res.status, { message: `Backend error: ${errorText}` });
      }

      return { success: true };
    } catch (err) {
      console.error('Upload failed:', err);
      return fail(500, { message: 'Internal server error while uploading.' });
    }
  }
} satisfies Actions;