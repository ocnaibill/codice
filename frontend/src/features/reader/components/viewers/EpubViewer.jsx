import React, { useState, useEffect, useRef } from 'react';
import { ReactReader } from 'react-reader';
import { api, authenticatedUrl } from '../../../../lib/api';

export default function EpubViewer({ fileUrl, bookId, initialProgress }) {
  const [location, setLocation] = useState(initialProgress || null);
  const [size, setSize] = useState(100);
  const [theme, setTheme] = useState('dark');

  const renditionRef = useRef(null);
  const timeoutRef = useRef(null);

  // Inject themes into EPUB iframe whenever theme state updates
  useEffect(() => {
    if (renditionRef.current) {
      const rendition = renditionRef.current;

      const themes = {
        light: {
          body: { background: '#f4f4f5', color: '#18181b' },
          a: { color: '#2563eb' }
        },
        dark: {
          body: { background: '#09090b', color: '#a1a1aa' },
          a: { color: '#3b82f6' }
        }
      };

      rendition.themes.register(theme, themes[theme]);
      rendition.themes.select(theme);
    }
  }, [theme]);

  // Adjust font size dynamically in real time
  const changeSize = (newSize) => {
    setSize(newSize);
    if (renditionRef.current) {
      renditionRef.current.themes.fontSize(`${newSize}%`);
    }
  };

  const handleLocationChange = (epubcfi) => {
    setLocation(epubcfi);

    // Debounce: reset previous timer if user navigated quickly
    if (timeoutRef.current) {
      clearTimeout(timeoutRef.current);
    }

    // Set 1-second debounce timer before sending PATCH to backend
    if (bookId) {
      timeoutRef.current = setTimeout(() => {
        api.patch(`/works/${bookId}/progress`, { progress: epubcfi })
          .catch((err) => console.error("Failed to save reading progress:", err));
      }, 1000);
    }
  };

  useEffect(() => {
    return () => {
      if (timeoutRef.current) {
        clearTimeout(timeoutRef.current);
      }
    };
  }, []);

  return (
    <div className="flex flex-col items-center min-h-full w-full">
      {/* Floating Reader Toolbar */}
      <div className="sticky top-0 z-50 flex items-center justify-between bg-zinc-900/90 backdrop-blur px-6 py-3 rounded-full mb-6 border border-zinc-800 shadow-xl w-full max-w-md">
        
        {/* Font Size Controls */}
        <div className="flex items-center gap-4">
          <button 
            onClick={() => changeSize(Math.max(80, size - 10))}
            className="text-zinc-400 hover:text-white font-medium px-2 transition-colors"
            title="Decrease font size"
          >
            A-
          </button>
          <span className="text-zinc-300 font-mono text-xs w-10 text-center font-medium">
            {size}%
          </span>
          <button 
            onClick={() => changeSize(Math.min(200, size + 10))}
            className="text-zinc-400 hover:text-white font-medium px-2 text-lg transition-colors"
            title="Increase font size"
          >
            A+
          </button>
        </div>

        <div className="w-px h-6 bg-zinc-800" />

        {/* Theme Toggle Button */}
        <button
          onClick={() => setTheme(theme === 'dark' ? 'light' : 'dark')}
          className="text-zinc-400 hover:text-white text-xs font-medium px-2 flex items-center gap-2 transition-colors"
        >
          {theme === 'dark' ? '☀️ Light Mode' : '🌙 Dark Mode'}
        </button>
      </div>

      {/* Reader Engine Viewport Container */}
      <div className={`w-full max-w-4xl h-[75vh] md:h-[80vh] shadow-2xl rounded-md overflow-hidden border transition-colors duration-300 ${
        theme === 'dark' ? 'border-zinc-800 bg-zinc-950' : 'border-zinc-300 bg-zinc-100'
      }`}>
        <ReactReader
          url={`${authenticatedUrl(fileUrl)}&dummy=.epub`}
          location={location}
          locationChanged={handleLocationChange}
          title="Códice"
          getRendition={(rendition) => {
            renditionRef.current = rendition;
            rendition.themes.fontSize(`${size}%`);
          }}
          epubOptions={{
            flow: 'paginated',
            manager: 'default',
          }}
        />
      </div>
    </div>
  );
}
