export type TFilter = {
  name: string;
  filter: string;
};

export const AudioFilters: TFilter[] = [
  {
    name: 'Clean',
    filter: 'anull'
  },
  {
    name: 'Old Radio',
    filter: 'highpass=f=400,lowpass=f=3500,volume=1.5'
  },
  {
    name: 'AM Radio',
    filter:
      'highpass=f=500,lowpass=f=2800,acrusher=level_in=1:level_out=0.7:bits=6:mode=log:aa=1,vibrato=f=4:d=0.3,acompressor=threshold=0.1:ratio=9:attack=5:release=50[voice];anoisesrc=d=9999:amplitude=0.02:c=pink:r=44100,aformat=channel_layouts=stereo[noise];[voice][noise]amix=inputs=2:duration=first:weights=1 1[mixed];[mixed]tremolo=f=0.15:d=0.4[aout]'
  },
  {
    name: 'Destroyed',
    filter:
      'highpass=f=800,lowpass=f=2000,volume=6dB,acrusher=level_in=1.2:level_out=0.5:bits=4:mode=log:aa=0.5,alimiter=limit=0.6:attack=1:release=20,vibrato=f=6:d=0.6,tremolo=f=0.3:d=0.7,acompressor=threshold=0.05:ratio=20:attack=2:release=80:makeup=3[voice];anoisesrc=d=9999:amplitude=0.05:c=pink:r=44100,aformat=channel_layouts=stereo[hiss];anoisesrc=d=9999:amplitude=0.03:c=white:r=44100,aformat=channel_layouts=stereo,highpass=f=3000[crackle];[voice][hiss]amix=inputs=2:duration=first:weights=1 1[mix1];[mix1][crackle]amix=inputs=2:duration=first:weights=1 0.6[mixed];[mixed]tremolo=f=0.1:d=0.9[aout]'
  },
  {
    name: 'Underwater',
    filter:
      'lowpass=f=500,volume=1.2,aecho=in_gain=0.8:out_gain=0.8:delay=50:decay=0.5[voice];[voice]lowpass=f=400[aout]'
  },
  {
    name: 'Lo-Fi Vinyl',
    filter:
      'highpass=f=100,lowpass=f=4000,volume=1.5,aecho=in_gain=0.1:out_gain=0.1:delay=30:decay=0.1[voice];anoisesrc=d=9999:amplitude=0.02:c=white:r=44100,lowpass=f=1000[crackle];[voice][crackle]amix=inputs=2:duration=first:weights=1 0.3[aout]'
  },
  {
    name: 'Telephone',
    filter:
      'highpass=f=1000,lowpass=f=3000,volume=2,acrusher=level_in=1:level_out=0.8:bits=8:mode=log:aa=1[aout]'
  },
  {
    name: 'Space/Psychedelic',
    filter:
      'aecho=in_gain=0.5:out_gain=0.5:delay=500:decay=0.8,vibrato=f=0.5:d=0.5,atempo=1.1[aout]'
  },
  {
    name: 'Bitcrushed',
    filter:
      'acrusher=level_in=1:level_out=1:bits=4:mode=log:aa=1,volume=2[aout]'
  },
  {
    name: 'Megaphone',
    filter:
      'highpass=f=600,lowpass=f=3500,volume=2,acompressor=threshold=0.1:ratio=10:attack=1:release=10[aout]'
  },
  {
    name: 'Radio Static',
    filter:
      'anoisesrc=d=9999:amplitude=0.05:c=white:r=44100,highpass=f=2000[noise];[0:a][noise]amix=inputs=2:duration=first:weights=1 0.2[aout]'
  }
];