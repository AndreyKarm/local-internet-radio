import { json } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { env } from '$env/dynamic/private';

const RADIO_URL = env.VITE_RADIO_URL ?? 'http://127.0.0.1:8080';

export const GET: RequestHandler = async ({ fetch }) => {
  try {
    const res = await fetch(`${RADIO_URL}/queue`);

    if (!res.ok) {
      return json({ error: 'Failed to fetch queue from backend' }, { status: res.status });
    }

    const data = await res.json();
    return json(data);
  } catch (err) {
    console.error('Proxy error:', err);
    return json({ error: 'Internal server error' }, { status: 500 });
  }
};