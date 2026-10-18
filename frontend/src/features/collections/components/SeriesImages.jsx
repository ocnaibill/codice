import React, { useRef, useState } from 'react';
import { WorkCover } from '../../../components/ui/WorkCover';
import { authenticatedUrl } from '../../../lib/api';
import { CoverCropDialog } from '../../library/components/CoverCropDialog';
import { collectionReason, useResetCollectionImage, useSetCollectionImage } from '../api/useCollections';

const MAX_PICTURE_BYTES = 20 * 1024 * 1024;
const BUTTON = 'min-h-10 rounded-lg border border-border-hairline bg-white px-3 py-2 text-xs text-ink hover:border-brand hover:text-brand disabled:opacity-40';
const QUIET = 'min-h-10 rounded-lg px-3 py-2 text-xs text-ink-soft hover:text-brand disabled:opacity-40';

/** One picture of the series: the cover (2:3) or the banner (3:1). It is saved at once, as the cover of a work is (DEC-173, DEC-201). */
function ImageRow({ collection, which }) {
  const banner = which === 'banner';
  const current = banner ? collection.bannerUrl : collection.coverManual ? collection.coverUrl : '';
  const [file, setFile] = useState(null);
  const [problem, setProblem] = useState('');
  const picker = useRef(null);
  const send = useSetCollectionImage(collection.id, which);
  const reset = useResetCollectionImage(collection.id, which);
  const busy = send.isPending || reset.isPending;

  const choose = (event) => {
    const picked = event.target.files?.[0];
    event.target.value = '';
    if (!picked) return;
    if (!/^image\/(jpeg|png|webp)$/.test(picked.type)) return setProblem('Escolha uma imagem JPEG, PNG ou WebP.');
    if (picked.size > MAX_PICTURE_BYTES) return setProblem('A imagem tem mais de 20 MB.');
    setProblem('');
    setFile(picked);
  };
  const confirm = (blob) => send.mutate(blob, { onSuccess: () => setFile(null) });

  return (
    <div className="flex items-start gap-4">
      <div
        className={`${banner ? 'w-48' : 'w-20'} shrink-0 overflow-hidden rounded-md bg-surface-alt ring-1 ring-border-hairline`}
        style={{ aspectRatio: banner ? '3 / 1' : '2 / 3' }}
      >
        {banner ? (
          current && <img src={authenticatedUrl(current)} alt="" className="h-full w-full object-cover" />
        ) : (
          <WorkCover item={{ title: collection.name, coverUrl: collection.coverUrl }} className="h-full w-full object-cover" />
        )}
      </div>
      <div className="flex min-w-0 flex-col items-start gap-2">
        <div className="text-xs font-medium text-ink-soft">{banner ? 'Fundo da série' : 'Capa da série'}</div>
        <p className="text-xs text-ink-soft">
          {banner
            ? current ? 'Escolhido por você.' : 'Sem fundo: a página mostra só a capa.'
            : current ? 'Escolhida por você.' : 'A da primeira obra que tem capa.'}
        </p>
        <div className="flex flex-wrap gap-2">
          <button type="button" onClick={() => picker.current?.click()} disabled={busy} className={BUTTON}>
            {banner ? 'Escolher fundo…' : 'Trocar capa…'}
          </button>
          {current && (
            <button type="button" onClick={() => reset.mutate()} disabled={busy} className={QUIET}>
              {reset.isPending ? 'Voltando…' : banner ? 'Tirar o fundo' : 'Voltar à capa automática'}
            </button>
          )}
        </div>
        <input
          ref={picker}
          type="file"
          accept="image/jpeg,image/png,image/webp"
          onChange={choose}
          className="hidden"
          aria-label={banner ? 'Imagem do fundo' : 'Imagem da capa'}
          tabIndex={-1}
        />
        {problem && <p role="alert" className="text-xs text-danger">{problem}</p>}
        {reset.isError && <p role="alert" className="text-xs text-danger">{collectionReason(reset.error, 'Não foi possível desfazer.')}</p>}
      </div>
      {file && (
        <CoverCropDialog
          file={file}
          banner={banner}
          busy={send.isPending}
          error={send.isError ? collectionReason(send.error, banner ? 'Não foi possível salvar o fundo.' : 'Não foi possível salvar a capa.') : ''}
          onConfirm={confirm}
          onCancel={() => { send.reset(); setFile(null); }}
        />
      )}
    </div>
  );
}

/** The cover and the banner of a series, for the staff (DEC-201). The series shows the cover of its first work until somebody chooses one. */
export function SeriesImages({ collection, onDone }) {
  return (
    <section aria-label="Capa e fundo da série" className="flex flex-col gap-5 rounded-xl border border-border-hairline bg-white p-4 shadow-sm">
      <ImageRow collection={collection} which="cover" />
      <ImageRow collection={collection} which="banner" />
      <p className="text-xs text-ink-faint">
        A capa da série aparece na lista de séries, nos cartões da biblioteca, nos favoritos, no Kindle e no catálogo OPDS. O fundo aparece no alto desta página.
        As capas das obras continuam sendo trocadas em cada obra.
      </p>
      <div>
        <button type="button" onClick={onDone} className={BUTTON}>Fechar</button>
      </div>
    </section>
  );
}
