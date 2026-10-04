import { useEffect, useRef } from 'react';
import { useAbout } from '../../features/about/useAbout';
import { THIRD_PARTY } from '../../features/about/thirdParty';
import { LoadError } from '../ui/LoadError';
import { isTopmostDialog } from '../../lib/topDialog';

function ExternalLink({ href, children }) {
  return (
    <a href={href} target="_blank" rel="noopener noreferrer" className="text-brand underline-offset-2 hover:underline">
      {children}
    </a>
  );
}

/** A link only when the server gave an http(s) address: it is shown to everyone, so nothing else becomes a link. */
const safe = (url) => (typeof url === 'string' && /^https?:\/\//i.test(url) ? url : null);

/**
 * "Sobre" (DEC-120): the version, the license and where the source is, and what the Códice carries that others
 * made. The AGPL gives everyone who uses the service over a network the right to the source of the version they
 * use, so the link to it is here for everyone who is signed in, not hidden in the administration. No contact of
 * the owner, by decision.
 */
export function AboutModal({ onClose }) {
  const dialogRef = useRef(null);
  const { data, isLoading, isError, error, refetch, isFetching } = useAbout();

  useEffect(() => {
    const onKey = (event) => event.key === 'Escape' && isTopmostDialog(dialogRef.current) && onClose();
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [onClose]);

  const source = safe(data?.sourceUrl);
  const licenseLink = safe(data?.licenseUrl);

  return (
    <div
      className="fixed inset-0 z-50 flex animate-fade-in items-center justify-center bg-black/50 p-4"
      onClick={(event) => event.target === event.currentTarget && onClose()}
    >
      <div ref={dialogRef} role="dialog" aria-modal="true" aria-label="Sobre o Códice" className="max-h-[92vh] w-full max-w-lg overflow-y-auto animate-pop-in rounded-xl bg-white p-6 shadow-2xl">
        <h2 className="font-display text-xl font-semibold text-ink">Sobre o Códice</h2>

        {isLoading && <p className="py-3 text-[13px] text-ink-faint">Carregando…</p>}
        {isError && (
          <LoadError className="mt-3" error={error} onRetry={refetch} retrying={isFetching}>
            Não foi possível carregar os dados desta instalação.
          </LoadError>
        )}
        {data && (
          <>
            <p className="mt-1 text-[13px] text-ink-soft">
              {data.version === 'dev' ? 'Versão de desenvolvimento' : <>Versão <span className="font-mono">{data.version}</span></>}
            </p>
            <p className="mt-4 text-[14px] text-ink">
              O Códice é software livre, sob a licença <strong>{data.license === 'AGPL-3.0' ? 'GNU AGPLv3' : data.license}</strong>.
              Quem usa um serviço baseado nele pela rede tem direito ao código-fonte da versão que está usando.
            </p>
            <p className="mt-3 flex flex-wrap gap-x-5 gap-y-1 text-[14px]">
              {source && <ExternalLink href={source}>Código-fonte</ExternalLink>}
              {licenseLink && <ExternalLink href={licenseLink}>Texto da licença</ExternalLink>}
            </p>
          </>
        )}

        <h3 className="mt-6 text-[13px] font-medium text-ink">Feito com a ajuda de</h3>
        <p className="mt-1 text-[12px] text-ink-faint">
          O que outras pessoas fizeram e o Códice usa, com a licença de cada uma. A lista completa de dependências está nos
          arquivos <span className="font-mono">go.mod</span>, <span className="font-mono">requirements.txt</span> e{' '}
          <span className="font-mono">package-lock.json</span> do código-fonte.
        </p>
        <ul className="mt-2 divide-y divide-border-hairline">
          {THIRD_PARTY.map((entry) => (
            <li key={entry.name} className="py-2 text-[13px]">
              <ExternalLink href={entry.url}>{entry.name}</ExternalLink>
              <span className="text-ink-soft">, {entry.what}.</span>{' '}
              <span className="text-ink-faint">{entry.license}</span>
            </li>
          ))}
        </ul>

        <div className="mt-5 flex justify-end">
          <button onClick={onClose} className="rounded-lg bg-surface-alt px-4 py-2 text-[13px] text-ink hover:brightness-95">Fechar</button>
        </div>
      </div>
    </div>
  );
}
