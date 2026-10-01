import React, { useState, useEffect, useRef } from 'react';
import { Document, Page, pdfjs } from 'react-pdf';
import { authenticatedUrl } from '../../../../lib/api';
import { completionFor } from '../../progressRules';
import { pdfPlaceProblem } from '../../placeCheck';
import { loadOutline } from '../../pdfOutline';

import 'react-pdf/dist/Page/AnnotationLayer.css';
import 'react-pdf/dist/Page/TextLayer.css';

// Vite worker configuration for PDF.js
pdfjs.GlobalWorkerOptions.workerSrc = new URL(
  'pdfjs-dist/build/pdf.worker.min.mjs',
  import.meta.url,
).toString();

export default function PdfViewer({ fileUrl, onProgress, initialProgress, onPlaceFailed }) {
  const [numPages, setNumPages] = useState(null);
  const [pageNumber, setPageNumber] = useState(initialProgress ? parseInt(initialProgress, 10) || 1 : 1);
  const [outline, setOutline] = useState([]);
  const [showOutline, setShowOutline] = useState(false);
  const [jump, setJump] = useState('');
  const timeoutRef = useRef(null);

  function onDocumentLoadSuccess(pdf) {
    setNumPages(pdf.numPages);
    // A page the file does not have is said, not hidden by opening the first one in silence.
    const problem = pdfPlaceProblem(initialProgress, pdf.numPages);
    if (problem) {
      setPageNumber(1);
      onPlaceFailed?.({ reason: problem });
    }
    loadOutline(pdf).then(setOutline).catch(() => setOutline([]));
  }

  // Debounced progress saving when pageNumber changes
  const changePage = (newPage) => {
    setPageNumber(newPage);

    if (timeoutRef.current) {
      clearTimeout(timeoutRef.current);
    }

    if (onProgress) {
      timeoutRef.current = setTimeout(() => {
        const percent = numPages ? (newPage / numPages) * 100 : undefined;
        const completed = completionFor(percent);
        onProgress({ type: 'pdf', page: newPage - 1 }, { percent, completed })
          .catch((err) => console.error("Failed to save PDF reading progress:", err));
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

  const prevPage = () => {
    if (pageNumber > 1) {
      changePage(pageNumber - 1);
    }
  };

  const nextPage = () => {
    if (pageNumber < numPages) {
      changePage(pageNumber + 1);
    }
  };

  const goTo = (page) => {
    if (Number.isInteger(page) && page >= 1 && (!numPages || page <= numPages)) changePage(page);
  };

  return (
    <div className="flex flex-col items-center justify-center min-h-full">
      <div className="sticky top-0 z-50 mb-6 flex w-full max-w-md flex-col items-center gap-2">
      {/* Floating Pagination Controls */}
      <div className="flex items-center gap-4 bg-zinc-900/90 backdrop-blur px-4 py-2 rounded-full border border-zinc-800 shadow-xl">
        <button 
          onClick={prevPage} 
          disabled={pageNumber <= 1}
          className="text-zinc-400 hover:text-zinc-100 disabled:opacity-30 px-2 font-bold text-xl transition-colors"
        >
          ←
        </button>
        <span className="text-zinc-300 text-sm font-medium font-mono">
          {pageNumber} / {numPages || '-'}
        </span>
        <button 
          onClick={nextPage} 
          disabled={pageNumber >= numPages}
          className="text-zinc-400 hover:text-zinc-100 disabled:opacity-30 px-2 font-bold text-xl transition-colors"
        >
          →
        </button>
        <input
          type="number"
          min={1}
          max={numPages || undefined}
          value={jump}
          onChange={(e) => setJump(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter') {
              goTo(parseInt(jump, 10));
              setJump('');
            }
          }}
          placeholder="ir a…"
          aria-label="Ir para a página"
          className="w-16 rounded bg-zinc-800 px-2 py-1 text-center text-xs text-zinc-200 placeholder:text-zinc-500"
        />
        {outline.length > 0 && (
          <button
            onClick={() => setShowOutline((v) => !v)}
            aria-expanded={showOutline}
            className="text-zinc-400 hover:text-zinc-100 text-xs font-medium px-2 transition-colors"
            title="Sumário do PDF"
          >
            ☰ Sumário
          </button>
        )}
      </div>

      {showOutline && outline.length > 0 && (
        <nav aria-label="Sumário do PDF" className="max-h-72 w-full overflow-y-auto rounded-lg border border-zinc-800 bg-zinc-900/95 p-2 shadow-2xl backdrop-blur">
          {outline.map((entry, i) => (
            <button
              key={`${i}-${entry.page}`}
              onClick={() => {
                goTo(entry.page);
                setShowOutline(false);
              }}
              style={{ paddingLeft: `${0.75 + entry.depth * 1}rem` }}
              className="flex w-full items-baseline justify-between gap-3 rounded-md py-2 pr-3 text-left text-sm text-zinc-400 hover:bg-zinc-800/60 hover:text-white"
              title={entry.title}
            >
              <span className="truncate">{entry.title}</span>
              <span className="shrink-0 font-mono text-[11px] text-zinc-500">{entry.page}</span>
            </button>
          ))}
        </nav>
      )}
      </div>

      {/* PDF Page Rendering Canvas */}
      <div className="shadow-2xl rounded-sm overflow-hidden border border-zinc-800 bg-white">
        <Document
          file={authenticatedUrl(fileUrl)}
          onLoadSuccess={onDocumentLoadSuccess}
          loading={<div className="p-20 text-zinc-500 animate-pulse">Rendering PDF...</div>}
        >
          <Page 
            pageNumber={pageNumber} 
            renderTextLayer={true}
            renderAnnotationLayer={false}
            width={Math.min(window.innerWidth * 0.9, 800)}
          />
        </Document>
      </div>
    </div>
  );
}
