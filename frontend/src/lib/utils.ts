import type { TQueueSong, TSongData } from "./types";

const AUDIO_EXT_REGEX = /\.(mp3|wav|flac|aac|ogg|oga|opus|m4a|m4b|wma|aiff?|alac|ape|wv|amr|au|mid|midi|caf|dsf|dff)$/i;

export function songTitle(data: TSongData | TQueueSong) {
  const title = data.title.replace(AUDIO_EXT_REGEX, '');
  const artist = data.artist

  let result = title

  if (!title.includes(artist) && artist != 'Unknown Artist') {
    result = `${title} - ${artist}`
  }

  return result
}