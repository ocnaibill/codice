import React, { useState, useEffect, useRef, useCallback } from 'react';
import { authenticatedUrl } from '../../../../lib/api';

// Number of pages to preload ahead and behind
const PRELOAD_COUNT = 2;
// Loading timeout per page in ms
const PAGE_TIMEOUT = 30000;

export default function MangaViewer({ fileUrl, bookId, initialProgress, workId }) {
  const [pages, setPages] = useState([]);
  const [currentPage, setCurrentPage] = useState(
    initialProgress ? parseInt(initialProgress, 10) || 0 : 0
  );
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [readingDirection, setReadingDirection] = useState('ltr'); // 'ltr', 'rtl', 'webtoon', 'double'
  const [pageStatus, setPageStatus] = useState({}); // { [pageNum]: 'loading'|'loaded'|'error' }
  const timeoutRef = useRef(null);
  const containerRef = useRef(null);

  // Fetch page list from backend
  useEffect(() => {
    let cancelled = false;

    const loadPageList = async () => {
      try {
        // Derive work ID from fileUrl: /files/<workId>/...
        // Fallback: use workId prop or extract from fileUrl
        const id = workId || (fileUrl ? fileUrl.split('/').filter(Boolean)[1] : null);
        if (!id) {
          setError('Cannot determine work ID');
          setLoading(false);
          return;
        }

        const url = authenticatedUrl(`/works/${id}/pages`);
        const response = await fetch(url);
        if (!response.ok) {
          throw new Error(`Failed to load page list: ${response.status}`);
        }
        const data = await response.json();
        
        if (!cancelled) {
          setPages(data);
          setLoading(false);
        }
      } catch (err) {
        if (!cancelled) {
          setError(err.message);
          setLoading(false);
        }
      }
    };

    loadPageList();
    return () => { cancelled = true; };
  }, [fileUrl, workId]);

  // Derive work ID from fileUrl or workId prop
  const workIdValue = workId || (fileUrl ? fileUrl.split('/').filter(Boolean)[1] : null);

  // Get authenticated URL for a specific page
  const getPageUrl = useCallback((pageNum) => {
    if (!workIdValue) return null;
    return authenticatedUrl(`/works/${workIdValue}/pages/${pageNum}`);
  }, [workIdValue]);

  // Get thumbnail URL for a specific page
  const getThumbnailUrl = useCallback((pageNum) => {
    if (!workIdValue) return null;
    return authenticatedUrl(`/works/${workIdValue}/pages/${pageNum}/thumbnail`);
  }, [workIdValue]);

  // Preload adjacent pages
  useEffect(() => {
    const toPreload = [];
    for (let i = currentPage - PRELOAD_COUNT; i <= currentPage + PRELOAD_COUNT; i++) {
      if (i >= 0 && i < pages.length && !pageStatus[i]) {
        toPreload.push(i);
      }
    }

    toPreload.forEach((pageNum) => {
      setPageStatus((prev) => ({ ...prev, [pageNum]: 'loading' }));
      const img = new Image();
      const timer = setTimeout(() => {
        setPageStatus((prev) => ({ ...prev, [pageNum]: 'error' }));
      }, PAGE_TIMEOUT);

      img.onload = () => {
        clearTimeout(timer);
        setPageStatus((prev) => ({ ...prev, [pageNum]: 'loaded' }));
      };
      img.onerror = () => {
        clearTimeout(timer);
        setPageStatus((prev) => ({ ...prev, [pageNum]: 'error' }));
      };
      img.src = getPageUrl(pageNum);
    });
  }, [currentPage, pages.length, getPageUrl, pageStatus]);

  // Debounced progress saving when currentPage changes
  const changePage = useCallback((newPage) => {
    const clampedPage = Math.max(0, Math.min(newPage, pages.length - 1));
    setCurrentPage(clampedPage);

    if (timeoutRef.current) {
      clearTimeout(timeoutRef.current);
    }

    if (bookId) {
      timeoutRef.current = setTimeout(() => {
        import('../../../../lib/api').then(({ api }) => {
          api.patch(`/works/${bookId}/progress`, { progress: clampedPage.toString() })
            .catch((err) => console.error('Failed to save reading progress:', err));
        });
      }, 1000);
    }
  }, [bookId, pages.length]);

  useEffect(() => {
    return () => {
      if (timeoutRef.current) clearTimeout(timeoutRef.current);
    };
  }, []);

  // Keyboard navigation
  useEffect(() => {
    const handleKeyDown = (e) => {
      if (readingDirection === 'webtoon') {
        if (e.key === 'ArrowDown') changePage(currentPage + 1);
        if (e.key === 'ArrowUp') changePage(currentPage - 1);
      } else if (readingDirection === 'rtl') {
        if (e.key === 'ArrowLeft') changePage(currentPage + 1);
        if (e.key === 'ArrowRight') changePage(currentPage - 1);
      } else {
        if (e.key === 'ArrowRight') changePage(currentPage + 1);
        if (e.key === 'ArrowLeft') changePage(currentPage - 1);
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [currentPage, readingDirection, changePage]);

  // Scroll detection for webtoon mode
  useEffect(() => {
    if (readingDirection !== 'webtoon' || !containerRef.current) return;

    const container = containerRef.current;
    const handleScroll = () => {
      const scrollBottom = container.scrollHeight - container.scrollTop - container.clientHeight;
      if (scrollBottom < 200) {
        // Near bottom, load more pages
        const visiblePages = Math.ceil(container.scrollTop / container.clientHeight) + 2;
        setCurrentPage(Math.min(visiblePages, pages.length - 1));
      }
    };

    container.addEventListener('scroll', handleScroll);
    return () => container.removeEventListener('scroll', handleScroll);
  }, [readingDirection, pages.length]);

  const prevPage = () => {
    if (readingDirection === 'rtl') {
      changePage(currentPage + 1);
    } else {
      changePage(currentPage - 1);
    }
  };

  const nextPage = () => {
    if (readingDirection === 'rtl') {
      changePage(currentPage - 1);
    } else {
      changePage(currentPage + 1);
    }
  };

  const cycleDirection = () => {
    const modes = ['ltr', 'rtl', 'webtoon', 'double'];
    const idx = modes.indexOf(readingDirection);
    setReadingDirection(modes[(idx + 1) % modes.length]);
  };

  if (loading) {
    return (
      <div className="flex flex-col items-center justify-center h-full gap-4 p-10">
        <div className="w-8 h-8 border-2 border-zinc-600 border-t-zinc-300 rounded-full animate-spin" />
        <span className="text-zinc-500 animate-pulse font-medium">
          Loading page list...
        </span>
      </div>
    );
  }

  if (error) {
    return (
      <div className="flex flex-col items-center justify-center h-full gap-4 p-10">
        <span className="text-red-400 font-medium">Failed to load comic: {error}</span>
      </div>
    );
  }

  if (pages.length === 0) {
    return (
      <div className="text-red-400 flex h-full items-center justify-center font-medium">
        No valid image pages found in this comic archive.
      </div>
    );
  }

  // Webtoon mode: render all pages in a scrollable column
  if (readingDirection === 'webtoon') {
    return (
      <div className="flex flex-col h-full">
        {/* Controls Bar */}
        <div className="sticky top-0 z-50 flex items-center justify-center gap-4 bg-zinc-900/90 backdrop-blur px-4 py-2 border-b border-zinc-800">
          <button onClick={cycleDirection} className="text-xs text-zinc-400 hover:text-white px-2 py-1 bg-zinc-800 rounded">
            {readingDirection.toUpperCase()}
          </button>
          <span className="text-zinc-300 font-mono text-sm font-medium">
            Scroll — {pages.length} pages
          </span>
          <button onClick={() => setReadingDirection('ltr')} className="text-xs text-zinc-400 hover:text-white px-2 py-1 bg-zinc-800 rounded">
            EXIT
          </button>
        </div>

        <div ref={containerRef} className="flex-1 overflow-y-auto">
          <div className="flex flex-col items-center gap-2 py-4">
            {pages.map((page, idx) => (
              <img
                key={idx}
                src={getPageUrl(idx)}
                alt={`Page ${idx + 1}`}
                className="max-w-full h-auto"
                loading={idx <= currentPage + PRELOAD_COUNT ? 'eager' : 'lazy'}
                onError={(e) => {
                  e.target.src = getThumbnailUrl(idx);
                }}
              />
            ))}
          </div>
        </div>
      </div>
    );
  }

  // Double-page spread mode
  if (readingDirection === 'double') {
    const leftPage = currentPage;
    const rightPage = currentPage + 1;
    const hasRight = rightPage < pages.length;

    return (
      <div className="flex flex-col h-full">
        {/* Controls Bar */}
        <div className="sticky top-0 z-50 flex items-center justify-center gap-4 bg-zinc-900/90 backdrop-blur px-4 py-2 border-b border-zinc-800">
          <button onClick={cycleDirection} className="text-xs text-zinc-400 hover:text-white px-2 py-1 bg-zinc-800 rounded">
            {readingDirection.toUpperCase()}
          </button>
          <button onClick={() => changePage(currentPage - 2)} disabled={currentPage <= 0}
            className="text-zinc-400 hover:text-white disabled:opacity-30 text-xl font-bold">
            ←
          </button>
          <span className="text-zinc-300 font-mono text-sm font-medium">
            {leftPage + 1}{hasRight ? `-${rightPage + 1}` : ''} / {pages.length}
          </span>
          <button onClick={() => changePage(currentPage + 2)} disabled={!hasRight}
            className="text-zinc-400 hover:text-white disabled:opacity-30 text-xl font-bold">
            →
          </button>
        </div>

        <div className="flex-1 flex items-center justify-center gap-1 p-4">
          <img
            src={getPageUrl(leftPage)}
            alt={`Page ${leftPage + 1}`}
            className="max-h-[85vh] max-w-[50%] object-contain cursor-pointer"
            onClick={() => changePage(currentPage + 2)}
          />
          {hasRight && (
            <img
              src={getPageUrl(rightPage)}
              alt={`Page ${rightPage + 1}`}
              className="max-h-[85vh] max-w-[50%] object-contain cursor-pointer"
              onClick={() => changePage(currentPage + 2)}
            />
          )}
        </div>
      </div>
    );
  }

  // LTR / RTL single-page mode
  return (
    <div className="flex flex-col h-full">
      {/* Controls Bar */}
      <div className="sticky top-0 z-50 flex items-center justify-center gap-4 bg-zinc-900/90 backdrop-blur px-4 py-2 border-b border-zinc-800">
        <button onClick={cycleDirection} className="text-xs text-zinc-400 hover:text-white px-2 py-1 bg-zinc-800 rounded">
          {readingDirection.toUpperCase()}
        </button>
        <button 
          onClick={prevPage} 
          disabled={readingDirection === 'rtl' ? currentPage >= pages.length - 1 : currentPage <= 0}
          className="text-zinc-400 hover:text-white disabled:opacity-30 text-xl font-bold"
        >
          ←
        </button>
        <span className="text-zinc-300 font-mono text-sm font-medium">
          {currentPage + 1} / {pages.length}
        </span>
        <button 
          onClick={nextPage} 
          disabled={readingDirection === 'rtl' ? currentPage <= 0 : currentPage >= pages.length - 1}
          className="text-zinc-400 hover:text-white disabled:opacity-30 text-xl font-bold"
        >
          →
        </button>
      </div>

      {/* Single Page Viewport */}
      <div className="flex-1 flex items-center justify-center p-4">
        {pageStatus[currentPage] === 'error' ? (
          <div className="flex flex-col items-center gap-2">
            <span className="text-red-400">Failed to load page</span>
            <button 
              onClick={() => {
                setPageStatus((prev) => ({ ...prev, [currentPage]: 'loading' }));
                const img = new Image();
                img.onload = () => setPageStatus((prev) => ({ ...prev, [currentPage]: 'loaded' }));
                img.onerror = () => setPageStatus((prev) => ({ ...prev, [currentPage]: 'error' }));
                img.src = getPageUrl(currentPage);
              }}
              className="text-sm bg-zinc-800 border border-zinc-700 text-zinc-300 px-3 py-1 rounded hover:bg-zinc-700"
            >
              Retry
            </button>
          </div>
        ) : (
          <img
            src={getPageUrl(currentPage)}
            alt={`Page ${currentPage + 1}`}
            className="max-h-[85vh] max-w-full object-contain cursor-pointer"
            onClick={nextPage}
            title="Click to advance page"
            onError={(e) => {
              setPageStatus((prev) => ({ ...prev, [currentPage]: 'error' }));
              // Fallback to thumbnail
              e.target.src = getThumbnailUrl(currentPage);
            }}
            onLoad={() => setPageStatus((prev) => ({ ...prev, [currentPage]: 'loaded' }))}
          />
        )}
      </div>
    </div>
  );
}