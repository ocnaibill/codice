import { useState } from 'react';
import { formatBytes } from '../format';
import { coverageLabel, isBig, matchesSearch, orderPackages, percent, progressLine, readyLine, sizeLine } from '../dictionaryText';
import { describeError, useCancelDictionary, useDictionaries, useInstallDictionary, useRemoveDictionary } from '../api/admin';
import { Btn, ErrorNote, Loading, Section } from './ui';
import { LoadError } from '../../../components/ui/LoadError';
import { PermissionNote } from '../../../components/ui/PermissionNote';
import { ConfirmDialog } from './ConfirmDialog';

/** The warning that comes before any installation: the file is not the project's, and the owner is the one who downloads it. */
function ThirdParty() {
  return (
    <div role="note" className="mt-4 rounded-lg border border-border-hairline bg-surface px-4 py-3 text-[13px] leading-relaxed text-ink-soft">
      <p className="font-medium text-ink">Estes arquivos não são do Códice.</p>
      <p className="mt-1">
        São de terceiros: o Wikcionário, o dicionário livre mantido por voluntários, extraído com o Wiktextract e publicado em
        kaikki.org. O Códice não traz nem hospeda nenhum dicionário. Ao clicar em <strong>Instalar</strong>, o <strong>seu servidor</strong>{' '}
        baixa o arquivo de lá, por sua conta, sob a licença CC BY-SA e GFDL (o cartão do verbete mostra a fonte). A origem é pública e muito
        usada, e o Códice só baixa de um endereço fixo da lista abaixo, nunca de um que alguém digite. Nenhuma palavra que você consulta
        sai do seu servidor.
      </p>
    </div>
  );
}

function Languages({ languages }) {
  return (
    <ul className="mt-2 flex flex-wrap gap-1.5" aria-label="Idiomas que traz">
      {languages.map((language) => (
        <li
          key={language.code}
          className={`rounded-full px-2.5 py-0.5 text-[11px] ${language.level === 'complete' ? 'bg-brand/10 text-brand' : 'bg-surface-alt text-ink-soft'}`}
        >
          {coverageLabel(language)}
        </li>
      ))}
    </ul>
  );
}

function Progress({ pkg }) {
  const done = percent(pkg.progress);
  return (
    <div className="mt-3">
      <div
        role="progressbar"
        aria-label={`Andamento da instalação de ${pkg.name}`}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={done}
        className="h-2 overflow-hidden rounded-full bg-surface-alt"
      >
        <div className="h-full rounded-full bg-brand transition-[width] duration-500" style={{ width: `${done}%` }} />
      </div>
      <p className="mt-1.5 text-[12px] text-ink-soft" aria-live="polite">{progressLine(pkg)}</p>
    </div>
  );
}

function Package({ pkg, isOwner, ask }) {
  const installing = pkg.state === 'installing';
  const ready = pkg.state === 'ready';
  const failed = pkg.state === 'failed';
  return (
    <li aria-label={pkg.name} className="rounded-xl bg-white p-5 shadow-[0px_1px_8px_0px_rgba(0,0,0,0.05)]">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <h3 className="font-display text-lg font-semibold text-ink">{pkg.name}</h3>
          <p className="mt-1 max-w-2xl text-[13px] text-ink-soft">{pkg.description}</p>
        </div>
        <div className="flex flex-wrap gap-2">
          {!pkg.installable && <span className="rounded-full bg-surface-alt px-3 py-1 text-[12px] text-ink-faint">Em breve</span>}
          {pkg.installable && isOwner && !installing && !ready && (
            <Btn tone="primary" onClick={() => ask(pkg, 'install')}>{failed ? 'Tentar de novo' : 'Instalar'}</Btn>
          )}
          {pkg.installable && isOwner && ready && <Btn onClick={() => ask(pkg, 'update')}>Atualizar</Btn>}
          {pkg.installable && isOwner && ready && <Btn tone="danger" onClick={() => ask(pkg, 'remove')}>Remover</Btn>}
          {pkg.installable && isOwner && installing && <Btn onClick={() => ask(pkg, 'cancel')}>Cancelar</Btn>}
        </div>
      </div>
      <Languages languages={pkg.languages} />
      <p className="mt-3 text-[12px] text-ink-soft">{sizeLine(pkg)}</p>
      <p className="mt-1 text-[12px] text-ink-faint">
        Licença: <a href={pkg.licenseUrl} target="_blank" rel="noopener noreferrer" className="underline">{pkg.license}</a>
        {' · '}Fonte: <a href={pkg.sourceUrl} target="_blank" rel="noopener noreferrer" className="underline">{pkg.source}</a>
      </p>
      {installing && <Progress pkg={pkg} />}
      {ready && <p className="mt-3 text-[13px] text-success">{readyLine(pkg)}</p>}
      {failed && <ErrorNote>{pkg.stage === 'cancelled' ? 'A instalação foi cancelada.' : `A instalação falhou: ${pkg.error || 'motivo desconhecido'}`}</ErrorNote>}
    </li>
  );
}

const QUESTIONS = {
  install: (pkg) => ({
    title: `Instalar ${pkg.name}?`,
    message: (
      <>
        <p>
          O seu servidor vai baixar <strong>{formatBytes(pkg.downloadBytes)}</strong> de kaikki.org (um arquivo de terceiros, sob a licença{' '}
          {pkg.license}) e importar no banco{pkg.storageBytes ? <>, onde passa a ocupar cerca de <strong>{formatBytes(pkg.storageBytes)}</strong></> : null}.
        </p>
        <p className="mt-2">Leva alguns minutos, em segundo plano, e você pode usar o Códice enquanto isso. Dá para cancelar a qualquer momento.</p>
        {isBig(pkg) && (
          <p className="mt-2 font-medium text-ink">
            É um arquivo grande: pode levar bem mais tempo para baixar e para importar, e ocupar bastante espaço no banco.
          </p>
        )}
      </>
    ),
    choices: [{ label: 'Baixar e instalar', value: 'go', tone: 'primary' }],
  }),
  update: (pkg) => ({
    title: `Atualizar ${pkg.name}?`,
    message: (
      <p>
        O servidor vai baixar o arquivo de novo ({formatBytes(pkg.downloadBytes)}). O dicionário atual continua valendo até o novo estar inteiro; se
        algo falhar, ele não muda.
      </p>
    ),
    choices: [{ label: 'Baixar de novo', value: 'go', tone: 'primary' }],
  }),
  remove: (pkg) => ({
    title: `Remover ${pkg.name}?`,
    message: <p>Os verbetes saem do banco e a consulta de palavras deixa de funcionar nesses idiomas. Dá para instalar de novo quando quiser.</p>,
    choices: [{ label: 'Remover', value: 'go', tone: 'danger' }],
  }),
  cancel: (pkg) => ({
    title: `Cancelar a instalação de ${pkg.name}?`,
    message: <p>O que já foi baixado ou lido é descartado. Se havia uma versão instalada, ela continua valendo. O worker pode levar alguns segundos para parar.</p>,
    choices: [{ label: 'Cancelar a instalação', value: 'go', tone: 'danger' }],
    cancelLabel: 'Continuar instalando',
  }),
};

export function DictionariesTab({ isOwner }) {
  const { data, isLoading, isError, error, refetch, isRefetching } = useDictionaries();
  const install = useInstallDictionary();
  const cancel = useCancelDictionary();
  const remove = useRemoveDictionary();
  const [question, setQuestion] = useState(null);
  const [search, setSearch] = useState('');

  const run = { install, update: install, cancel, remove };
  const choose = async () => {
    const { pkg, kind } = question;
    setQuestion(null);
    try {
      await run[kind].mutateAsync(pkg.id);
    } catch {
      // the screen says why, below
    }
  };
  const failure = [install, cancel, remove].find((m) => m.isError);

  if (isLoading) return <Loading />;
  if (isError || !data) return <LoadError error={error} onRetry={refetch} retrying={isRefetching}>Não foi possível carregar os dicionários.</LoadError>;
  const dialog = question && QUESTIONS[question.kind](question.pkg);
  const shown = orderPackages(data.packages).filter((pkg) => matchesSearch(pkg, search));
  return (
    <>
      <Section
        title="Dicionários"
        hint="Para consultar uma palavra no texto, o leitor precisa de um dicionário instalado. Cada pacote traz definições em um idioma e a tradução de outros."
      >
        <ThirdParty />
        {!isOwner && <PermissionNote className="mt-3">Só o dono do acervo instala, atualiza e remove dicionários.</PermissionNote>}
        <div className="mt-4 flex flex-wrap items-center gap-3">
          <input
            type="search"
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            placeholder="Buscar um idioma (por exemplo, japonês)"
            aria-label="Buscar um idioma"
            className="min-h-11 w-full max-w-sm rounded-lg border border-border-hairline bg-white px-3 text-[14px] text-ink outline-none focus:border-brand"
          />
          <span className="text-[12px] text-ink-faint" aria-live="polite">
            {search.trim() ? `${shown.length} de ${data.packages.length} dicionários` : `${data.packages.length} dicionários`}
          </span>
        </div>
        <ErrorNote>{failure && describeError(failure.error)}</ErrorNote>
      </Section>
      {shown.length === 0 && (
        <p className="mt-4 text-[13px] text-ink-soft">
          Nenhum dicionário para “{search.trim()}”. Cada Wikcionário traz as definições no próprio idioma; os que o Códice sabe instalar estão na lista inteira (apague a busca).
        </p>
      )}
      <ul className="mt-4 flex flex-col gap-3">
        {shown.map((pkg) => (
          <Package key={pkg.id} pkg={pkg} isOwner={isOwner} ask={(p, kind) => setQuestion({ pkg: p, kind })} />
        ))}
      </ul>
      {dialog && <ConfirmDialog title={dialog.title} message={dialog.message} choices={dialog.choices} cancelLabel={dialog.cancelLabel} onChoose={choose} onCancel={() => setQuestion(null)} />}
    </>
  );
}
