import { useState } from 'react';
import {
  useDuplicates, useScanDuplicates, useDismissDuplicate, useLinkDuplicate, useOcr, describeError,
} from '../api/admin';
import { ConfirmDialog } from './ConfirmDialog';
import { Btn, Empty, ErrorNote, Loading, Section } from './ui';

const REASON = { isbn: 'Mesmo ISBN', title_author: 'Mesmo título e autor' };

function Side({ work }) {
  return (
    <div className="min-w-0">
      <p className="text-[14px] font-medium text-ink">{work.title}</p>
      <p className="text-[12px] text-ink-faint">
        {[work.author, work.language, work.formats?.join(', '), work.isbn].filter(Boolean).join(' · ')}
      </p>
    </div>
  );
}

function Duplicates() {
  const { data, isLoading, isError } = useDuplicates();
  const scan = useScanDuplicates();
  const dismiss = useDismissDuplicate();
  const link = useLinkDuplicate();
  const [linking, setLinking] = useState(null); // { candidate, keep }
  const pairs = data?.data || [];

  return (
    <Section
      title="Possíveis duplicatas"
      hint="Obras que talvez sejam o mesmo livro (outro formato, outra edição). O sistema só sugere: nada é unido sem você decidir."
      actions={<Btn onClick={() => scan.mutate()} disabled={scan.isPending}>Comparar o acervo agora</Btn>}
    >
      {scan.isSuccess && <p role="status" className="mb-3 text-[13px] text-ink-soft">Comparação na fila; acompanhe em Trabalhos.</p>}
      {isLoading && <Loading />}
      {isError && <ErrorNote>Não foi possível carregar as sugestões.</ErrorNote>}
      {!isLoading && !isError && pairs.length === 0 && <Empty>Nenhuma sugestão pendente.</Empty>}
      <ul className="divide-y divide-border-hairline">
        {pairs.map((pair) => (
          <li key={pair.id} className="py-4">
            <p className="mb-2 text-[11px] font-bold uppercase tracking-wider text-ink-faint">{REASON[pair.reason] || pair.reason}</p>
            <div className="grid gap-3 sm:grid-cols-2">
              <Side work={pair.a} />
              <Side work={pair.b} />
            </div>
            <div className="mt-3 flex flex-wrap gap-2">
              <Btn onClick={() => setLinking({ pair, keep: pair.a })}>Unir, mantendo “{pair.a.title}” (A)</Btn>
              <Btn onClick={() => setLinking({ pair, keep: pair.b })}>Unir, mantendo “{pair.b.title}” (B)</Btn>
              <Btn onClick={() => dismiss.mutate(pair.id)} disabled={dismiss.isPending}>Não é duplicata</Btn>
            </div>
          </li>
        ))}
      </ul>
      <ErrorNote>{dismiss.isError ? describeError(dismiss.error) : link.isError ? describeError(link.error) : scan.isError && describeError(scan.error)}</ErrorNote>

      {linking && (
        <ConfirmDialog
          title="Unir estas obras?"
          message={
            <p>
              Os arquivos, notas, favoritos e etiquetas de uma passam para “{linking.keep.title}”, que fica como
              única obra, com as duas edições. Isso não pode ser desfeito.
            </p>
          }
          choices={[{ label: 'Unir obras', value: true, tone: 'danger' }]}
          onChoose={() => { link.mutate({ id: linking.pair.id, keep: linking.keep.id }); setLinking(null); }}
          onCancel={() => setLinking(null)}
        />
      )}
    </Section>
  );
}

function Ocr() {
  const { data, isLoading } = useOcr();
  const items = data?.data || [];
  return (
    <Section
      title="PDFs com páginas sem texto"
      hint="Páginas que são só imagem: não dá para buscar nem selecionar o texto nelas. Rodar o OCR ainda não faz parte do sistema; esta lista mostra onde ele seria útil."
    >
      {isLoading && <Loading />}
      {!isLoading && items.length === 0 && <Empty>Nenhum PDF precisa de OCR.</Empty>}
      <ul className="divide-y divide-border-hairline text-[13px]">
        {items.map((item) => (
          <li key={item.fileId} className="py-2">
            <p className="text-ink">{item.title}</p>
            <p className="text-[12px] text-ink-faint">
              {item.pagesWithoutText.length === item.pageCount
                ? `Todas as ${item.pageCount} páginas são imagem`
                : `${item.pagesWithoutText.length} de ${item.pageCount} páginas sem texto (${item.pagesWithoutText.slice(0, 12).join(', ')}${item.pagesWithoutText.length > 12 ? '…' : ''})`}
            </p>
          </li>
        ))}
      </ul>
    </Section>
  );
}

export function DuplicatesTab() {
  return (
    <div className="flex flex-col gap-5">
      <Duplicates />
      <Ocr />
    </div>
  );
}
