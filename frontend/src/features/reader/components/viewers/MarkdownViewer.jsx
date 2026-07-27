import React, { useState, useEffect } from 'react';

export default function MarkdownViewer({ fileUrl, bookId, initialProgress }) {
  const [content, setContent] = useState('');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  useEffect(() => {
    let cancelled = false;
    const loadMd = async () => {
      try {
        const resp = await fetch(fileUrl);
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
    loadMd();
    return () => { cancelled = true; };
  }, [fileUrl]);

  if (loading) {
    return (
      <div className="flex items-center justify-center h-full text-zinc-500 animate-pulse font-medium">
        Loading markdown...
      </div>
    );
  }

  if (error) {
    return (
      <div className="flex flex-col items-center justify-center h-full gap-4">
        <span className="text-red-400 font-medium">Failed to load: {error}</span>
      </div>
    );
  }

  // Simple markdown rendering (basic headers, bold, lists)
  const renderHtml = (md) => {
    let html = md
      // Headers
      .replace(/^### (.+)$/gm, '<h3 class="text-lg font-semibold text-zinc-100 mt-6 mb-2">$1</h3>')
      .replace(/^## (.+)$/gm, '<h2 class="text-xl font-semibold text-zinc-100 mt-8 mb-3">$1</h2>')
      .replace(/^# (.+)$/gm, '<h1 class="text-2xl font-bold text-zinc-100 mt-8 mb-4">$1</h1>')
      // Bold
      .replace(/\*\*(.+?)\*\*/g, '<strong class="font-semibold text-zinc-100">$1</strong>')
      // Italic
      .replace(/\*(.+?)\*/g, '<em class="italic text-zinc-300">$1</em>')
      // Code blocks
      .replace(/```(\w*)\n([\s\S]*?)```/g, '<pre class="bg-zinc-950 border border-zinc-800 rounded-md p-4 my-4 overflow-x-auto"><code class="text-sm text-zinc-300">$2</code></pre>')
      // Inline code
      .replace(/`(.+?)`/g, '<code class="bg-zinc-800 text-zinc-300 px-1 rounded text-sm font-mono">$1</code>')
      // Lists
      .replace(/^- (.+)$/gm, '<li class="text-zinc-300 ml-4 list-disc">$1</li>')
      // Links
      .replace(/\[(.+?)\]\((.+?)\)/g, '<a href="$2" class="text-blue-400 hover:text-blue-300 underline">$1</a>')
      // Paragraphs (double newline)
      .replace(/\n\n/g, '</p><p class="text-zinc-300 leading-relaxed mb-4">')
      // Single newlines within paragraphs
      .replace(/\n/g, '<br/>');

    return `<p class="text-zinc-300 leading-relaxed mb-4">${html}</p>`;
  };

  return (
    <div className="flex flex-col h-full">
      <div className="flex-1 overflow-y-auto p-6">
        <article
          className="prose prose-invert max-w-3xl mx-auto"
          dangerouslySetInnerHTML={{ __html: renderHtml(content) }}
        />
      </div>
    </div>
  );
}