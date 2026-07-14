import { get } from 'svelte/store';
import { settings } from '$lib/store/settings';
import { RADIO_URL } from '$lib';
import type { TQueueSong, TSongData } from '$lib/types';

export class PlayerState {
  data = $state<TSongData | undefined>(undefined);
  queue = $state<TQueueSong[]>([]);
  timestamp = $state(Date.now());
  elapsed = $state(0);
  remaining = $state(0);

  private audio: HTMLAudioElement | undefined;
  private ws: WebSocket | undefined;
  private reconnectTimer: ReturnType<typeof setTimeout> | undefined;
  private timer: ReturnType<typeof setInterval> | undefined;

  private lastPauseTime: number = 0;

  setAudioElement(el: HTMLAudioElement) {
    this.audio = el;

    if (get(settings).playing) {
      this.playStream();
    }
  }

  init() {
    this.connectWebSocket();
    this.startTimer();
  }

  destroy() {
    if (this.timer) clearInterval(this.timer);
    if (this.reconnectTimer) clearTimeout(this.reconnectTimer);
    if (this.ws) {
      this.ws.onclose = null;
      this.ws.close();
    }
  }

  togglePlay() {
    if (!this.audio) return;
    const current = get(settings);

    if (current.playing) {
      this.audio.pause();
      this.lastPauseTime = Date.now();
      settings.update((s) => ({ ...s, playing: false }));
    } else {
      const pauseDuration = Date.now() - this.lastPauseTime;

      if (this.audio.src && pauseDuration < 5000) {
        this.audio.play().catch((err) => {
          console.error('Playback failed:', err);
          settings.update((s) => ({ ...s, playing: false }));
        });
      } else {
        this.playStream();
      }
      settings.update((s) => ({ ...s, playing: true }));
    }
  }

  private playStream() {
    if (!this.audio) return;

    this.audio.src = `${RADIO_URL}/stream?t=${Date.now()}`;

    this.audio.play().catch((err) => {
      console.error('Playback failed:', err);
      settings.update((s) => ({ ...s, playing: false }));
    });
  }

  private connectWebSocket() {
    if (
      this.ws &&
      (this.ws.readyState === WebSocket.OPEN ||
        this.ws.readyState === WebSocket.CONNECTING)
    ) {
      return;
    }

    const wsUrl = RADIO_URL.replace(/^http/, 'ws') + '/ws/now-playing';
    this.ws = new WebSocket(wsUrl);

    this.ws.onmessage = async (event) => {
      try {
        const response = JSON.parse(event.data);

        if (response.track !== undefined || response.title !== undefined) {
          if (this.data?.title !== response.title) {
            this.timestamp = Date.now();
          }
          this.data = {
            ...(this.data as TSongData),
            ...response
          } as TSongData;

          if (response.queue) {
            this.queue = response.queue;
          }
        }

      } catch (e) {
        console.error('Failed to parse WebSocket message', e);
      }
    };

    this.ws.onclose = () => {
      console.log('WebSocket disconnected. Reconnecting in 3s...');
      this.reconnectTimer = setTimeout(() => this.connectWebSocket(), 3000);
    };

    this.ws.onerror = (err) => {
      console.error('WebSocket error:', err);
      this.ws?.close();
    };
  }

  setQueue(queue: TQueueSong[]) {
    this.queue = queue;
  }

  private startTimer() {
    this.timer = setInterval(() => {
      if (this.data && this.data.duration && this.data.started_at) {
        const now = Date.now();
        let currentElapsed = Math.floor(
          (now - this.data.started_at) / 1000
        );

        if (currentElapsed > this.data.duration) {
          currentElapsed = this.data.duration;
        }

        this.elapsed = currentElapsed;
        this.remaining = this.data.duration - this.elapsed;
      } else {
        this.elapsed = 0;
        this.remaining = 0;
      }
    }, 1000);
  }
}