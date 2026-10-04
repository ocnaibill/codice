import { useState } from 'react';
import { describeError, useBackup, useHealth, useRunBackup, useVerifyBackup } from '../api/admin';
import { messageOf } from '../../../lib/serverMessage';
import { formatAge, formatBytes, formatDate } from '../format';
import { LOW_SPACE, STALE_AFTER, STUCK_AFTER } from '../systemLimits';
import { LoadError } from '../../../components/ui/LoadError';
import { Notice } from '../../../components/ui/Notice';
import { AskPassword } from './AskPassword';
import { Btn, Loading, Section } from './ui';

const COMPONENTS = {
  database: { label: 'Banco de dados', ok: 'funcionando', down: 'fora do ar: nada funciona sem ele' },
  redis: { label: 'Redis', ok: 'funcionando', off: 'não usado', down: 'fora do ar: nada se perde, os trabalhos esperam no banco' },
};

function Health() {
  const { data, isLoading, isError, error, refetch, isRefetching } = useHealth();
  return (
    <Section title="Saúde" hint="O que o servidor diz de si mesmo. A página se atualiza sozinha a cada meio minuto.">
      {isLoading && <Loading />}
      {isError && <LoadError error={error} onRetry={refetch} retrying={isRefetching}>Não foi possível perguntar ao servidor como ele está.</LoadError>}
      {data?.components && (
        <ul className="divide-y divide-border-hairline">
          {Object.entries(COMPONENTS).map(([key, c]) => {
            const state = data.components[key];
            const bad = state === 'down';
            return (
              <li key={key} className="flex items-baseline justify-between gap-3 py-2 text-[13px]">
                <span className="font-medium text-ink">{c.label}</span>
                <span className={bad ? 'text-danger' : 'text-ink-soft'}>{c[state] ?? state}</span>
              </li>
            );
          })}
        </ul>
      )}
    </Section>
  );
}

function Queue({ queue }) {
  const waited = queue.oldestWaiting ? Date.now() - new Date(queue.oldestWaiting).getTime() : 0;
  const stuck = waited > STUCK_AFTER;
  return (
    <>
      <ul className="divide-y divide-border-hairline">
        <li className="flex justify-between gap-3 py-2 text-[13px]"><span className="font-medium text-ink">Esperando</span><span className="text-ink-soft">{queue.pending}</span></li>
        <li className="flex justify-between gap-3 py-2 text-[13px]"><span className="font-medium text-ink">Em andamento</span><span className="text-ink-soft">{queue.running}</span></li>
        <li className="flex justify-between gap-3 py-2 text-[13px]">
          <span className="font-medium text-ink">Falharam nas últimas 24 horas</span>
          <span className={queue.failedRecent > 0 ? 'text-danger' : 'text-ink-soft'}>{queue.failedRecent}</span>
        </li>
      </ul>
      {stuck && (
        <Notice tone="warning" className="mt-3" title="A fila pode estar parada">
          O trabalho mais antigo está esperando para começar {formatAge(waited)}. Confira se o serviço de trabalhos (worker) está rodando.
        </Notice>
      )}
    </>
  );
}

function Space({ storage }) {
  if (!storage) return <p className="text-[13px] text-ink-faint">O servidor não soube medir o espaço.</p>;
  const free = storage.totalBytes ? storage.freeBytes / storage.totalBytes : 1;
  const used = Math.round((1 - free) * 100);
  const low = free < LOW_SPACE;
  return (
    <>
      <p className="text-[13px] text-ink">
        {formatBytes(storage.freeBytes)} livres de {formatBytes(storage.totalBytes)} ({Math.round(free * 100)}% livre)
      </p>
      <div role="img" aria-label={`${used}% usado`} className="mt-2 h-2 w-full overflow-hidden rounded-full bg-surface-alt">
        <div className={`h-full ${low ? 'bg-danger' : 'bg-brand'}`} style={{ width: `${used}%` }} />
      </div>
      {low && (
        <Notice tone="danger" className="mt-3" title="Pouco espaço livre">
          Menos de {Math.round(LOW_SPACE * 100)}% livre onde ficam os livros e as capas. Sem espaço, novos envios e backups falham.
        </Notice>
      )}
    </>
  );
}

function CopyCommand({ label, command }) {
  const [copied, setCopied] = useState(false);
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(command);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      // No clipboard (an address that is not secure): the command stays selectable on screen.
      setCopied(false);
    }
  };
  return (
    <div className="flex flex-col gap-1">
      <span className="text-[11px] uppercase tracking-wide text-ink-soft">{label}</span>
      <div className="flex items-stretch gap-2">
        <input
          readOnly
          value={command}
          aria-label={label}
          onFocus={(event) => event.target.select()}
          className="min-w-0 flex-1 rounded bg-surface px-3 py-2 font-mono text-[12px] text-ink outline-none"
        />
        <button
          type="button"
          onClick={copy}
          aria-label={`Copiar ${label.toLowerCase()}`}
          className="shrink-0 rounded bg-surface-alt px-3 text-[12px] font-medium text-ink hover:brightness-95"
        >
          {copied ? 'Copiado' : 'Copiar'}
        </button>
      </div>
    </div>
  );
}

const CREATE = 'CODICE_BACKUP_PASSPHRASE="sua frase" scripts/backup.sh /mnt/backups/codice';
const RESTORE = 'docker compose -f docker-compose.full.yml run --rm --no-deps -T -e CODICE_BACKUP_PASSPHRASE backend codice-admin restore --in - < CAMINHO-DO-PACOTE';

const DAY = 24 * 60 * 60 * 1000;
const LIVE = ['pending', 'running'];

/** What the last job of the two buttons says, while it runs and for a day after it ended; nothing when it is old news. */
function jobSentence(job, now = Date.now()) {
  if (!job) return null;
  const checking = job.type === 'verify_backup';
  if (LIVE.includes(job.state)) {
    return { tone: 'info', text: checking ? `Verificando ${job.name}: lendo o pacote e ensaiando a restauração…` : 'Fazendo o backup…' };
  }
  const endedAgo = job.finishedAt ? now - new Date(job.finishedAt).getTime() : 0;
  if (job.state !== 'failed' && endedAgo > DAY) return null;
  const when = job.finishedAt ? ` ${formatAge(endedAgo)}` : '';
  if (job.state === 'succeeded') {
    return { tone: 'success', text: checking ? `A verificação de ${job.name} terminou${when}: o ensaio de restauração passou.` : `O backup feito por aqui terminou${when}.` };
  }
  if (job.state === 'cancelled') return { tone: 'info', text: `${checking ? 'A verificação' : 'O backup'} foi cancelado.` };
  // What the server has a sentence for is said in Portuguese; what it does not (the text of pg_restore, say) is the
  // owner's to read as it came, set apart as a technical detail, so that it is never taken for the screen's own words.
  const known = messageOf(job.error);
  const detail = job.error && known === job.error ? job.error : '';
  return {
    tone: 'danger',
    text: `${checking ? `A verificação de ${job.name}` : 'O backup'} falhou${when}.${known && !detail ? ` ${known}` : ''}`,
    detail,
  };
}

function OwnerPanel({ panel, isOwner }) {
  const run = useRunBackup();
  const verify = useVerifyBackup();
  const [asking, setAsking] = useState(null); // { kind: 'run' } or { kind: 'verify', name }
  const [noted, setNoted] = useState('');
  const job = panel.job;
  const busy = job && LIVE.includes(job.state);
  const sentence = jobSentence(job);
  const action = asking?.kind === 'verify' ? verify : run;

  const close = () => { setAsking(null); run.reset(); verify.reset(); };
  const confirm = (password) => {
    const body = asking.kind === 'verify' ? { password, name: asking.name } : { password };
    action.mutate(body, {
      onSuccess: () => { setNoted(''); close(); },
      onError: (error) => {
        // Another job is already running: nothing to retry, the screen follows it.
        if (error?.response?.status === 409 && error.response.data?.job_id) {
          setNoted('Já há um trabalho de backup em andamento: acompanhe-o abaixo.');
          close();
        }
      },
    });
  };
  const failure = action.isError && !(action.error?.response?.status === 409 && action.error.response.data?.job_id)
    ? (messageOf(action.error?.response?.data) || describeError(action.error))
    : '';

  if (!panel.enabled) {
    return isOwner ? (
      <Notice tone="info" className="mt-5" title="Fazer e verificar backups por aqui">
        O servidor ainda não está preparado. Defina <code>CODICE_BACKUP_DIR</code> (a pasta dos pacotes, que o contêiner da API precisa poder escrever)
        e <code>CODICE_BACKUP_PASSPHRASE_FILE</code> (um arquivo com a frase de segurança, de pelo menos 8 caracteres, guardado fora do servidor também) e
        reinicie a API. O guia está no README do projeto.
      </Notice>
    ) : null;
  }
  const packages = panel.packages.slice(0, 8);
  return (
    <div className="mt-5 flex flex-col gap-3 border-t border-border-hairline pt-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h3 className="text-[13px] font-medium text-ink">Pacotes na pasta de backups</h3>
        {isOwner && (
          <Btn tone="primary" disabled={busy} onClick={() => setAsking({ kind: 'run' })}>Fazer um backup agora</Btn>
        )}
      </div>
      {isOwner && panel.dir && <p className="text-[12px] text-ink-faint">Pasta: <span className="break-all font-mono text-ink-soft">{panel.dir}</span></p>}
      {noted && <p role="status" className="text-[13px] text-ink-soft">{noted}</p>}
      {sentence && (
        <Notice tone={sentence.tone}>
          {sentence.text}
          {sentence.detail && <p className="mt-1 text-[12px] text-ink-soft">Detalhe técnico: <code className="break-all">{sentence.detail}</code></p>}
        </Notice>
      )}
      {packages.length === 0 ? (
        <p className="text-[13px] text-ink-faint">Nenhum pacote nesta pasta ainda.</p>
      ) : (
        <ul className="divide-y divide-border-hairline">
          {packages.map((p) => (
            <li key={p.name} className="flex flex-wrap items-center justify-between gap-2 py-2 text-[13px]">
              <div className="min-w-0">
                <p className="break-all font-mono text-ink">{p.name}</p>
                <p className="text-[12px] text-ink-faint">{formatDate(p.at)} · {formatBytes(p.bytes)}{p.encrypted ? ' · criptografado' : ''}</p>
              </div>
              {isOwner && (
                <Btn disabled={busy} onClick={() => setAsking({ kind: 'verify', name: p.name })} aria-label={`Verificar ${p.name}`}>Verificar</Btn>
              )}
            </li>
          ))}
        </ul>
      )}
      {asking && (
        <AskPassword
          title={asking.kind === 'verify' ? 'Verificar este pacote?' : 'Fazer um backup agora?'}
          message={asking.kind === 'verify' ? (
            <p>O pacote <strong className="break-all">{asking.name}</strong> será lido inteiro e a restauração será ensaiada num banco temporário, que é apagado em seguida. Nada do acervo muda. Pode levar alguns minutos.</p>
          ) : (
            <p>O servidor vai gravar, na pasta de backups, um pacote <strong>criptografado</strong> com o banco, a lista de arquivos e os próprios arquivos. Outros trabalhos esperam até ele terminar.</p>
          )}
          confirmLabel={asking.kind === 'verify' ? 'Verificar' : 'Fazer o backup'}
          busy={action.isPending}
          error={failure}
          onConfirm={confirm}
          onCancel={close}
        />
      )}
    </div>
  );
}

function Backup({ last, panel, isOwner }) {
  const age = last ? Date.now() - new Date(last.at).getTime() : 0;
  const stale = last && age > STALE_AFTER;
  return (
    <Section
      title="Backup"
      hint="O Códice não agenda backups: você agenda o comando no servidor. O pacote leva o banco e a lista de arquivos com seus hashes (e os arquivos, com --include-files). Sessões, tokens, convites e o registro de entradas nunca entram."
    >
      {!last && (
        <Notice tone="danger" title="Nenhum backup registrado nesta instância">
          Sem backup, uma falha do disco leva o acervo e as anotações. Veja abaixo o comando para fazer o primeiro.
        </Notice>
      )}
      {last && (
        <div className="flex flex-col gap-3">
          <p className="text-[13px] text-ink">
            Último backup: <strong>{formatDate(last.at)}</strong> ({formatAge(age)}), {formatBytes(last.bytes)}
            {last.includesFiles ? `, com ${last.files} arquivo(s)` : ', só banco e lista de arquivos'}
            {last.encrypted ? ', criptografado' : ', sem criptografia'}.
          </p>
          {stale && (
            <Notice tone="danger" title="O último backup é velho">
              Faz mais de um dia e meio: a meta é um por dia. Confira o agendamento no servidor.
            </Notice>
          )}
          {last.path ? (
            <p className="text-[13px] text-ink-soft">Pacote: <span className="break-all font-mono text-ink">{last.path}</span></p>
          ) : last.name ? (
            <p className="text-[13px] text-ink-soft">Pacote: <span className="break-all font-mono text-ink">{last.name}</span></p>
          ) : (
            <p className="text-[13px] text-ink-faint">
              O pacote foi enviado para fora do servidor, e o Códice não sabe onde ele está. Para registrar, acrescente <code>--as-path CAMINHO</code> ao <code>codice-admin backup</code> (o <code>scripts/backup.sh</code> já faz isso).
            </p>
          )}
          {last.verified ? (
            <p className="text-[13px] text-ink-soft">
              Verificado em {formatDate(last.verified.at)}:{' '}
              {last.verified.deep ? 'o ensaio de restauração passou.' : 'o pacote foi lido inteiro e confere. Falta o ensaio de restauração (--deep).'}
            </p>
          ) : (
            <p className="text-[13px] text-ink-faint">
              Este pacote ainda não foi verificado. Um backup que nunca foi restaurado é só uma esperança: rode <code>verify-backup --deep</code>.
            </p>
          )}
          {last.sameDisk === true && (
            <Notice tone="warning" title="O pacote está no mesmo disco do acervo">
              Se esse disco falhar, o acervo e o backup se perdem juntos. Guarde uma cópia em outro disco ou em outra máquina.
            </Notice>
          )}
          {last.sameDisk == null && (
            <p className="text-[12px] text-ink-faint">O Códice não enxerga o disco onde o pacote está: confirme que não é o mesmo do acervo.</p>
          )}
        </div>
      )}
      <div className="mt-5 flex flex-col gap-3">
        <CopyCommand label="Fazer um backup (no servidor)" command={CREATE} />
        <CopyCommand label="Restaurar um pacote (no servidor)" command={RESTORE} />
        <p className="text-[12px] text-ink-faint">
          Restaurar troca o banco que está no ar, por isso é feito no servidor e não por esta página. O guia completo, com o ensaio, está no README do projeto.
        </p>
      </div>
      {panel && <OwnerPanel panel={panel} isOwner={isOwner} />}
    </Section>
  );
}

/** "Sistema" (UI-19, DEC-123): how the server is, how much room is left, how the queue of jobs is doing and the state of the backups. */
export function SystemTab({ isOwner = false }) {
  const { data, isLoading, isError, error, refetch, isRefetching } = useBackup();
  return (
    <div className="flex flex-col gap-5">
      <Health />
      <Section title="Fila de trabalhos e espaço" hint="O que o sistema tem para fazer e quanto lugar sobra para os livros e as capas.">
        {isLoading && <Loading />}
        {isError && <LoadError error={error} onRetry={refetch} retrying={isRefetching}>Não foi possível carregar o estado do sistema.</LoadError>}
        {data && (
          <div className="grid gap-6 sm:grid-cols-2">
            <div><h3 className="mb-1 text-[12px] font-medium uppercase tracking-wide text-ink-soft">Fila</h3><Queue queue={data.queue} /></div>
            <div><h3 className="mb-1 text-[12px] font-medium uppercase tracking-wide text-ink-soft">Armazenamento</h3><Space storage={data.storage} /></div>
          </div>
        )}
      </Section>
      {data && <Backup last={data.lastBackup} panel={data.panel} isOwner={isOwner} />}
    </div>
  );
}
