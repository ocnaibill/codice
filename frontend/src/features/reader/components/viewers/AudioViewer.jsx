import React, { useRef, useState, useEffect } from 'react';
import { authenticatedUrl } from '../../../../lib/api';
import { markActivity } from '../../activity';
import { completionFor } from '../../progressRules';
import { audioPlaceProblem } from '../../placeCheck';
import { formatTime, spoken } from '../../audioTime';

const SKIP_BACK = 15;
const SKIP_FORWARD = 30;
const SPEEDS = [0.5, 0.75, 1, 1.25, 1.5, 2];

const iconProps = { viewBox: '0 0 24 24', width: 26, height: 26, fill: 'none', stroke: 'currentColor', strokeWidth: 1.6, strokeLinecap: 'round', strokeLinejoin: 'round', 'aria-hidden': true };
const round =
  'relative flex items-center justify-center rounded-full text-ink transition-[background-color,transform] duration-150 hover:bg-surface-alt active:scale-95 disabled:opacity-30 disabled:hover:bg-transparent disabled:active:scale-100';

export default function AudioViewer({ fileUrl, onProgress, initialProgress, onPlaceFailed }) {
  const audioRef = useRef(null);
  const [playing, setPlaying] = useState(false);
  const [currentTime, setCurrentTime] = useState(0);
  const [duration, setDuration] = useState(0);
  const [speed, setSpeed] = useState(1);
  const [problem, setProblem] = useState(null); // why it cannot play, in words
  const timeoutRef = useRef(null);

  const saveProgress = (time, total, ended = false) => {
    if (!onProgress) return;
    if (timeoutRef.current) clearTimeout(timeoutRef.current);
    timeoutRef.current = setTimeout(() => {
      const percent = total ? (time / total) * 100 : undefined;
      // Reaching the end finishes it; going back never says it is not finished.
      onProgress({ type: 'audio', track: 0, ms: Math.round(time * 1000) }, { percent, completed: ended ? true : completionFor(percent) })
        ?.catch?.((err) => console.error('Failed to save audio reading progress:', err));
    }, 1500);
  };

  useEffect(() => {
    return () => {
      if (timeoutRef.current) clearTimeout(timeoutRef.current);
    };
  }, []);

  // What the player does (the person, a headset, the end of the file) is what is shown: the state is the player's.
  const togglePlay = () => {
    const audio = audioRef.current;
    if (!audio) return;
    if (audio.paused) {
      setProblem(null);
      Promise.resolve(audio.play()).catch(() => setProblem('Não foi possível tocar este áudio.'));
    } else {
      audio.pause();
    }
  };

  // Back and forward by a few seconds: to hear again what was missed, or to skip on.
  const skip = (seconds) => {
    const audio = audioRef.current;
    if (!audio || !duration) return;
    const to = Math.min(duration, Math.max(0, audio.currentTime + seconds));
    audio.currentTime = to;
    setCurrentTime(to);
    saveProgress(to, duration);
  };

  const handleTimeUpdate = () => {
    if (audioRef.current) {
      const time = audioRef.current.currentTime;
      setCurrentTime(time);
      markActivity(); // audio that is playing is reading time
      saveProgress(time, audioRef.current.duration);
    }
  };

  const handleEnded = () => {
    if (audioRef.current) {
      saveProgress(audioRef.current.duration, audioRef.current.duration, true);
    }
  };

  // What happens once the player knows how long the audio is: the duration is shown, and a place asked for is opened.
  const handleLoadedMetadata = () => {
    if (audioRef.current) {
      setDuration(audioRef.current.duration);
      if (initialProgress) {
        // A point past the end is said, not hidden by starting from the beginning.
        const problem = audioPlaceProblem(initialProgress, audioRef.current.duration);
        if (problem) onPlaceFailed?.({ reason: problem });
        else audioRef.current.currentTime = parseFloat(initialProgress) || 0;
      }
    }
  };

  // The file can be here before the listener is: from the cache, or over a fast network, the player has its metadata
  // when the element is created and the event is already gone. Without this the duration stays 0:00 and a place asked for
  // (a chapter found by the search) is never opened.
  useEffect(() => {
    if (audioRef.current && audioRef.current.readyState >= 1) handleLoadedMetadata();
    // Only once, when the viewer opens: the event handles every later load.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const handleSeek = (e) => {
    const seekTime = parseFloat(e.target.value);
    if (audioRef.current) {
      audioRef.current.currentTime = seekTime;
      saveProgress(seekTime, audioRef.current.duration);
    }
    setCurrentTime(seekTime);
  };

  const changeSpeed = () => {
    const nextSpeed = SPEEDS[(SPEEDS.indexOf(speed) + 1) % SPEEDS.length];
    setSpeed(nextSpeed);
    if (audioRef.current) {
      audioRef.current.playbackRate = nextSpeed;
    }
  };

  // The keyboard. It is not taken from someone who is typing or on a control that uses the same keys (the slider).
  useEffect(() => {
    const onKey = (e) => {
      if (e.target?.closest?.('input, textarea, select, button, [contenteditable="true"]') || e.metaKey || e.ctrlKey || e.altKey) return;
      if (e.key === ' ') {
        e.preventDefault();
        togglePlay();
      } else if (e.key === 'ArrowLeft') skip(-SKIP_BACK);
      else if (e.key === 'ArrowRight') skip(SKIP_FORWARD);
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  });

  const retry = () => {
    setProblem(null);
    audioRef.current?.load();
  };

  const ready = duration > 0;

  return (
    <div className="flex min-h-full items-center justify-center p-4 sm:p-8">
      <audio
        ref={audioRef}
        src={authenticatedUrl(fileUrl)}
        onTimeUpdate={handleTimeUpdate}
        onLoadedMetadata={handleLoadedMetadata}
        onEnded={handleEnded}
        onPlay={() => setPlaying(true)}
        onPause={() => setPlaying(false)}
        onError={() => setProblem('Não foi possível carregar este áudio.')}
        preload="metadata"
      />

      <div className="flex w-full max-w-md animate-rise-in flex-col items-center gap-6 rounded-3xl border border-border-hairline bg-white p-6 shadow-lg sm:p-8">
        <div className="flex h-28 w-28 items-center justify-center rounded-2xl bg-brand/10 text-brand">
          <svg viewBox="0 0 24 24" width="48" height="48" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
            <path d="M4 15v-3a8 8 0 0 1 16 0v3" />
            <rect x="3" y="14" width="4" height="7" rx="1.5" />
            <rect x="17" y="14" width="4" height="7" rx="1.5" />
          </svg>
        </div>

        {problem && (
          <div role="alert" className="flex flex-col items-center gap-2 text-center">
            <p className="text-sm font-medium text-danger">{problem}</p>
            <button
              onClick={retry}
              className="min-h-11 rounded-full border border-border-hairline bg-white px-5 text-sm text-ink transition-[background-color,transform] duration-150 hover:bg-surface-alt active:scale-95"
            >
              Tentar de novo
            </button>
          </div>
        )}

        <div className="flex w-full flex-col gap-2">
          <input
            type="range"
            min="0"
            max={duration || 0}
            value={Math.min(currentTime, duration || 0)}
            onChange={handleSeek}
            disabled={!ready}
            aria-label="Posição no áudio"
            aria-valuetext={`${spoken(currentTime)} de ${ready ? spoken(duration) : 'duração desconhecida'}`}
            className="h-6 w-full cursor-pointer accent-brand disabled:cursor-default disabled:opacity-40"
          />
          <div className="flex justify-between font-mono text-xs text-ink-soft" aria-hidden="true">
            <span>{formatTime(currentTime)}</span>
            <span>{ready ? formatTime(duration) : '–:––'}</span>
          </div>
        </div>

        <div className="flex items-center gap-3">
          <button onClick={() => skip(-SKIP_BACK)} disabled={!ready} aria-label={`Voltar ${SKIP_BACK} segundos`} className={`${round} h-12 w-12`}>
            <svg {...iconProps}><path d="M3 12a9 9 0 1 0 3-6.7" /><path d="M3 4v5h5" /></svg>
            <span className="absolute text-[10px] font-semibold">{SKIP_BACK}</span>
          </button>
          <button
            onClick={togglePlay}
            aria-label={playing ? 'Pausar' : 'Tocar'}
            className="flex h-16 w-16 items-center justify-center rounded-full bg-brand text-white shadow-md transition-[background-color,transform] duration-150 hover:bg-brand-light active:scale-95"
          >
            {playing ? (
              <svg viewBox="0 0 24 24" width="26" height="26" fill="currentColor" aria-hidden="true"><rect x="6" y="5" width="4" height="14" rx="1" /><rect x="14" y="5" width="4" height="14" rx="1" /></svg>
            ) : (
              <svg viewBox="0 0 24 24" width="26" height="26" fill="currentColor" aria-hidden="true"><path d="M8 5.5v13a1 1 0 0 0 1.5.9l10-6.5a1 1 0 0 0 0-1.8l-10-6.5A1 1 0 0 0 8 5.5Z" /></svg>
            )}
          </button>
          <button onClick={() => skip(SKIP_FORWARD)} disabled={!ready} aria-label={`Avançar ${SKIP_FORWARD} segundos`} className={`${round} h-12 w-12`}>
            <svg {...iconProps}><path d="M21 12a9 9 0 1 1-3-6.7" /><path d="M21 4v5h-5" /></svg>
            <span className="absolute text-[10px] font-semibold">{SKIP_FORWARD}</span>
          </button>
        </div>

        <button
          onClick={changeSpeed}
          aria-label={`Velocidade ${speed}×, mudar`}
          title="Velocidade"
          className="min-h-11 min-w-16 rounded-full border border-border-hairline px-4 font-mono text-sm text-ink transition-[background-color,transform] duration-150 hover:bg-surface-alt active:scale-95"
        >
          {speed}×
        </button>
      </div>
    </div>
  );
}
