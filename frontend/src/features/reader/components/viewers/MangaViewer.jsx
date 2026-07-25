import React, { useState, useEffect, useRef } from 'react';
import JSZip from 'jszip';
import { api } from '../../../../lib/api';

export default function MangaViewer({ fileUrl, bookId, initialProgress }) {
  const [pages, setPages] = useState([]);
  const [currentPage, setCurrentPage] = useState(initialProgress ? parseInt(initialProgress, 10) || 0 : 0);
  const [loading, setLoading] = useState(true);
  const timeoutRef = useRef(null);

  useEffect(() => {
    let createdUrls = [];

    const loadComic = async () => {
      try {
        // Fetch .cbz archive from backend
        const response = await fetch(fileUrl);
        const blob = await response.blob();
        
        // Decompress ZIP archive in browser memory
        const zip = await JSZip.loadAsync(blob);
        
        // Filter image files and sort alphabetically (001.jpg, 002.jpg...)
        const imageFiles = Object.values(zip.files).filter(file => 
          !file.dir && file.name.match(/\.(jpg|jpeg|png|webp)$/i)
        );
        imageFiles.sort((a, b) => a.name.localeCompare(b.name, undefined, { numeric: true, sensitivity: 'base' }));
        
        // Convert binary blobs into temporary object URLs
        createdUrls = await Promise.all(imageFiles.map(async (file) => {
          const fileData = await file.async('blob');
          return URL.createObjectURL(fileData);
        }));
        
        setPages(createdUrls);
        setLoading(false);
      } catch (error) {
        console.error("Error loading CBZ manga archive:", error);
        setLoading(false);
      }
    };
    
    loadComic();
    
    // Revoke object URLs on component unmount to prevent RAM memory leaks
    return () => {
      createdUrls.forEach(url => URL.revokeObjectURL(url));
    };
  }, [fileUrl]);

  // Debounced progress saving when currentPage changes
  const changePage = (newPage) => {
    setCurrentPage(newPage);

    if (timeoutRef.current) {
      clearTimeout(timeoutRef.current);
    }

    if (bookId) {
      timeoutRef.current = setTimeout(() => {
        api.patch(`/works/${bookId}/progress`, { progress: newPage.toString() })
          .catch((err) => console.error("Failed to save Manga reading progress:", err));
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
    if (currentPage > 0) {
      changePage(currentPage - 1);
    }
  };

  const nextPage = () => {
    if (currentPage < pages.length - 1) {
      changePage(currentPage + 1);
    }
  };

  if (loading) {
    return (
      <div className="text-zinc-500 flex h-full items-center justify-center animate-pulse font-medium">
        Decompressing high-quality comic pages...
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

  return (
    <div className="flex flex-col items-center min-h-full">
      {/* Floating Controls Bar */}
      <div className="sticky top-0 z-50 flex items-center gap-4 bg-zinc-900/90 backdrop-blur px-4 py-2 rounded-full mb-6 border border-zinc-800 shadow-xl">
        <button 
          onClick={prevPage} 
          disabled={currentPage === 0} 
          className="text-zinc-400 hover:text-white px-2 font-bold text-xl disabled:opacity-30 transition-colors"
        >
          ←
        </button>
        <span className="text-zinc-300 font-mono text-sm font-medium">
          {currentPage + 1} / {pages.length}
        </span>
        <button 
          onClick={nextPage} 
          disabled={currentPage === pages.length - 1} 
          className="text-zinc-400 hover:text-white px-2 font-bold text-xl disabled:opacity-30 transition-colors"
        >
          →
        </button>
      </div>
      
      {/* Page Canvas Viewport */}
      <div 
        className="max-w-4xl w-full shadow-2xl rounded-sm overflow-hidden bg-black flex justify-center cursor-pointer border border-zinc-800" 
        onClick={nextPage}
        title="Click to advance page"
      >
        <img 
          src={pages[currentPage]} 
          alt={`Page ${currentPage + 1}`} 
          className="max-h-[85vh] object-contain"
        />
      </div>
    </div>
  );
}
