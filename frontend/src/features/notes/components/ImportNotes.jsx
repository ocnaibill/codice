import React, { useRef, useState } from 'react';
import { useDialog } from '../../../lib/useDialog';
import { describeError } from '../../admin/api/admin';
import { chosenMapping, initialMapping, skippedText, useImportNotes, usePreviewImport } from '../api/useNotesImport';

const NO_WORK = '0';

/**
 * Notes brought from a reading device: the file My Clippings.txt of a Kindle, or the highlights KOReader exports in JSON. The person
 * chooses the file, sees which book of the library each of its books is (the ones the title and the author tell are chosen already) and
 * confirms; nothing is written before that. Importing again brings nothing twice.
 */
export function ImportNotes({ onClose }) {
  const dialogRef = useRef(null);
  const [file, setFile] = useState(null);
  const [mapping, setMapping] = useState({});
  const preview = usePreviewImport();
  const run = useImportNotes();
  useDialog(dialogRef, { onEscape: onClose });

  const choose = (event) => {
    const picked = event.target.files?.[0];
    if (!picked) return;
    setFile(picked);
    run.reset();
    preview.mutate(picked, { onSuccess: (data) => setMapping(initialMapping(data.groups)) });
  };
  const data = preview.data;
  const chosen = chosenMapping(mapping);
  const done = run.data;
  const books = Object.keys(chosen).length;

  return (
    <div
      ref={dialogRef}
      role="dialog"
      aria-modal="true"
      aria-label="Importar anotações"
      className="fixed inset-0 z-[60] flex items-center justify-center bg-ink/60 backdrop-blur-sm sm:p-4"
      onClick={(event) => event.target === event.currentTarget && onClose()}
    >
      <div className="flex h-full w-full flex-col overflow-hidden bg-[#faf8f4] shadow-2xl sm:h-auto sm:max-h-[90vh] sm:max-w-2xl sm:rounded-2xl">
        <div className="flex shrink-0 items-start justify-between gap-3 border-b border-border-hairline px-4 py-3 sm:px-6">
          <div>
            <p className="font-mono text-[10px] uppercase tracking-widest text-ink-faint">Anotações</p>
            <h2 className="font-display text-2xl text-ink">Importar do leitor</h2>
          </div>
          <button type="button" onClick={onClose} aria-label="Fechar" className="min-h-10 rounded-lg px-3 text-sm text-ink-soft hover:bg-surface-alt">Fechar</button>
        </div>

        <div className="flex-1 overflow-y-auto px-4 py-4 sm:px-6">
          {!done && (
            <>
              <p className="text-sm text-ink-soft">
                Traga o que você marcou no seu leitor. Cada trecho vira um destaque ou uma nota sua, no livro certo, com o dia em que foi marcado.
                O que já veio antes não vem de novo.
              </p>
              <ul className="mt-2 list-disc pl-5 text-[13px] text-ink-soft">
                <li><strong>Kindle:</strong> o arquivo <code>My Clippings.txt</code>, na pasta <code>documents</code> do aparelho, copiado por USB.</li>
                <li><strong>KOReader:</strong> no menu, <em>Exportar destaques</em> em JSON, e o arquivo que ele gera.</li>
              </ul>
              <label className="mt-3 block text-sm text-ink">
                Arquivo
                <input type="file" accept=".txt,.json,text/plain,application/json" onChange={choose} className="mt-1 block w-full text-sm" />
              </label>
              {preview.isPending && <p className="mt-3 animate-pulse text-sm text-ink-faint">Lendo o arquivo…</p>}
              {preview.isError && <p role="alert" className="mt-3 text-[13px] text-danger">{describeError(preview.error)}</p>}
            </>
          )}

          {data && !done && (
            <section aria-label="Livros do arquivo" className="mt-4">
              <p className="text-sm text-ink">
                {data.entries} {data.entries === 1 ? 'anotação' : 'anotações'} em {data.groups.length} {data.groups.length === 1 ? 'livro' : 'livros'}
                {data.format === 'kindle' ? ' (Kindle)' : ' (KOReader)'}.
              </p>
              {skippedText(data.skipped) && <p className="mt-1 text-[12px] text-ink-faint">{skippedText(data.skipped)}</p>}
              <ul className="mt-3 flex flex-col gap-3">
                {data.groups.map((g) => (
                  <li key={g.key} className="rounded-xl border border-border-hairline bg-white p-3">
                    <p className="text-sm font-semibold text-ink">{g.title || 'Sem título'}</p>
                    <p className="text-[12px] text-ink-faint">
                      {g.author || 'Autor desconhecido'} · {g.highlights} {g.highlights === 1 ? 'destaque' : 'destaques'}
                      {g.notes > 0 ? ` · ${g.notes} ${g.notes === 1 ? 'nota' : 'notas'}` : ''}
                    </p>
                    <label className="mt-2 block text-[13px] text-ink">
                      Na biblioteca é
                      <select
                        value={String(mapping[g.key] ?? 0)}
                        onChange={(event) => setMapping((m) => ({ ...m, [g.key]: Number(event.target.value) }))}
                        className="mt-1 w-full rounded-lg border border-border-hairline bg-white px-3 py-2 text-sm text-ink"
                      >
                        <option value={NO_WORK}>{g.candidates.length ? 'Não importar este livro' : 'Nenhuma obra parecida: não será importado'}</option>
                        {g.candidates.map((c) => (
                          <option key={c.workId} value={String(c.workId)}>{c.title}{c.author ? ` · ${c.author}` : ''}</option>
                        ))}
                      </select>
                    </label>
                  </li>
                ))}
              </ul>
              {run.isError && <p role="alert" className="mt-3 text-[13px] text-danger">{describeError(run.error)}</p>}
            </section>
          )}

          {done && (
            <div role="status" className="rounded-xl border border-brand/30 bg-brand/5 p-4">
              <p className="text-sm font-medium text-ink">
                {done.created} {done.created === 1 ? 'anotação importada' : 'anotações importadas'}.
              </p>
              <ul className="mt-1 list-disc pl-5 text-[13px] text-ink-soft">
                {done.duplicates > 0 && <li>{done.duplicates} já estavam aqui.</li>}
                <li>{done.located} com o lugar no livro; {done.unlocated} só no livro, sem o lugar (o texto não foi achado, ou o Kindle não diz onde está).</li>
                {done.ignored > 0 && <li>{done.ignored} de livros que você deixou de fora.</li>}
              </ul>
            </div>
          )}
        </div>

        <div className="flex shrink-0 items-center justify-end gap-2 border-t border-border-hairline px-4 py-3 sm:px-6">
          {done ? (
            <button type="button" onClick={onClose} className="min-h-10 rounded-lg bg-brand px-4 py-2 text-sm font-medium text-white hover:brightness-110">Pronto</button>
          ) : (
            <>
              <button type="button" onClick={onClose} className="min-h-10 rounded-lg border border-border-hairline bg-white px-4 py-2 text-sm text-ink hover:bg-surface-alt">Cancelar</button>
              <button
                type="button"
                disabled={!data || books === 0 || run.isPending}
                onClick={() => run.mutate({ file, mapping: chosen })}
                className="min-h-10 rounded-lg bg-brand px-4 py-2 text-sm font-medium text-white hover:brightness-110 disabled:opacity-40"
              >
                {run.isPending ? 'Importando…' : books === 0 ? 'Importar' : `Importar ${books} ${books === 1 ? 'livro' : 'livros'}`}
              </button>
            </>
          )}
        </div>
      </div>
    </div>
  );
}
