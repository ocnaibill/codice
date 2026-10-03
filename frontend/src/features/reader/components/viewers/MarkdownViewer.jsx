import React from 'react';
import MarkdownContent from './MarkdownContent';
import ReadingSurface from './ReadingSurface';
import { useTextFile } from './useTextFile';

/** A Markdown file, read by scrolling, in the page color, the font and the size the person chose. */
export default function MarkdownViewer({ fileUrl, onProgress, initialProgress, immersive, onImmersiveChange, onSelection }) {
  const { content, loading, error, retry } = useTextFile(fileUrl);
  return (
    <ReadingSurface
      content={content}
      loading={loading}
      error={error}
      onRetry={retry}
      loadingLabel="Carregando o texto"
      errorTitle="Não foi possível abrir este arquivo."
      emptyText="Este arquivo está vazio."
      immersive={immersive}
      onImmersiveChange={onImmersiveChange}
      onProgress={onProgress}
      initialProgress={initialProgress}
      onSelection={onSelection}
    >
      <MarkdownContent>{content}</MarkdownContent>
    </ReadingSurface>
  );
}
