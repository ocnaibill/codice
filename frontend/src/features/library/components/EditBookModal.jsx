import React, { useState } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../lib/api';

export function EditBookModal({ book, onClose }) {
  const queryClient = useQueryClient();

  const [title, setTitle] = useState(book.title || '');
  const [author, setAuthor] = useState(book.author || '');
  const [series, setSeries] = useState(book.series || '');
  const [seriesIndex, setSeriesIndex] = useState(book.seriesIndex || '');
  const [isbn, setIsbn] = useState(book.isbn || '');
  const [publisher, setPublisher] = useState(book.publisher || '');
  const [language, setLanguage] = useState(book.language || '');
  const [description, setDescription] = useState(book.description || '');
  const [tagsInput, setTagsInput] = useState(book.tags ? book.tags.join(', ') : '');
  const [isConfirmingDelete, setIsConfirmingDelete] = useState(false);

  const updateMutation = useMutation({
    mutationFn: async (updatedData) => {
      await api.put(`/works/${book.id}`, updatedData);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['works'] });
      onClose();
    },
  });

  const deleteMutation = useMutation({
    mutationFn: async () => {
      await api.delete(`/works/${book.id}`);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['works'] });
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
      series_index: seriesIndex ? parseFloat(seriesIndex) : undefined,
      isbn,
      publisher,
      language,
      description,
      tags: cleanTags,
    });
  };

  const handleDelete = () => {
    if (isConfirmingDelete) {
      deleteMutation.mutate();
    } else {
      setIsConfirmingDelete(true);
      setTimeout(() => setIsConfirmingDelete(false), 3000);
    }
  };

  const isPending = updateMutation.isPending || deleteMutation.isPending;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4">
      <div className="bg-zinc-900 border border-zinc-800 rounded-xl shadow-2xl w-full max-w-lg max-h-[90vh] overflow-y-auto p-6">
        <div className="flex justify-between items-center mb-6">
          <h2 className="text-xl font-semibold text-zinc-100">Edit Book Metadata</h2>
          <div className="flex items-center gap-3">
            <button 
              type="button"
              onClick={handleDelete}
              disabled={isPending}
              className={`px-3 py-1 text-xs font-medium rounded-md transition-all ${
                isConfirmingDelete 
                  ? 'bg-red-600 text-white animate-pulse' 
                  : 'bg-zinc-950 text-red-400 border border-red-900/50 hover:bg-red-950 hover:border-red-800'
              }`}
            >
              {deleteMutation.isPending ? 'Deleting...' : (isConfirmingDelete ? 'Are you sure?' : 'Delete Book')}
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