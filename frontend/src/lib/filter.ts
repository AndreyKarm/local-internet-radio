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
    name: 'Underwater',
    filter:
      'lowpass=f=500,volume=1.2,aecho=in_gain=0.8:out_gain=0.8:delays=50:decays=0.5,lowpass=f=400'
  },
  {
    name: 'Telephone',
    filter:
      'highpass=f=1000,lowpass=f=3000,volume=2,acrusher=level_in=1:level_out=0.8:bits=8:mode=log:aa=1'
  },
  {
    name: 'Bitcrushed',
    filter:
      'acrusher=level_in=1:level_out=1:bits=4:mode=log:aa=1,volume=2'
  },
  {
    name: 'Bass Boost',
    filter: 'bass=g=10'
  },
  {
    name: 'Bass Cut',
    filter: 'bass=g=-10'
  },
  {
    name: "Super Equalizer",
    filter: "superequalizer=1b=10:2b=10:3b=1:4b=5:5b=7:6b=5:7b=2:8b=3:9b=4:10b=5:11b=6:12b=7:13b=8:14b=8:15b=9:16b=9:17b=10:18b=10[a];[a]loudnorm=I=-16:TP=-1.5:LRA=14[aout]"
  },
  {
    name: 'Retro 8-Bit',
    filter: 'acrusher=bits=4:mode=log,aresample=8000,volume=1.5'
  },
  {
    name: 'Lo-Fi Crunch',
    filter: 'acrusher=bits=8:mode=log,volume=1.2'
  },
  {
    name: 'Broken Radio',
    filter: 'acrusher=bits=6,aresample=11025,volume=1.5'
  }
];