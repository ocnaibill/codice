import { useState } from 'react';
import { useBulkImport, describeError } from '../api/admin';
import { ConfirmDialog } from './ConfirmDialog';
import { Btn, ErrorNote, Section } from './ui';

/**
 * Imports a folder that lives on the server. Whether the originals are deleted
 * afterwards is asked EVERY time, before anything is copied: there is no default
 * and no "remember my choice", because deleting someone's files is not something
 * to do by habit. Deleting them saves space; keeping them is the safe choice.
 */
export function ImportFolder() {
  const [directory, setDirectory] = useState('');
  const [asking, setAsking] = useState(false);
  const importFolder = useBulkImport();
  const result = importFolder.data;

  const choose = (removeOriginals) => {
    setAsking(false);
    importFolder.mutate({ directory: directory.trim(), removeOriginals });
  };

  return (
    <Section
      title="Importar uma pasta"
      hint="Copia os livros de uma pasta do servidor para o acervo. Deixe em branco para usar a pasta de importação padrão."
    >
      <div className="flex flex-wrap gap-2">
        <input
          value={directory}
          onChange={(event) => setDirectory(event.target.value)}
          placeholder="/caminho/da/pasta (opcional)"
          aria-label="Pasta a importar"
          className="min-w-[240px] flex-1 rounded bg-surface px-3 py-2 text-[13px] outline-none"
        />
        <Btn tone="primary" disabled={importFolder.isPending} onClick={() => setAsking(true)}>
          {importFolder.isPending ? 'Importando…' : 'Importar'}
        </Btn>
      </div>

      <ErrorNote>{importFolder.isError && describeError(importFolder.error)}</ErrorNote>

      {result && (
        <p role="status" className="mt-3 text-[13px] text-ink-soft">
          {result.scanned} arquivo(s) encontrado(s): {result.enqueued} importado(s), {result.duplicates} já
          existia(m), {result.errors} com erro. Originais apagados: {result.originalsRemoved}
          {result.cleanupPending > 0 && ` (${result.cleanupPending} aguardando remoção, veja Armazenamento)`}.
        </p>
      )}

      {asking && (
        <ConfirmDialog
          title="Apagar os originais depois de copiar?"
          message={
            <>
              <p>
                Cada livro é copiado para o acervo. Se você apagar os originais, o espaço é liberado, mas os
                arquivos só saem da pasta se a cópia estiver íntegra e o original não tiver mudado.
              </p>
              <p className="mt-2">Manter os originais não apaga nada.</p>
            </>
          }
          choices={[
            { label: 'Manter originais', value: false, tone: 'primary' },
            { label: 'Apagar originais', value: true, tone: 'danger' },
          ]}
          onChoose={choose}
          onCancel={() => setAsking(false)}
        />
      )}
    </Section>
  );
}
