export type TSongData = {
  album: string;
  artist: string;
  cover: string;
  title: string;
  track: string;
  duration: number;
  started_at: number;

  queue: TQueueSong[];
  queue_index: number;
};

export type TQueueSong = {
  key: string;
  title: string;
  artist: string;
  album: string;
  cover_url: string;
};