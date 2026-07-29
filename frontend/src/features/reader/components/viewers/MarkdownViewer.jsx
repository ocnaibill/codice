import React, { useState, useEffect, useRef } from 'react';
import ReactMarkdown from 'react-markdown';
import { api, authenticatedUrl } from '../../../../lib/api';

export default function MarkdownViewer({ fileUrl, bookId, initialProgress }) {
  const [content, setContent] = useState('');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  useEffect(() => {
    let cancelled = false;
    const loadMd = async () => {
      try {
        const resp = await fetch(authenticatedUrl(fileUrl));
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

  return (
    <div className="flex flex-col h-full">
      <div className="flex-1 overflow-y-auto p-6">
        <article className="prose prose-invert max-w-3xl mx-auto prose-headings:text-zinc-100 prose-p:text-zinc-300 prose-a:text-blue-400 prose-code:text-zinc-300 prose-pre:bg-zinc-950 prose-pre:border prose-pre:border-zinc-800">
          <ReactMarkdown>{content}</ReactMarkdown>
        </article>
      </div>
    </div>
  );
}