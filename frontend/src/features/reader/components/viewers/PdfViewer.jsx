import React, { useState } from 'react';
import { Document, Page, pdfjs } from 'react-pdf';

import 'react-pdf/dist/Page/AnnotationLayer.css';
import 'react-pdf/dist/Page/TextLayer.css';

// Vite worker configuration for PDF.js
pdfjs.GlobalWorkerOptions.workerSrc = new URL(
  'pdfjs-dist/build/pdf.worker.min.mjs',
  import.meta.url,
).toString();

export default function PdfViewer({ fileUrl }) {
  const [numPages, setNumPages] = useState(null);
  const [pageNumber, setPageNumber] = useState(1);

  function onDocumentLoadSuccess({ numPages }) {
    setNumPages(numPages);
  }

  const prevPage = () => setPageNumber((prev) => (prev > 1 ? prev - 1 : prev));
  const nextPage = () => setPageNumber((prev) => (prev < numPages ? prev + 1 : prev));

  return (
    <div className="flex flex-col items-center justify-center min-h-full">
      {/* Floating Pagination Controls */}
      <div className="sticky top-0 z-50 flex items-center gap-4 bg-zinc-900/90 backdrop-blur px-4 py-2 rounded-full mb-6 border border-zinc-800 shadow-xl">
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
      </div>

      {/* PDF Page Rendering Canvas */}
      <div className="shadow-2xl rounded-sm overflow-hidden border border-zinc-800 bg-white">
        <Document
          file={fileUrl}
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
