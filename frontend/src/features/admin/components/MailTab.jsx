import { useState } from 'react';
import { useMailSettings, useSetMail, useTestMail, describeError } from '../api/admin';
import { sendFailure } from '../../auth/api/useKindle';
import { Btn, ErrorNote, Loading, Section } from './ui';

const SECURITY = [
  ['starttls', 'STARTTLS (porta 587, o mais comum)'],
  ['tls', 'TLS direto (porta 465)'],
  ['none', 'Nenhuma (só em rede de casa, sem senha)'],
];

const inputClass = 'rounded bg-surface px-2 py-1 text-[13px]';

/**
 * The owner's tab for the mail server that sends books to the e-readers (DEC-059: optional, off until set). The password is kept
 * sealed by the server and never comes back: the field only says whether one is kept.
 */
export function MailTab() {
  const { data, isLoading, isError } = useMailSettings();
  if (isLoading) return <Loading />;
  if (isError || !data) return <ErrorNote>Não foi possível carregar a configuração.</ErrorNote>;
  return <MailForm key={JSON.stringify([data.enabled, data.host, data.port, data.security, data.username, data.from, data.insecureTLS, data.maxMB, data.hasPassword])} state={data} />;
}

function MailForm({ state }) {
  const save = useSetMail();
  const test = useTestMail();
  const [enabled, setEnabled] = useState(state.enabled);
  const [host, setHost] = useState(state.host);
  const [port, setPort] = useState(state.port);
  const [security, setSecurity] = useState(state.security);
  const [username, setUsername] = useState(state.username);
  const [password, setPassword] = useState('');
  const [dropPassword, setDropPassword] = useState(false);
  const [from, setFrom] = useState(state.from);
  const [insecureTLS, setInsecureTLS] = useState(state.insecureTLS);
  const [maxMB, setMaxMB] = useState(state.maxMB);
  const [to, setTo] = useState('');

  const submit = (event) => {
    event.preventDefault();
    const body = { enabled, host, port: Number(port), security, username, from, insecureTLS, maxMB: Number(maxMB) };
    if (dropPassword) body.password = '';
    else if (password) body.password = password;
    save.mutate(body);
  };

  return (
    <div className="flex flex-col gap-5">
      <Section
        title="Enviar livros por e-mail"
        hint="Quem tem um Kindle (ou outro leitor que receba livros por e-mail) manda o livro da página da obra direto para ele. Fica desligado até você dizer qual servidor de e-mail usar."
      >
        <form className="flex flex-col gap-3" onSubmit={submit}>
          <label className="flex items-start gap-2 text-[13px] text-ink">
            <input type="checkbox" className="mt-1" checked={enabled} onChange={(event) => setEnabled(event.target.checked)} />
            <span>Ligar o envio por e-mail</span>
          </label>
          <div className="flex flex-wrap gap-3">
            <label className="flex flex-col gap-1 text-[13px] text-ink">
              Servidor (SMTP)
              <input value={host} onChange={(event) => setHost(event.target.value)} placeholder="smtp.exemplo.com" className={`${inputClass} w-64`} />
            </label>
            <label className="flex flex-col gap-1 text-[13px] text-ink">
              Porta
              <input type="number" min="1" max="65535" value={port} onChange={(event) => setPort(event.target.value)} className={`${inputClass} w-24`} />
            </label>
            <label className="flex flex-col gap-1 text-[13px] text-ink">
              Proteção da conexão
              <select value={security} onChange={(event) => setSecurity(event.target.value)} className={inputClass}>
                {SECURITY.map(([value, label]) => <option key={value} value={value}>{label}</option>)}
              </select>
            </label>
          </div>
          <div className="flex flex-wrap gap-3">
            <label className="flex flex-col gap-1 text-[13px] text-ink">
              Usuário
              <input value={username} onChange={(event) => setUsername(event.target.value)} autoComplete="off" className={`${inputClass} w-64`} />
            </label>
            <label className="flex flex-col gap-1 text-[13px] text-ink">
              Senha
              <input type="password" value={password} onChange={(event) => { setPassword(event.target.value); setDropPassword(false); }}
                autoComplete="new-password" placeholder={state.hasPassword && !dropPassword ? 'mantida (deixe vazio para manter)' : ''}
                disabled={!state.masterKey} className={`${inputClass} w-64`} />
            </label>
          </div>
          {state.hasPassword && (
            <label className="flex items-start gap-2 text-[12px] text-ink-soft">
              <input type="checkbox" className="mt-0.5" checked={dropPassword} onChange={(event) => { setDropPassword(event.target.checked); if (event.target.checked) setPassword(''); }} />
              <span>Tirar a senha guardada</span>
            </label>
          )}
          {!state.masterKey && (
            <p role="note" className="text-[12px] text-danger">
              Para guardar a senha, o servidor precisa da chave mestra (<code>CODICE_SECRET_KEY</code>, 32 caracteres ou mais). Sem ela só dá para usar um servidor sem senha.
            </p>
          )}
          <label className="flex flex-col gap-1 text-[13px] text-ink">
            Remetente
            <input value={from} onChange={(event) => setFrom(event.target.value)} placeholder="codice@exemplo.com" className={`${inputClass} w-64`} />
            <span className="text-[12px] text-ink-faint">
              A Amazon só aceita livros de endereços aprovados: em Conteúdo e dispositivos → Preferências → Documentos pessoais, inclua este endereço na lista de remetentes aprovados.
            </span>
          </label>
          <label className="flex flex-wrap items-center gap-2 text-[13px] text-ink">
            Tamanho máximo de um livro
            <input type="number" min="1" max="50" value={maxMB} aria-label="Tamanho máximo em MB" onChange={(event) => setMaxMB(event.target.value)} className={`${inputClass} w-20`} />
            MB
            <span className="text-[12px] text-ink-faint">(a Amazon aceita até 50; muitos servidores de e-mail, só 25)</span>
          </label>
          <label className="flex items-start gap-2 text-[13px] text-ink">
            <input type="checkbox" className="mt-1" checked={insecureTLS} onChange={(event) => setInsecureTLS(event.target.checked)} />
            <span>
              Aceitar certificado sem validação
              <span className="block text-[12px] text-ink-faint">Só para um servidor da sua rede com certificado próprio. Com o servidor de um provedor, deixe desligado.</span>
            </span>
          </label>
          <div>
            <Btn tone="primary" type="submit" disabled={save.isPending}>Salvar</Btn>
          </div>
          {save.isSuccess && <p role="status" className="text-[13px] text-ink-soft">Configuração salva.</p>}
          <ErrorNote>{save.isError && describeError(save.error)}</ErrorNote>
        </form>
      </Section>

      <Section title="Testar" hint="Manda uma mensagem curta, pelo servidor salvo, para o endereço que você digitar.">
        <div className="flex flex-wrap items-end gap-3">
          <label className="flex flex-col gap-1 text-[13px] text-ink">
            Enviar o teste para
            <input type="email" value={to} onChange={(event) => setTo(event.target.value)} placeholder="voce@exemplo.com" className={`${inputClass} w-64`} />
          </label>
          <Btn onClick={() => test.mutate(to)} disabled={test.isPending || !to.trim() || !state.enabled}>Enviar teste</Btn>
        </div>
        {!state.enabled && <p className="mt-2 text-[12px] text-ink-faint">Salve com o envio ligado para poder testar.</p>}
        {test.isSuccess && <p role="status" className="mt-2 text-[13px] text-success">Mensagem enviada. Confira a caixa de entrada.</p>}
        <ErrorNote>{test.isError && sendFailure(test.error, 'Não foi possível enviar o teste.')}</ErrorNote>
      </Section>
    </div>
  );
}
