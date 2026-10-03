import React from 'react';
import ReadingSurface from './ReadingSurface';
import { useTextFile } from './useTextFile';

/** A plain text file, read by scrolling, in the page color, the font and the size the person chose. */
export default function TextViewer({ fileUrl, onProgress, initialProgress, immersive, onImmersiveChange, onSelection }) {
  const { content, loading, error, retry } = useTextFile(fileUrl);
  return (
    <ReadingSurface
      content={content}
      loading={loading}
      error={error}
      onRetry={retry}
      loadingLabel="Carregando o texto"
      errorTitle="Não foi possível abrir este texto."
      emptyText="Este arquivo está vazio."
      immersive={immersive}
      onImmersiveChange={onImmersiveChange}
      onProgress={onProgress}
      initialProgress={initialProgress}
      onSelection={onSelection}
    >
      <pre className="whitespace-pre-wrap break-words [font:inherit]">{content}</pre>
    </ReadingSurface>
  );
}
