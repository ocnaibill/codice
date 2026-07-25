import React, { useState } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../lib/api';

export function EditBookModal({ book, onClose }) {
  const queryClient = useQueryClient();

  // Populate initial state with existing book metadata
  const [title, setTitle] = useState(book.title || '');
  const [author, setAuthor] = useState(book.author || '');
  const [tagsInput, setTagsInput] = useState(book.tags ? book.tags.join(', ') : '');
  
  // State for double confirmation delete safety
  const [isConfirmingDelete, setIsConfirmingDelete] = useState(false);

  // Mutation to send PUT request to Go API
  const updateMutation = useMutation({
    mutationFn: async (updatedData) => {
      await api.put(`/works/${book.id}`, updatedData);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['works'] });
      onClose();
    },
  });

  // Mutation to send DELETE request to Go API
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
    
    // Parse comma-separated tags string into a cleaned array
    const cleanTags = tagsInput
      .split(',')
      .map((tag) => tag.trim())
      .filter((tag) => tag.length > 0);

    updateMutation.mutate({
      title,
      author,
      tags: cleanTags,
    });
  };

  const handleDelete = () => {
    if (isConfirmingDelete) {
      deleteMutation.mutate();
    } else {
      setIsConfirmingDelete(true);
      // Automatically reset confirmation after 3 seconds if not confirmed
      setTimeout(() => setIsConfirmingDelete(false), 3000);
    }
  };

  const isPending = updateMutation.isPending || deleteMutation.isPending;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4">
      <div className="bg-zinc-900 border border-zinc-800 rounded-xl shadow-2xl w-full max-w-md p-6">
        
        {/* Header with Title, Delete Button & Close Button */}
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
          <div>
            <label className="block text-xs font-medium text-zinc-400 mb-1">Book Title</label>
            <input 
              type="text" 
              value={title} 
              onChange={(e) => setTitle(e.target.value)}
              className="w-full bg-zinc-950 border border-zinc-800 rounded-md px-3 py-2 text-sm text-zinc-200 focus:outline-none focus:border-blue-500 transition-colors"
              required
              disabled={isPending}
            />
          </div>

          <div>
            <label className="block text-xs font-medium text-zinc-400 mb-1">Author</label>
            <input 
              type="text" 
              value={author} 
              onChange={(e) => setAuthor(e.target.value)}
              className="w-full bg-zinc-950 border border-zinc-800 rounded-md px-3 py-2 text-sm text-zinc-200 focus:outline-none focus:border-blue-500 transition-colors"
              required
              disabled={isPending}
            />
          </div>

          <div>
            <label className="block text-xs font-medium text-zinc-400 mb-1">Tags (comma separated)</label>
            <input 
              type="text" 
              value={tagsInput} 
              onChange={(e) => setTagsInput(e.target.value)}
              placeholder="Fantasy, Sci-Fi, RPG"
              className="w-full bg-zinc-950 border border-zinc-800 rounded-md px-3 py-2 text-sm text-zinc-200 focus:outline-none focus:border-blue-500 transition-colors"
              disabled={isPending}
            />
          </div>

          {(updateMutation.isError || deleteMutation.isError) && (
            <p className="text-red-400 text-xs font-medium">
              {deleteMutation.isError ? 'Error deleting book.' : 'Error saving metadata changes.'}
            </p>
          )}

          <div className="flex justify-end gap-3 mt-4 pt-4 border-t border-zinc-800/80">
            <button 
              type="button" 
              onClick={onClose}
              disabled={isPending}
              className="px-4 py-2 text-sm font-medium text-zinc-400 hover:text-zinc-100 disabled:opacity-30 transition-colors"
            >
              Cancel
            </button>
            <button 
              type="submit"
              disabled={isPending}
              className="px-4 py-2 bg-blue-600 text-white text-sm font-medium rounded-md hover:bg-blue-500 disabled:opacity-50 transition-colors"
            >
              {updateMutation.isPending ? 'Saving...' : 'Save Changes'}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
