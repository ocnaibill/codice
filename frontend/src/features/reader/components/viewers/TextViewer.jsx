import React, { useState, useEffect } from 'react';
import { authenticatedUrl } from '../../../../lib/api';

export default function TextViewer({ fileUrl, bookId, initialProgress }) {
  const [content, setContent] = useState('');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [fontSize, setFontSize] = useState(16);

  useEffect(() => {
    let cancelled = false;
    const loadText = async () => {
      try {
        const resp = await fetch(authenticatedUrl(fileUrl));
        if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
        const text = await resp.text();
        if (!cancelled) {
          setContent(text);
          setLoading(false);
        }
      } catch (err) {
        if (!cancelled) {
          setError(err.message);
          setLoading(false);
        }
      }
    };
    loadText();
    return () => { cancelled = true; };
  }, [fileUrl]);

  if (loading) {
    return (
      <div className="flex items-center justify-center h-full text-zinc-500 animate-pulse font-medium">
        Loading text content...
      </div>
    );
  }

  if (error) {
    return (
      <div className="flex flex-col items-center justify-center h-full gap-4">
        <span className="text-red-400 font-medium">Failed to load text: {error}</span>
      </div>
    );
  }

  return (
    <div className="flex flex-col h-full">
      <div className="sticky top-0 z-50 flex items-center justify-center gap-4 bg-zinc-900/90 backdrop-blur px-4 py-2 border-b border-zinc-800">
        <button onClick={() => setFontSize(s => Math.max(12, s - 2))}
          className="text-zinc-400 hover:text-white text-sm px-2">A-</button>
        <span className="text-zinc-300 font-mono text-xs">{fontSize}px</span>
        <button onClick={() => setFontSize(s => Math.min(32, s + 2))}
          className="text-zinc-400 hover:text-white text-sm px-2">A+</button>
      </div>
      <div className="flex-1 overflow-y-auto p-6">
        <pre className="whitespace-pre-wrap font-sans leading-relaxed text-zinc-200 max-w-3xl mx-auto"
             style={{ fontSize: `${fontSize}px` }}>
          {content}
        </pre>
      </div>
    </div>
  );
}