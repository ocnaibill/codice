import React, { useState, useEffect, useRef, useCallback } from 'react';
import ePub from 'epubjs';
import { api } from '../../../../lib/api';

/**
 * EPUB Viewer using epubjs directly (not react-reader).
 *
 * react-reader wraps epubjs in a class component whose lifecycle / internal
 * queue has timing issues that produce "No Section Found" errors when
 * displaying sections whose TOC-hrefs don't match the OPF-relative spine
 * hrefs.  By driving epubjs ourselves we side-step all of that.
 */
export default function EpubViewer({ fileUrl, bookId, initialProgress }) {
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [toc, setToc] = useState([]);
  const [showToc, setShowToc] = useState(false);
  const [size, setSize] = useState(100);
  const [theme, setTheme] = useState('dark');
  const [currentSection, setCurrentSection] = useState('');

  const viewerRef = useRef(null);
  const bookRef = useRef(null);
  const renditionRef = useRef(null);
  const timeoutRef = useRef(null);

  // ── Helpers ──────────────────────────────────────────────────────

  /**
   * Find a spine Section whose href matches `rawHref` (which may be
   * relative to nav.xhtml, e.g. "chapter03.xhtml").  The spine stores
   * hrefs relative to the OPF (e.g. "Text/chapter03.xhtml"), so a
   * simple suffix match is needed.
   */
  const resolveToSpine = useCallback((book, rawHref) => {
    if (!book || !rawHref) return null;
    const clean = rawHref.split('#')[0];
    // 1) Direct lookup
    let section = book.spine.get(clean);
    if (section) return section;
    // 2) Suffix match against spine items
    section = book.spine.spineItems.find(
      (s) => s.href === clean || s.href.endsWith('/' + clean),
    );
    return section || null;
  }, []);

  // ── Theme helpers ────────────────────────────────────────────────
  const applyTheme = useCallback((rendition, t) => {
    if (!rendition) return;
    const themes = {
      light: {
        body: { background: '#f4f4f5', color: '#18181b' },
        a: { color: '#2563eb' },
      },
      dark: {
        body: { background: '#09090b', color: '#a1a1aa' },
        a: { color: '#3b82f6' },
      },
    };
    rendition.themes.register('light', themes.light);
    rendition.themes.register('dark', themes.dark);
    rendition.themes.select(t);
  }, []);

  // ── Initialise book + rendition ──────────────────────────────────
  useEffect(() => {
    let cancelled = false;
    let book = null;
    let rendition = null;

    async function init() {
      try {
        setLoading(true);
        setError(null);

        // 1. Fetch EPUB binary via authenticated axios request
        const response = await api.get(fileUrl, {
          responseType: 'arraybuffer',
          timeout: 60000,
        });
        if (cancelled) return;

        // 2. Create book – pass ArrayBuffer + force openAs 'binary'
        book = ePub(response.data, { openAs: 'binary' });
        bookRef.current = book;

        // 3. Wait for navigation (TOC) to be ready
        const nav = await book.loaded.navigation;
        if (cancelled) return;
        setToc(nav.toc || []);

        // 4. Wait for book to be fully opened (spine + replacements done)
        await book.opened;
        if (cancelled) return;

        // DEBUG – keep this until the viewer is stable
        console.log('[EpubViewer] Book opened');
        console.log('[EpubViewer] Spine items:', book.spine.spineItems.length);
        if (book.spine.spineItems.length > 0) {
          console.log('[EpubViewer] First spine href:', book.spine.spineItems[0].href);
        }

        // 5. Ensure the container element is available
        if (!viewerRef.current) {
          throw new Error('Viewer container not mounted');
        }

        // 6. Create rendition — book.opened already resolved so the
        //    internal queue processes immediately.
        rendition = book.renderTo(viewerRef.current, {
          width: '100%',
          height: '100%',
          flow: 'paginated',
          manager: 'default',
          allowScriptedContent: true,
        });
        renditionRef.current = rendition;

        // 7. Themes + font size
        applyTheme(rendition, 'dark');
        rendition.themes.fontSize(`${size}%`);

        // 8. Track location changes
        rendition.on('relocated', (location) => {
          if (!location || !location.start) return;
          const cfi = location.start.cfi;
          setCurrentSection(cfi);

          // Debounced progress save
          if (timeoutRef.current) clearTimeout(timeoutRef.current);
          if (bookId) {
            timeoutRef.current = setTimeout(() => {
              api
                .patch(`/works/${bookId}/progress`, { progress: cfi })
                .catch((err) => console.error('Failed to save progress:', err));
            }, 1000);
          }
        });

        // 9. Display the initial location
        //    - CFI strings ("epubcfi(...)") go straight to display()
        //    - Raw hrefs from old progress data need resolving through
        //      the spine because TOC hrefs and spine hrefs may differ
        //    - No progress → display() with no args = first linear item
        try {
          if (initialProgress && initialProgress.startsWith('epubcfi(')) {
            console.log('[EpubViewer] Displaying CFI:', initialProgress);
            await rendition.display(initialProgress);
          } else if (initialProgress) {
            // Legacy raw-href progress – resolve to spine
            const section = resolveToSpine(book, initialProgress);
            if (section) {
              console.log('[EpubViewer] Resolved progress to spine href:', section.href);
              await rendition.display(section.href);
            } else {
              console.warn('[EpubViewer] Could not resolve saved progress, showing first page');
              await rendition.display();
            }
          } else {
            console.log('[EpubViewer] No saved progress, displaying first section');
            await rendition.display();
          }
        } catch (displayErr) {
          console.warn('[EpubViewer] Display failed, trying first spine item:', displayErr);
          try {
            const firstHref = book.spine.spineItems[0]?.href;
            if (firstHref) {
              await rendition.display(firstHref);
            } else {
              throw displayErr;
            }
          } catch (fallbackErr) {
            console.error('[EpubViewer] Fallback also failed:', fallbackErr);
            throw fallbackErr;
          }
        }

        if (!cancelled) setLoading(false);
      } catch (err) {
        if (!cancelled) {
          console.error('[EpubViewer] init error:', err);
          setError(`Failed to load EPUB: ${err.message}`);
          setLoading(false);
        }
      }
    }

    init();

    return () => {
      cancelled = true;
      if (rendition) {
        rendition.destroy();
      }
      if (book) {
        book.destroy();
      }
      bookRef.current = null;
      renditionRef.current = null;
      if (timeoutRef.current) clearTimeout(timeoutRef.current);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [fileUrl]);

  // ── Theme changes (re-apply on toggle) ────────────────────────
  useEffect(() => {
    applyTheme(renditionRef.current, theme);
  }, [theme, applyTheme]);

  // ── Font size ────────────────────────────────────────────────────
  const changeSize = (newSize) => {
    setSize(newSize);
    if (renditionRef.current) {
      renditionRef.current.themes.fontSize(`${newSize}%`);
    }
  };

  // ── Navigation ──────────────────────────────────────────────────
  const prevPage = () => renditionRef.current?.prev();
  const nextPage = () => renditionRef.current?.next();

  const goToTocItem = (href) => {
    if (!renditionRef.current || !bookRef.current) return;

    // TOC hrefs are relative to nav.xhtml (e.g. "chapter03.xhtml#ch3")
    // but spine hrefs are relative to the OPF (e.g. "Text/chapter03.xhtml").
    // Resolve via suffix matching.
    const section = resolveToSpine(bookRef.current, href);
    if (section) {
      renditionRef.current.display(section.href);
    } else {
      // Last resort: pass the raw href and hope epubjs can figure it out
      renditionRef.current.display(href);
    }
    setShowToc(false);
  };

  // ── Keyboard navigation ───────────────────────────────────────
  useEffect(() => {
    const handleKeyUp = (e) => {
      if (e.key === 'ArrowLeft') prevPage();
      if (e.key === 'ArrowRight') nextPage();
    };
    document.addEventListener('keyup', handleKeyUp);
    return () => document.removeEventListener('keyup', handleKeyUp);
  }, []);

  // ── Cleanup debounce timer ────────────────────────────────────
  useEffect(() => {
    return () => {
      if (timeoutRef.current) clearTimeout(timeoutRef.current);
    };
  }, []);

  // ── Render ───────────────────────────────────────────────────────
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

        {/* TOC Toggle */}
        <button
          onClick={() => setShowToc(!showToc)}
          className="text-zinc-400 hover:text-white text-xs font-medium px-2 flex items-center gap-1 transition-colors"
          title="Table of Contents"
        >
          ☰ TOC
        </button>

        <div className="w-px h-6 bg-zinc-800" />

        {/* Theme Toggle Button */}
        <button
          onClick={() => setTheme(theme === 'dark' ? 'light' : 'dark')}
          className="text-zinc-400 hover:text-white text-xs font-medium px-2 flex items-center gap-2 transition-colors"
        >
          {theme === 'dark' ? '☀️ Light' : '🌙 Dark'}
        </button>
      </div>

      {/* Loading State */}
      {loading && (
        <div className="flex items-center justify-center w-full max-w-4xl h-[75vh] md:h-[80vh] rounded-md border border-zinc-800 bg-zinc-950">
          <span className="text-zinc-500 animate-pulse font-medium">Loading EPUB file...</span>
        </div>
      )}

      {/* Error State */}
      {error && (
        <div className="flex flex-col items-center justify-center w-full max-w-4xl h-[75vh] md:h-[80vh] rounded-md border border-red-900 bg-zinc-950 gap-4">
          <span className="text-red-400 font-medium">{error}</span>
          <button
            onClick={() => window.location.reload()}
            className="px-4 py-2 bg-zinc-800 border border-zinc-700 text-zinc-300 rounded-md hover:bg-zinc-700"
          >
            Reload
          </button>
        </div>
      )}

      {/* Reader Container with optional TOC sidebar */}
      <div className="relative w-full max-w-4xl h-[75vh] md:h-[80vh]">
        {/* TOC Sidebar */}
        {showToc && toc.length > 0 && (
          <div className="absolute inset-y-0 left-0 w-64 z-40 bg-zinc-900/95 backdrop-blur border-r border-zinc-800 overflow-y-auto rounded-l-md shadow-2xl">
            <div className="p-4 border-b border-zinc-800">
              <h3 className="text-zinc-300 font-medium text-sm">Table of Contents</h3>
            </div>
            <nav className="p-2">
              {toc.map((item, i) => (
                <button
                  key={item.id || i}
                  onClick={() => goToTocItem(item.href)}
                  className="block w-full text-left px-3 py-2 text-sm text-zinc-400 hover:text-white hover:bg-zinc-800/60 rounded-md transition-colors truncate"
                  title={item.label}
                >
                  {item.label?.trim()}
                </button>
              ))}
            </nav>
          </div>
        )}

        {/* Prev / Next navigation overlays */}
        <button
          onClick={prevPage}
          className="absolute left-0 inset-y-0 z-30 w-16 flex items-center justify-center text-zinc-600 hover:text-zinc-300 hover:bg-zinc-900/30 transition-colors rounded-l-md"
          title="Previous page"
          aria-label="Previous page"
        >
          <span className="text-2xl">‹</span>
        </button>
        <button
          onClick={nextPage}
          className="absolute right-0 inset-y-0 z-30 w-16 flex items-center justify-center text-zinc-600 hover:text-zinc-300 hover:bg-zinc-900/30 transition-colors rounded-r-md"
          title="Next page"
          aria-label="Next page"
        >
          <span className="text-2xl">›</span>
        </button>

        {/* epubjs render target */}
        <div
          ref={viewerRef}
          className={`w-full h-full shadow-2xl rounded-md overflow-hidden border transition-colors duration-300 ${
            theme === 'dark'
              ? 'border-zinc-800 bg-zinc-950'
              : 'border-zinc-300 bg-zinc-100'
          }`}
        />
      </div>
    </div>
  );
}
