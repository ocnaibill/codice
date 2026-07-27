import React, { useRef, useState } from 'react';

export default function AudioViewer({ fileUrl, bookId, initialProgress }) {
  const audioRef = useRef(null);
  const [playing, setPlaying] = useState(false);
  const [currentTime, setCurrentTime] = useState(0);
  const [duration, setDuration] = useState(0);
  const [speed, setSpeed] = useState(1);

  const togglePlay = () => {
    if (audioRef.current) {
      if (playing) {
        audioRef.current.pause();
      } else {
        audioRef.current.play();
      }
      setPlaying(!playing);
    }
  };

  const handleTimeUpdate = () => {
    if (audioRef.current) {
      setCurrentTime(audioRef.current.currentTime);
    }
  };

  const handleLoadedMetadata = () => {
    if (audioRef.current) {
      setDuration(audioRef.current.duration);
      if (initialProgress) {
        audioRef.current.currentTime = parseFloat(initialProgress) || 0;
      }
    }
  };

  const handleSeek = (e) => {
    const seekTime = parseFloat(e.target.value);
    if (audioRef.current) {
      audioRef.current.currentTime = seekTime;
    }
    setCurrentTime(seekTime);
  };

  const changeSpeed = () => {
    const speeds = [0.5, 0.75, 1, 1.25, 1.5, 2];
    const idx = speeds.indexOf(speed);
    const nextSpeed = speeds[(idx + 1) % speeds.length];
    setSpeed(nextSpeed);
    if (audioRef.current) {
      audioRef.current.playbackRate = nextSpeed;
    }
  };

  const formatTime = (t) => {
    const m = Math.floor(t / 60);
    const s = Math.floor(t % 60);
    return `${m}:${s.toString().padStart(2, '0')}`;
  };

  return (
    <div className="flex flex-col items-center justify-center h-full gap-8 p-10">
      <audio
        ref={audioRef}
        src={fileUrl}
        onTimeUpdate={handleTimeUpdate}
        onLoadedMetadata={handleLoadedMetadata}
        onEnded={() => setPlaying(false)}
        preload="metadata"
      />

      {/* Album art placeholder */}
      <div className="w-48 h-48 rounded-2xl bg-gradient-to-br from-blue-500 to-purple-600 flex items-center justify-center shadow-2xl">
        <span className="text-6xl">🎧</span>
      </div>

      {/* Playback controls */}
      <div className="flex flex-col items-center gap-4 w-full max-w-md">
        {/* Progress bar */}
        <div className="w-full flex items-center gap-3">
          <span className="text-xs text-zinc-500 font-mono w-12 text-right">
            {formatTime(currentTime)}
          </span>
          <input
            type="range"
            min="0"
            max={duration || 0}
            value={currentTime}
            onChange={handleSeek}
            className="flex-1 h-1.5 bg-zinc-800 rounded-full appearance-none cursor-pointer accent-blue-500"
          />
          <span className="text-xs text-zinc-500 font-mono w-12">
            {formatTime(duration)}
          </span>
        </div>

        {/* Buttons */}
        <div className="flex items-center gap-6">
          <button onClick={changeSpeed}
            className="px-3 py-1 text-xs font-medium bg-zinc-800 text-zinc-300 rounded-md hover:bg-zinc-700 transition-colors">
            {speed}x
          </button>
          <button onClick={togglePlay}
            className="w-14 h-14 rounded-full bg-blue-600 hover:bg-blue-500 text-white flex items-center justify-center text-2xl transition-all shadow-lg hover:scale-105">
            {playing ? '⏸' : '▶'}
          </button>
          <div className="w-14" /> {/* Spacer for alignment */}
        </div>
      </div>
    </div>
  );
}