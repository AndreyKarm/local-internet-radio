import { fail } from '@sveltejs/kit';
import type { Actions } from './$types';
import { env } from '$env/dynamic/private';

const RADIO_INTERNAL_URL = env.VITE_RADIO_URL ?? 'http://127.0.0.1:8080';

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
  },

  shuffle: async ({ fetch }) => {
    try {
      const res = await fetch(`${RADIO_INTERNAL_URL}/shuffle`, {
        method: 'POST'
      });

      if (!res.ok) {
        return fail(res.status, { message: 'Failed to shuffle' });
      }
      return { success: true };
    } catch (err) {
      console.error('Shuffle failed:', err);
      return fail(500, { message: 'Internal server error while shuffling.' });
    }
  },

  loop: async ({ fetch }) => {
    try {
      const res = await fetch(`${RADIO_INTERNAL_URL}/loop`, {
        method: 'POST'
      });

      if (!res.ok) {
        return fail(res.status, { message: 'Failed to loop' });
      }
      return { success: true };
    } catch (err) {
      console.error('Loop failed:', err);
      return fail(500, { message: 'Internal server error loop.' });
    }
  },

  previous: async ({ fetch }) => {
    try {
      const res = await fetch(`${RADIO_INTERNAL_URL}/previous`, {
        method: 'POST'
      });

      if (!res.ok) {
        return fail(res.status, { message: 'Failed to come back to previous track' });
      }
      return { success: true };
    } catch (err) {
      console.error('Previous selection failed:', err);
      return fail(500, { message: 'Internal server error while coming back to previous.' });
    }
  },

  skip: async ({ fetch }) => {
    try {
      const res = await fetch(`${RADIO_INTERNAL_URL}/skip`, {
        method: 'POST'
      });

      if (!res.ok) {
        return fail(res.status, { message: 'Failed to skip track' });
      }
      return { success: true };
    } catch (err) {
      console.error('Skip failed:', err);
      return fail(500, { message: 'Internal server error while skipping.' });
    }
  },

  play: async ({ request, fetch }) => {
    const data = await request.formData();
    const index = data.get('index');

    if (index === null) {
      return fail(400, { message: 'Index is required' });
    }

    try {
      const res = await fetch(`${RADIO_INTERNAL_URL}/play?index=${index}`, {
        method: 'POST'
      });

      if (!res.ok) {
        const errorText = await res.text();
        return fail(res.status, { message: `Backend error: ${errorText}` });
      }

      return { success: true };
    } catch (err) {
      console.error('Play failed:', err);
      return fail(500, { message: 'Internal server error while playing.' });
    }
  },
} satisfies Actions;