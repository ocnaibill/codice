import React, { useState } from 'react';
import { ReactReader } from 'react-reader';

export default function EpubViewer({ fileUrl }) {
  // epub.js uses EPUB Canonical Fragment Identifier (CFI) to track reading progress location
  const [location, setLocation] = useState(null);

  return (
    <div className="flex flex-col items-center justify-center min-h-full w-full">
      <div className="w-full max-w-4xl h-[75vh] md:h-[80vh] shadow-2xl rounded-md overflow-hidden border border-zinc-800 bg-zinc-50 relative">
        <ReactReader
          url={fileUrl}
          location={location}
          locationChanged={(epubcfi) => setLocation(epubcfi)}
          title="Códice" 
          tocChanged={(toc) => console.log('Table of contents loaded:', toc)}
          epubOptions={{
            flow: 'paginated',
            manager: 'default',
          }}
        />
      </div>
    </div>
  );
}
