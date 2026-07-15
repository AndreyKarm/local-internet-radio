import { fail } from '@sveltejs/kit';
import type { Actions, PageServerLoad } from './$types';
import { env } from '$env/dynamic/private';

const RADIO_URL = env.VITE_RADIO_URL ?? 'http://127.0.0.1:8080';

export const load: PageServerLoad = async ({ fetch }) => {
  try {
    const [queueRes, filterRes] = await Promise.all([
      fetch(`${RADIO_URL}/queue`),
      fetch(`${RADIO_URL}/filter/get`)
    ]);

    const queueData = queueRes.ok ? await queueRes.json() : { queue: [] };
    const filterData = filterRes.ok ? await filterRes.json() : { filter: '' };

    return {
      queue: queueData.queue ?? [],
      currentFilter: filterData.filter ?? ''
    };
  } catch (err) {
    console.error('Failed to load queue:', err);
    return { queue: [] };
  }
};

export const actions = {
  upload: async ({ request, fetch }) => {
    const data = await request.formData();
    const file = data.get('track') as File | null;

    if (!file || file.size === 0) {
      return fail(400, { message: 'No file selected' });
    }

    if (!file.type.startsWith('audio/')) {
      return fail(400, { message: 'Must be an audio file' });
    }

    const formData = new FormData();
    formData.append('track', file);

    try {
      const res = await fetch(`${RADIO_URL}/upload`, {
        method: 'POST',
        body: formData
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
      const res = await fetch(`${RADIO_URL}/shuffle`, {
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
      const res = await fetch(`${RADIO_URL}/loop`, {
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
      const res = await fetch(`${RADIO_URL}/previous`, {
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
      const res = await fetch(`${RADIO_URL}/skip`, {
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
      const res = await fetch(`${RADIO_URL}/play?index=${index}`, {
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

  delete: async ({ request, fetch }) => {
    const data = await request.formData();
    const key = data.get('key');

    if (!key) {
      return fail(400, { message: 'Key is required' });
    }

    try {
      const res = await fetch(
        `${RADIO_URL}/delete?key=${encodeURIComponent(key.toString())}`,
        { method: 'DELETE' }
      );

      if (!res.ok) {
        const errorText = await res.text();
        return fail(res.status, { message: `Backend error: ${errorText}` });
      }

      return { success: true };
    } catch (err) {
      console.error('Delete failed:', err);
      return fail(500, { message: 'Internal server error while deleting.' });
    }
  },

  setFilter: async ({ request, fetch }) => {
    const data = await request.formData();
    const filter = data.get('filter');

    if (!filter) {
      return fail(400, { message: 'Filter is required' });
    }

    try {
      const res = await fetch(`${RADIO_URL}/filter`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ filter: filter.toString() })
      });

      if (!res.ok) {
        const errorText = await res.text();
        return fail(res.status, { message: `Backend error: ${errorText}` });
      }

      return { success: true, filter: filter.toString() };
    } catch (err) {
      console.error('Set filter failed:', err);
      return fail(500, { message: 'Internal server error while setting filter.' });
    }
  }
} satisfies Actions;