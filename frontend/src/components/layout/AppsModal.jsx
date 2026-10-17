import { useRef, useState } from 'react';
import { useMe } from '../../features/auth/api/useMe';
import {
  catalogAddress,
  isLocalAddress,
  koreaderOpdsFile,
  koreaderSyncFile,
  kosyncAddress,
  useAppTokens,
  useCreateAppToken,
  useRevokeAppToken,
} from '../../features/auth/api/useAppTokens';
import { ConfirmDialog } from '../../features/admin/components/ConfirmDialog';
import { formatDate } from '../../features/admin/format';
import { ago } from '../../lib/ago';
import { useDialog } from '../../lib/useDialog';

// Names to start from, so the usual case is one click; the person can write any other.
const SUGGESTIONS = ['KOReader', 'Moon+ Reader', 'Librera', 'Meu leitor'];

function CopyRow({ label, value, secret }) {
  const [copied, setCopied] = useState(false);
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(value);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      // No clipboard (an address that is not secure): the value stays selectable on screen.
      setCopied(false);
    }
  };
  return (
    <div className="flex flex-col gap-1">
      <span className="text-[11px] uppercase tracking-wide text-ink-soft">{label}</span>
      <div className="flex items-stretch gap-2">
        <input
          readOnly
          value={value}
          aria-label={label}
          onFocus={(event) => event.target.select()}
          className={`min-w-0 flex-1 rounded bg-surface px-3 py-2 text-[13px] text-ink outline-none ${secret ? 'font-mono' : ''}`}
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

/** Hands the person a file made in the browser, with the token inside: it is the only time the token exists in the clear. */
function downloadText(name, text) {
  const url = URL.createObjectURL(new Blob([text], { type: 'text/plain;charset=utf-8' }));
  const link = document.createElement('a');
  link.href = url;
  link.download = name;
  document.body.appendChild(link);
  link.click();
  link.remove();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}

/** The short token of KOReader (DEC-190): said to be typed without the spaces, and offered as a file so nothing has to be typed. */
function KoreaderConnect({ created, username, onDone }) {
  const address = catalogAddress();
  return (
    <div role="status" className="rounded-lg border border-brand/30 bg-brand/5 p-4">
      <p className="text-[14px] font-medium text-ink">Pronto: o acesso “{created.name}” foi criado.</p>
      <p className="mt-1 text-[13px] text-ink-soft">
        Esta senha é curta para digitar na tela do leitor: são 16 letras e números, sem 0, 1, i, l e o. Pode digitar sem os espaços, e
        maiúscula ou minúscula tanto faz.
      </p>
      <p aria-label="Senha do KOReader" className="mt-3 select-all rounded bg-surface px-3 py-3 text-center font-mono text-[22px] tracking-widest text-ink">
        {created.token}
      </p>
      <div className="mt-4 flex flex-col gap-3">
        <CopyRow label="Endereço" value={address} />
        <CopyRow label="Usuário" value={username} />
      </div>
      <div className="mt-4 rounded bg-surface p-3">
        <p className="text-[13px] font-medium text-ink">Sem digitar nada</p>
        <p className="mt-1 text-[13px] text-ink-soft">
          Baixe os dois arquivos abaixo e copie para a pasta <strong>koreader/settings</strong> do aparelho, por USB, com o KOReader fechado.
          O catálogo do Códice e a sincronização do progresso de leitura aparecem já configurados.
        </p>
        <ul className="mt-1 list-disc pl-5 text-[12px] text-ink-soft">
          <li><code>opds.lua</code>: o catálogo, para baixar livros.</li>
          <li><code>kosync.lua</code>: a sincronização do ponto onde você parou, ligada. O KOReader pergunta antes de pular para um ponto mais novo.</li>
        </ul>
        <p className="mt-1 text-[12px] text-warning">
          Cada arquivo substitui o que já estiver lá com o mesmo nome. Se você tem outros catálogos, guarde uma cópia do <code>opds.lua</code>{' '}
          do aparelho ou adicione este à mão.
        </p>
        <div className="mt-2 flex flex-wrap gap-2">
          <button
            type="button"
            onClick={() => downloadText('opds.lua', koreaderOpdsFile({ address, username, token: created.token }))}
            className="rounded bg-surface-alt px-3 py-2 text-[13px] font-medium text-ink hover:brightness-95"
          >
            Baixar opds.lua
          </button>
          <button
            type="button"
            onClick={() => downloadText('kosync.lua', koreaderSyncFile({ address: kosyncAddress(), username, token: created.token }))}
            className="rounded bg-surface-alt px-3 py-2 text-[13px] font-medium text-ink hover:brightness-95"
          >
            Baixar kosync.lua
          </button>
        </div>
      </div>
      {isLocalAddress() && (
        <p className="mt-3 rounded bg-warning-soft/50 p-3 text-[13px] text-warning">
          Você abriu o Códice por <strong>{window.location.hostname}</strong>, que só funciona neste computador. No leitor, troque esse
          trecho do endereço pelo endereço do computador onde o Códice está, na sua rede.
        </p>
      )}
      <p className="mt-3 text-[13px] font-medium text-warning">
        Esta senha aparece só agora e não é a senha da sua conta. Se perder, crie outro acesso e remova este.
      </p>
      <div className="mt-4 flex justify-end">
        <button onClick={onDone} className="rounded bg-brand px-4 py-2 text-[13px] text-white hover:brightness-110">
          Já copiei
        </button>
      </div>
    </div>
  );
}

/** Shown once, right after the access is created: the three things to type into the app, each with a copy button. */
function Connect({ created, username, onDone }) {
  if (created.scope === 'koreader') return <KoreaderConnect created={created} username={username} onDone={onDone} />;
  return (
    <div role="status" className="rounded-lg border border-brand/30 bg-brand/5 p-4">
      <p className="text-[14px] font-medium text-ink">Pronto: o acesso “{created.name}” foi criado.</p>
      <p className="mt-1 text-[13px] text-ink-soft">
        No aplicativo, procure por <strong>catálogo OPDS</strong> (ou “biblioteca online”), adicione um novo e preencha estes três campos.
        Se ele pedir um nome, pode ser qualquer um.
      </p>
      <div className="mt-4 flex flex-col gap-3">
        <CopyRow label="Endereço" value={catalogAddress()} />
        <CopyRow label="Usuário" value={username} />
        <CopyRow label="Senha" value={created.token} secret />
      </div>
      {isLocalAddress() && (
        <p className="mt-3 rounded bg-warning-soft/50 p-3 text-[13px] text-warning">
          Você abriu o Códice por <strong>{window.location.hostname}</strong>, que só funciona neste computador. No celular ou no
          leitor, troque esse trecho do endereço pelo endereço do computador onde o Códice está, na sua rede. O usuário e a senha
          são os mesmos.
        </p>
      )}
      <p className="mt-3 text-[13px] font-medium text-warning">
        Esta senha aparece só agora e não é a senha da sua conta. Se perder, crie outro acesso e remova este.
      </p>
      <div className="mt-4 flex justify-end">
        <button onClick={onDone} className="rounded bg-brand px-4 py-2 text-[13px] text-white hover:brightness-110">
          Já copiei
        </button>
      </div>
    </div>
  );
}

function CreateForm({ onCreate, busy, error }) {
  const [name, setName] = useState(SUGGESTIONS[0]);
  // KOReader is read on e-readers, where a long password is hard to type (DEC-190): its chip asks for the short one.
  const [short, setShort] = useState(true);
  const choose = (suggestion) => {
    setName(suggestion);
    setShort(suggestion === 'KOReader');
  };
  return (
    <form
      onSubmit={(event) => {
        event.preventDefault();
        if (name.trim()) onCreate({ name: name.trim(), scope: short ? 'koreader' : 'app' });
      }}
      className="flex flex-col gap-3"
    >
      <label className="flex flex-col gap-1 text-[13px] font-medium text-ink">
        Qual aplicativo vai usar?
        <input
          value={name}
          onChange={(event) => setName(event.target.value)}
          maxLength={100}
          required
          disabled={busy}
          className="rounded bg-surface px-3 py-2 text-[14px] font-normal text-ink outline-none"
        />
      </label>
      <div className="flex flex-wrap gap-2" aria-label="Sugestões de nome">
        {SUGGESTIONS.map((suggestion) => (
          <button
            key={suggestion}
            type="button"
            onClick={() => choose(suggestion)}
            className="rounded-full bg-surface-alt px-3 py-1 text-[12px] text-ink hover:brightness-95"
          >
            {suggestion}
          </button>
        ))}
      </div>
      <label className="flex items-start gap-2 text-[13px] text-ink">
        <input type="checkbox" checked={short} onChange={(event) => setShort(event.target.checked)} disabled={busy} className="mt-1" />
        <span>
          Senha curta, para digitar na tela de um leitor (KOReader no Kindle)
          <span className="block text-[12px] text-ink-faint">16 letras e números. Sem marcar, a senha é longa, boa para copiar e colar.</span>
        </span>
      </label>
      <p className="text-[12px] text-ink-faint">O nome serve só para você reconhecer o acesso depois, na lista.</p>
      {error && <p role="alert" className="text-[13px] text-danger">{error}</p>}
      <div>
        <button type="submit" disabled={busy || !name.trim()} className="rounded bg-brand px-4 py-2 text-[13px] font-medium text-white hover:brightness-110 disabled:opacity-50">
          {busy ? 'Criando…' : 'Criar acesso'}
        </button>
      </div>
    </form>
  );
}

/**
 * Reading apps that speak OPDS (KOReader, Moon+ Reader…) cannot use the password of the account, which the
 * Códice does not accept there (DEC-071): each is given a password of its own, which only reads, can be taken
 * back at any time, and is shown once. A person sees and removes only their own.
 */
export function AppsModal({ onClose }) {
  const dialogRef = useRef(null);
  const { data: me } = useMe();
  const { data: tokens, isLoading, isError } = useAppTokens();
  const create = useCreateAppToken();
  const revoke = useRevokeAppToken();
  const [created, setCreated] = useState(null);
  const [removing, setRemoving] = useState(null);

  useDialog(dialogRef, { onEscape: onClose });

  const submit = (request) => create.mutate(request, { onSuccess: (data) => setCreated(data) });

  return (
    <div
      className="fixed inset-0 z-50 flex animate-fade-in items-center justify-center bg-black/50 p-4"
      onClick={(event) => event.target === event.currentTarget && onClose()}
    >
      <div ref={dialogRef} role="dialog" aria-modal="true" aria-label="Aplicativos" className="max-h-[92vh] w-full max-w-lg overflow-y-auto animate-pop-in rounded-xl bg-white p-6 shadow-2xl">
        <h2 className="font-display text-xl font-semibold text-ink">Aplicativos</h2>
        <p className="mt-1 text-[13px] text-ink-soft">
          Para ler seu acervo em outro aplicativo (KOReader, Moon+ Reader, Librera…), crie um acesso para ele. É uma senha só
          dele, que serve apenas para ler. Assim você não entrega a senha da sua conta ao aplicativo e pode cortar o acesso quando quiser.
        </p>

        <div className="mt-5">
          {created ? (
            <Connect created={created} username={me?.username ?? ''} onDone={() => setCreated(null)} />
          ) : (
            <CreateForm
              onCreate={submit}
              busy={create.isPending}
              error={create.isError ? 'Não foi possível criar o acesso.' : ''}
            />
          )}
        </div>

        <h3 className="mt-6 text-[13px] font-medium text-ink">Acessos que você já criou</h3>
        {isLoading && <p className="py-3 text-[13px] text-ink-faint">Carregando…</p>}
        {isError && <p role="alert" className="py-3 text-[13px] text-danger">Não foi possível carregar a lista.</p>}
        {!isLoading && !isError && tokens?.length === 0 && (
          <p className="py-3 text-[13px] text-ink-faint">Nenhum ainda. Quando criar, ele aparece aqui.</p>
        )}
        <ul className="divide-y divide-border-hairline">
          {(tokens || []).map((token) => (
            <li key={token.id} className="flex items-center justify-between gap-3 py-3">
              <div className="min-w-0">
                <p className="truncate text-[14px] font-medium text-ink">
                  {token.name}
                  {token.scope === 'koreader' && <span className="ml-2 rounded bg-surface-alt px-1.5 py-0.5 font-mono text-[10px] font-normal text-ink-soft">senha curta</span>}
                </p>
                <p className="text-[12px] text-ink-faint">
                  Criado em {formatDate(token.createdAt)} ·{' '}
                  {token.lastUsedAt
                    ? `usado pela última vez em ${formatDate(token.lastUsedAt)}${token.scope === 'koreader' && ago(token.lastUsedAt) ? ` (${ago(token.lastUsedAt)})` : ''}`
                    : 'ainda não usado'}
                </p>
              </div>
              <button
                onClick={() => setRemoving(token)}
                aria-label={`Remover o acesso ${token.name}`}
                className="shrink-0 rounded bg-surface-alt px-3 py-1.5 text-[12px] text-danger hover:brightness-95"
              >
                Remover
              </button>
            </li>
          ))}
        </ul>
        {revoke.isError && <p role="alert" className="mt-2 text-[13px] text-danger">Não foi possível remover o acesso.</p>}

        <p className="mt-4 text-[12px] text-ink-faint">
          Ainda estamos testando com cada aplicativo. Se algum não conectar, avise: o endereço, o usuário e a senha são tudo o que ele precisa.
        </p>
        <div className="mt-5 flex justify-end">
          <button onClick={onClose} className="rounded-lg bg-surface-alt px-4 py-2 text-[13px] text-ink hover:brightness-95">Fechar</button>
        </div>
      </div>

      {removing && (
        <ConfirmDialog
          title={`Remover o acesso “${removing.name}”?`}
          message={<p>O aplicativo perde o acesso na hora. Para voltar a usá-lo, crie outro acesso e preencha os dados de novo nele.</p>}
          choices={[{ label: 'Remover acesso', value: true, tone: 'danger' }]}
          onChoose={() => { revoke.mutate(removing.id); setRemoving(null); }}
          onCancel={() => setRemoving(null)}
        />
      )}
    </div>
  );
}
