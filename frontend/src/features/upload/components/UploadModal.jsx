import React, { useState } from 'react';
import { useGlobalStore } from '../../../store/useGlobalStore';
import { useUploadBook } from '../api/useUploadBook';

export function UploadModal() {
  const [selectedFile, setSelectedFile] = useState(null);
  const [uploadProgress, setUploadProgress] = useState(0);
  
  const isOpen = useGlobalStore((state) => state.isUploadModalOpen);
  const closeModal = useGlobalStore((state) => state.closeUploadModal);
  
  const { mutate: uploadBook, isPending, isError } = useUploadBook();

  if (!isOpen) return null;

  const handleFileChange = (e) => {
    if (e.target.files && e.target.files.length > 0) {
      setSelectedFile(e.target.files[0]);
      setUploadProgress(0);
    }
  };

  const handleClose = () => {
    if (!isPending) {
      setSelectedFile(null);
      setUploadProgress(0);
      closeModal();
    }
  };

  const handleUpload = () => {
    if (selectedFile) {
      setUploadProgress(0);
      uploadBook(
        {
          file: selectedFile,
          onProgress: (percent) => setUploadProgress(percent),
        },
        {
          onSuccess: () => {
            // Auto close modal and clear file state on successful upload completion
            handleClose();
          },
        }
      );
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4">
      <div className="bg-zinc-900 border border-zinc-800 rounded-xl shadow-2xl w-full max-w-md p-6">
        <div className="flex justify-between items-center mb-6">
          <h2 className="text-xl font-semibold text-zinc-100">Add Book</h2>
          <button 
            onClick={handleClose} 
            disabled={isPending}
            className="text-zinc-500 hover:text-zinc-300 disabled:opacity-30 transition-colors"
          >
            ✕
          </button>
        </div>

        <div className="flex flex-col gap-4">
          {/* Drop Area / File Input */}
          <label className="flex flex-col items-center justify-center w-full h-32 border-2 border-zinc-700 border-dashed rounded-lg cursor-pointer bg-zinc-950/50 hover:bg-zinc-800 hover:border-zinc-500 transition-colors">
            <div className="flex flex-col items-center justify-center pt-5 pb-6">
              <span className="text-2xl mb-2">📄</span>
              <p className="text-sm text-zinc-400">
                <span className="font-semibold text-blue-400">Click to select</span> a PDF or EPUB
              </p>
            </div>
            <input 
              type="file" 
              className="hidden" 
              accept=".pdf,.epub" 
              onChange={handleFileChange} 
              disabled={isPending}
            />
          </label>

          {selectedFile && (
            <div className="text-sm text-zinc-300 bg-zinc-800 p-3 rounded-md truncate">
              Selected: {selectedFile.name}
            </div>
          )}

          {/* Upload Progress Bar */}
          {isPending && (
            <div className="flex flex-col gap-2 bg-zinc-950 p-4 rounded-lg border border-zinc-800">
              <div className="flex justify-between text-xs text-zinc-400 font-mono">
                <span>{uploadProgress < 100 ? 'Uploading file...' : 'Enqueuing task...'}</span>
                <span>{uploadProgress}%</span>
              </div>
              <div className="w-full h-2 bg-zinc-800 rounded-full overflow-hidden">
                <div 
                  className="h-full bg-blue-500 rounded-full transition-all duration-300 ease-out"
                  style={{ width: `${uploadProgress}%` }}
                />
              </div>
            </div>
          )}

          {/* Status Feedback */}
          {isError && <p className="text-red-400 text-sm font-medium">Error uploading file.</p>}

          {/* Action Buttons */}
          <div className="flex justify-end gap-3 mt-4">
            <button 
              onClick={handleClose}
              disabled={isPending}
              className="px-4 py-2 text-sm font-medium text-zinc-400 hover:text-zinc-100 disabled:opacity-30 transition-colors"
            >
              Cancel
            </button>
            <button 
              onClick={handleUpload}
              disabled={!selectedFile || isPending}
              className="px-4 py-2 text-sm font-medium bg-blue-600 text-white rounded-md hover:bg-blue-500 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
            >
              {isPending ? 'Uploading...' : 'Upload'}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}