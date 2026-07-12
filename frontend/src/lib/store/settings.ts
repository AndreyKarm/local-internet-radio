import { writable } from 'svelte/store';
import { browser } from '$app/environment';

export type Settings = {
  volume: number;
  playing: boolean;
}


const DEFAULT_SETTINGS: Settings = {
  volume: 0.2,
  playing: false,
};

const getInitialSettings = (): Settings => {
  if (!browser) return DEFAULT_SETTINGS;

  const stored = localStorage.getItem('settings');
  if (!stored) return DEFAULT_SETTINGS;

  try {
    return JSON.parse(stored) as Settings;
  } catch (e) {
    console.error("Failed to parse settings", e);
    return DEFAULT_SETTINGS;
  }
};


export const settings = writable<Settings>(getInitialSettings());

if (browser) {
  settings.subscribe((setting) => {
    localStorage.setItem('settings', JSON.stringify(setting))
  })
}