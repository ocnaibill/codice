import React from 'react';
import { useGlobalStore } from '../../../store/useGlobalStore';

export function Reader() {
  const activeBookId = useGlobalStore((state) => state.activeBookId);
  const closeBook = useGlobalStore((state) => state.closeBook);
  const books = useGlobalStore((state) => state.books);

  // Busca os metadados do livro atual
  const book = books.find((b) => b.id === activeBookId);

  if (!book) return null;

  return (
    <div className="flex flex-col h-[calc(100vh-4rem)] bg-zinc-950">
      {/* Barra de Controle Interna do Leitor */}
      <div className="flex items-center justify-between px-6 py-3 border-b border-zinc-900 bg-zinc-950 shadow-sm z-10">
        <div>
          <h2 className="text-zinc-200 font-medium">{book.title}</h2>
          <p className="text-xs text-zinc-500">{book.author}</p>
        </div>
        <div className="flex gap-4 items-center">
          <span className="text-xs font-mono text-zinc-500">{book.progress}% Concluído</span>
          <button 
            onClick={closeBook}
            className="p-2 rounded-md bg-zinc-900 border border-zinc-800 hover:bg-zinc-800 text-zinc-400 hover:text-zinc-100 transition-all"
            title="Voltar para a estante"
          >
            ✕ Fechar
          </button>
        </div>
      </div>

      {/* Área de Leitura Centralizada */}
      <div className="flex-1 overflow-y-auto px-4 py-12 md:py-20 scroll-smooth bg-zinc-900/30">
        <div className="max-w-2xl mx-auto text-zinc-300 font-serif leading-relaxed space-y-6 text-lg">
          
          {/* Mock do conteúdo para testar o visual e a rolagem */}
          <h1 className="text-3xl font-bold text-zinc-100 mb-8 font-sans">Prólogo</h1>
          
          <p>
            Os servidores zumbiam silenciosamente no rack de metal. Era o som de uma biblioteca inteira sendo 
            processada em background, alimentada por containers Docker e scripts em Python que não dormiam.
          </p>
          <p>
            Para garantir que a virtualização contínua de fato funcionasse, este texto de marcação 
            precisa ser longo o suficiente para testar a mecânica de rolagem e o contraste da fonte serifada 
            contra o fundo quase completamente escuro da interface.
          </p>
          <p>
            No Códice real, este bloco será substituído pela renderização dinâmica do conteúdo do EPUB, 
            extraída em blocos e renderizada sob demanda usando o IntersectionObserver para garantir que 
            o navegador não engasgue ao carregar um livro de 800 páginas.
          </p>
          
          {/* Blocos repetidos para forçar o scroll vertical */}
          <div className="h-96 border-l-2 border-zinc-800 pl-6 my-12 text-zinc-500 italic flex items-center">
            [Espaço reservado para carregar o próximo capítulo do banco de dados...]
          </div>
          
        </div>
      </div>
    </div>
  );
}