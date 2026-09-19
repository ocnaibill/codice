import React, { useEffect, useRef, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, authenticatedUrl } from '../../../lib/api';
import { refreshLibrary } from '../../../lib/refreshLibrary';

export function EditBookModal({ book, onClose }) {
  const queryClient = useQueryClient();

  // The list card does not carry ISBN, publisher, language, description or the
  // locks, so the form starts from the full record. Saving before it loads would
  // send empty values and clear those fields.
  const { data: full, isLoading: isLoadingFull } = useQuery({
    queryKey: ['work-edit', book.id],
    queryFn: async () => (await api.get(`/works/${book.id}`)).data,
  });
  const { data: candidates = [] } = useQuery({
    queryKey: ['work-candidates', book.id],
    queryFn: async () => (await api.get(`/works/${book.id}/candidates`)).data.data,
  });

  const [title, setTitle] = useState(book.title || '');
  const [author, setAuthor] = useState(book.author || '');
  const [series, setSeries] = useState('');
  const [seriesIndex, setSeriesIndex] = useState('');
  const [isbn, setIsbn] = useState('');
  const [publisher, setPublisher] = useState('');
  const [language, setLanguage] = useState('');
  const [publicationDate, setPublicationDate] = useState('');
  const [description, setDescription] = useState('');
  const [tagsInput, setTagsInput] = useState(book.tags ? book.tags.join(', ') : '');
  const [locks, setLocks] = useState({});
  const initialized = useRef(false);

  useEffect(() => {
    if (!full || initialized.current) return;
    initialized.current = true;
    const m = full.metadata || {};
    setTitle(full.title || '');
    setAuthor(full.author === 'Unknown Author' ? '' : full.author || '');
    setSeries(m.series || '');
    setSeriesIndex(m.seriesIndex ? String(m.seriesIndex) : '');
    setIsbn(m.isbn || '');
    setPublisher(m.publisher || '');
    setLanguage(m.language || '');
    setPublicationDate(m.publicationDate || '');
    setDescription(m.description || '');
    setTagsInput((full.tags || []).join(', '));
    setLocks(m.locks || {});
  }, [full]);

  const setLock = (field, value) => setLocks((prev) => ({ ...prev, [field]: value }));
  const [isConfirmingDelete, setIsConfirmingDelete] = useState(false);

  // --- Metadata Search ---
  const [searchQuery, setSearchQuery] = useState('');
  const [searchResults, setSearchResults] = useState([]);
  const [isSearching, setIsSearching] = useState(false);
  const [searchError, setSearchError] = useState('');

  const handleSearch = async () => {
    const q = searchQuery.trim() || book.title;
    if (!q) return;
    setIsSearching(true);
    setSearchError('');
    setSearchResults([]);
    try {
      const res = await api.get('/metadata/search', { params: { q, format: book.format } });
      setSearchResults(res.data.results || []);
      if (!res.data.results || res.data.results.length === 0) {
        setSearchError('No results found from any provider.');
      }
    } catch (err) {
      setSearchError('Search service unavailable. Is the worker search server running?');
    } finally {
      setIsSearching(false);
    }
  };

  const applySearchResult = (result) => {
    if (result.title) setTitle(result.title);
    if (result.author) setAuthor(result.author);
    if (result.series) setSeries(result.series);
    if (result.series_index != null) setSeriesIndex(String(result.series_index));
    if (result.isbn) setIsbn(result.isbn);
    if (result.publisher) setPublisher(result.publisher);
    if (result.language) setLanguage(result.language);
    if (result.description) setDescription(result.description);
    if (result.tags && result.tags.length > 0) setTagsInput(result.tags.join(', '));
  };

  const updateMutation = useMutation({
    mutationFn: async (updatedData) => {
      await api.put(`/works/${book.id}`, updatedData);
    },
    onSuccess: () => {
      refreshLibrary(queryClient);
      onClose();
    },
  });

  const deleteMutation = useMutation({
    mutationFn: async () => {
      await api.delete(`/works/${book.id}`);
    },
    onSuccess: () => {
      refreshLibrary(queryClient);
      onClose();
    },
  });

  const handleSubmit = (e) => {
    e.preventDefault();
    const cleanTags = tagsInput
      .split(',')
      .map((tag) => tag.trim())
      .filter((tag) => tag.length > 0);

    updateMutation.mutate({
      title,
      author,
      series,
      series_index: seriesIndex ? parseFloat(seriesIndex) : 0,
      isbn,
      publisher,
      language,
      publication_date: publicationDate,
      description,
      tags: cleanTags,
      title_lock: !!locks.title,
      author_lock: !!locks.author,
      series_lock: !!locks.series,
      cover_lock: !!locks.cover,
      isbn_lock: !!locks.isbn,
      publisher_lock: !!locks.publisher,
      language_lock: !!locks.language,
      description_lock: !!locks.description,
      publication_date_lock: !!locks.publication_date,
    });
  };

  // Suggestions from external providers: nothing changes until an admin accepts.
  const decideMutation = useMutation({
    mutationFn: async ({ id, verb }) => {
      await api.post(`/works/${book.id}/candidates/${id}/${verb}`);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['work-candidates', book.id] });
      queryClient.invalidateQueries({ queryKey: ['work-edit', book.id] });
      refreshLibrary(queryClient);
      // Reload the form from the record the server now holds.
      initialized.current = false;
    },
  });

  const handleDelete = () => {
    if (isConfirmingDelete) {
      deleteMutation.mutate();
    } else {
      setIsConfirmingDelete(true);
      setTimeout(() => setIsConfirmingDelete(false), 3000);
    }
  };

  const isPending = updateMutation.isPending || deleteMutation.isPending || isLoadingFull;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4">
      <div className="bg-zinc-900 border border-zinc-800 rounded-xl shadow-2xl w-full max-w-lg max-h-[90vh] overflow-y-auto p-6">
        <div className="flex justify-between items-center mb-6">
          <h2 className="text-xl font-semibold text-zinc-100">Edit Book Metadata</h2>
          <div className="flex items-center gap-3">
            <button 
              type="button"
              title="Hides the book from the library. Files, notes and progress are kept and it can be restored."
              onClick={handleDelete}
              disabled={isPending}
              className={`px-3 py-1 text-xs font-medium rounded-md transition-all ${
                isConfirmingDelete 
                  ? 'bg-red-600 text-white animate-pulse' 
                  : 'bg-zinc-950 text-red-400 border border-red-900/50 hover:bg-red-950 hover:border-red-800'
              }`}
            >
              {deleteMutation.isPending ? 'Retiring...' : (isConfirmingDelete ? 'Are you sure?' : 'Retire from Library')}
            </button>
            <button 
              onClick={onClose} 
              disabled={isPending}
              className="text-zinc-500 hover:text-zinc-300 disabled:opacity-30 transition-colors"
            >
              ✕
            </button>
          </div>
        </div>

        {/* Manual Metadata Search Section */}
        <div className="mb-4 p-3 bg-blue-950/30 border border-blue-900/50 rounded-lg">
          <label className="block text-xs font-medium text-blue-300 mb-1">🔎 Search Internet</label>
          <div className="flex gap-2">
            <input
              type="text"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && (e.preventDefault(), handleSearch())}
              placeholder={book.title || 'Search title...'}
              className="flex-1 bg-zinc-950 border border-zinc-800 rounded-md px-3 py-2 text-sm text-zinc-200 focus:outline-none focus:border-blue-500"
              disabled={isSearching}
            />
            <button
              type="button"
              onClick={handleSearch}
              disabled={isSearching}
              className="px-3 py-2 bg-blue-600 text-white text-sm font-medium rounded-md hover:bg-blue-500 disabled:opacity-50 transition-colors whitespace-nowrap"
            >
              {isSearching ? 'Searching...' : 'Search'}
            </button>
          </div>
          {searchError && <p className="text-red-400 text-xs mt-1">{searchError}</p>}
          {searchResults.length > 0 && (
            <div className="mt-2 space-y-2 max-h-48 overflow-y-auto">
              {searchResults.map((result, idx) => (
                <div key={idx} className="p-2 bg-zinc-950 rounded border border-zinc-800 text-xs">
                  <div className="flex justify-between items-start">
                    <div className="flex-1 min-w-0">
                      <p className="font-medium text-blue-300 truncate">[{result.source}] {result.title}</p>
                      {result.author && <p className="text-zinc-400 truncate">by {result.author}</p>}
                      {result.series && (
                        <p className="text-zinc-500 truncate">
                          {result.series}{result.series_index != null ? ` #${result.series_index}` : ''}
                        </p>
                      )}
                      {result.description && (
                        <p className="text-zinc-600 mt-1 line-clamp-2">{result.description}</p>
                      )}
                    </div>
                    <button
                      type="button"
                      onClick={() => applySearchResult(result)}
                      className="ml-2 px-2 py-1 bg-green-700 text-white text-xs rounded hover:bg-green-600 transition-colors whitespace-nowrap shrink-0"
                    >
                      Apply
                    </button>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>

        <form onSubmit={handleSubmit} className="flex flex-col gap-4">
          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="block text-xs font-medium text-zinc-400 mb-1">Title</label>
              <input type="text" value={title} onChange={(e) => setTitle(e.target.value)}
                className="w-full bg-zinc-950 border border-zinc-800 rounded-md px-3 py-2 text-sm text-zinc-200 focus:outline-none focus:border-blue-500"
                required disabled={isPending} />
            </div>
            <div>
              <label className="block text-xs font-medium text-zinc-400 mb-1">Author</label>
              <input type="text" value={author} onChange={(e) => setAuthor(e.target.value)}
                className="w-full bg-zinc-950 border border-zinc-800 rounded-md px-3 py-2 text-sm text-zinc-200 focus:outline-none focus:border-blue-500"
                required disabled={isPending} />
            </div>
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="block text-xs font-medium text-zinc-400 mb-1">Series</label>
              <input type="text" value={series} onChange={(e) => setSeries(e.target.value)}
                placeholder="e.g. Harry Potter"
                className="w-full bg-zinc-950 border border-zinc-800 rounded-md px-3 py-2 text-sm text-zinc-200 focus:outline-none focus:border-blue-500"
                disabled={isPending} />
            </div>
            <div>
              <label className="block text-xs font-medium text-zinc-400 mb-1">Series #</label>
              <input type="number" step="0.1" value={seriesIndex} onChange={(e) => setSeriesIndex(e.target.value)}
                placeholder="e.g. 1"
                className="w-full bg-zinc-950 border border-zinc-800 rounded-md px-3 py-2 text-sm text-zinc-200 focus:outline-none focus:border-blue-500"
                disabled={isPending} />
            </div>
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="block text-xs font-medium text-zinc-400 mb-1">ISBN</label>
              <input type="text" value={isbn} onChange={(e) => setIsbn(e.target.value)}
                placeholder="e.g. 978-3-16-148410-0"
                className="w-full bg-zinc-950 border border-zinc-800 rounded-md px-3 py-2 text-sm text-zinc-200 focus:outline-none focus:border-blue-500"
                disabled={isPending} />
            </div>
            <div>
              <label className="block text-xs font-medium text-zinc-400 mb-1">Publisher</label>
              <input type="text" value={publisher} onChange={(e) => setPublisher(e.target.value)}
                className="w-full bg-zinc-950 border border-zinc-800 rounded-md px-3 py-2 text-sm text-zinc-200 focus:outline-none focus:border-blue-500"
                disabled={isPending} />
            </div>
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="block text-xs font-medium text-zinc-400 mb-1">Language</label>
              <input type="text" value={language} onChange={(e) => setLanguage(e.target.value)}
                placeholder="e.g. en, pt-BR"
                className="w-full bg-zinc-950 border border-zinc-800 rounded-md px-3 py-2 text-sm text-zinc-200 focus:outline-none focus:border-blue-500"
                disabled={isPending} />
            </div>
            <div>
              <label className="block text-xs font-medium text-zinc-400 mb-1">Tags (comma separated)</label>
              <input type="text" value={tagsInput} onChange={(e) => setTagsInput(e.target.value)}
                placeholder="Fantasy, Sci-Fi"
                className="w-full bg-zinc-950 border border-zinc-800 rounded-md px-3 py-2 text-sm text-zinc-200 focus:outline-none focus:border-blue-500"
                disabled={isPending} />
            </div>
          </div>

          <div>
            <label className="block text-xs font-medium text-zinc-400 mb-1">Description</label>
            <textarea value={description} onChange={(e) => setDescription(e.target.value)}
              rows={3}
              className="w-full bg-zinc-950 border border-zinc-800 rounded-md px-3 py-2 text-sm text-zinc-200 focus:outline-none focus:border-blue-500 resize-none"
              disabled={isPending} />
          </div>

          {/* Suggestions from external providers */}
          {candidates.length > 0 && (
            <div className="border-t border-zinc-800/80 pt-4">
              <p className="text-xs font-medium text-amber-300 mb-2">💡 Suggestions ({candidates.length}) — nothing changes until you accept</p>
              <div className="flex flex-col gap-2 max-h-40 overflow-y-auto">
                {candidates.map((c) => (
                  <div key={c.id} className="flex items-start justify-between gap-3 bg-zinc-950 border border-zinc-800 rounded-md p-2">
                    <div className="text-xs min-w-0">
                      <span className="text-zinc-500">{c.field} · {c.source}</span>
                      <p className="text-zinc-200 break-words">{c.field === 'tags' ? JSON.parse(c.value).join(', ') : c.value}</p>
                      {c.current && c.field !== 'tags' && <p className="text-zinc-600 break-words">now: {c.current}</p>}
                    </div>
                    <div className="flex gap-1 shrink-0">
                      <button type="button" disabled={decideMutation.isPending}
                        onClick={() => decideMutation.mutate({ id: c.id, verb: 'accept' })}
                        className="px-2 py-1 text-xs bg-blue-600 text-white rounded hover:bg-blue-500 disabled:opacity-50">Accept</button>
                      <button type="button" disabled={decideMutation.isPending}
                        onClick={() => decideMutation.mutate({ id: c.id, verb: 'reject' })}
                        className="px-2 py-1 text-xs bg-zinc-800 text-zinc-300 rounded hover:bg-zinc-700 disabled:opacity-50">Reject</button>
                    </div>
                  </div>
                ))}
              </div>
            </div>
          )}

          {/* Lock toggles: a locked field is never changed by automatic extraction.
              A field you edit is locked when you save. */}
          <div className="flex flex-wrap items-center gap-x-4 gap-y-2 border-t border-zinc-800/80 pt-4">
            {[
              ['title', 'Title'], ['author', 'Author'], ['series', 'Series'], ['cover', 'Cover'],
              ['isbn', 'ISBN'], ['publisher', 'Publisher'], ['language', 'Language'], ['description', 'Description'],
            ].map(([field, label]) => (
              <div key={field} className="flex items-center gap-2">
                <input type="checkbox" id={`lock-${field}`} checked={!!locks[field]}
                  onChange={(e) => setLock(field, e.target.checked)}
                  disabled={isPending}
                  className="rounded bg-zinc-800 border-zinc-700 text-blue-600 focus:ring-blue-500" />
                <label htmlFor={`lock-${field}`} className="text-xs text-zinc-400">🔒 {label}</label>
              </div>
            ))}
          </div>

          {(updateMutation.isError || deleteMutation.isError) && (
            <p className="text-red-400 text-xs font-medium">
              {deleteMutation.isError ? 'Error deleting book.' : 'Error saving changes.'}
            </p>
          )}

          <div className="flex justify-end gap-3 mt-4 pt-4 border-t border-zinc-800/80">
            <button type="button" onClick={onClose} disabled={isPending}
              className="px-4 py-2 text-sm font-medium text-zinc-400 hover:text-zinc-100 disabled:opacity-30 transition-colors">
              Cancel
            </button>
            <button type="submit" disabled={isPending}
              className="px-4 py-2 bg-blue-600 text-white text-sm font-medium rounded-md hover:bg-blue-500 disabled:opacity-50 transition-colors">
              {updateMutation.isPending ? 'Saving...' : 'Save Changes'}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}