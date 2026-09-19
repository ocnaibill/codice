import { useState } from 'react';
import { useLdap, useSetLdapPolicy, useCheckLdap, describeError } from '../api/admin';
import { Btn, ErrorNote, Loading, Section } from './ui';

const REASON = {
  not_configured: 'O servidor não tem o LDAP configurado.',
  unavailable: 'Não foi possível conectar, ou a conta de serviço foi recusada. Confira o endereço e a conta de serviço.',
  error: 'O teste falhou por outro motivo. Veja o log do servidor.',
};

/**
 * The owner's view of sign-in through an LDAP directory. The connection (address and
 * service account, with its secret) is set on the server and shown here without any
 * secret; what the owner decides is whether a first sign-in creates an account, and
 * how long an account may stay signed in without the directory confirming it.
 */
export function LdapTab() {
  const { data, isLoading, isError } = useLdap();
  if (isLoading) return <Loading />;
  if (isError || !data) return <ErrorNote>Não foi possível carregar a configuração.</ErrorNote>;
  return <LdapForm key={`${data.policy.allowCreate}-${data.policy.revalidateHours}`} state={data} />;
}

function LdapForm({ state }) {
  const save = useSetLdapPolicy();
  const check = useCheckLdap();
  const [allowCreate, setAllowCreate] = useState(state.policy.allowCreate);
  const [hours, setHours] = useState(state.policy.revalidateHours);

  return (
    <div className="flex flex-col gap-5">
      <Section
        title="Login pelo diretório (LDAP)"
        hint="Quem já tem conta no seu diretório, como o Authentik, entra com a mesma senha. O dono sempre entra por senha local. O endereço e a conta de serviço são definidos no servidor (variáveis LDAP_*), não aqui."
        actions={<Btn onClick={() => check.mutate()} disabled={check.isPending}>Testar conexão</Btn>}
      >
        {state.configured ? (
          <p className="text-[13px] text-ink">
            <span className="mr-2 rounded bg-green-100 px-2 py-0.5 text-[11px] text-green-800">Configurado</span>
            {state.host} · base {state.baseDN} · {state.linkedAccounts} conta(s) ligada(s)
          </p>
        ) : (
          <p className="text-[13px] text-ink-soft">
            <span className="mr-2 rounded bg-surface-alt px-2 py-0.5 text-[11px]">Desligado</span>
            Para ligar, defina <code>LDAP_URL</code>, <code>LDAP_BIND_DN</code>, <code>LDAP_BIND_PASSWORD</code> e <code>LDAP_BASE_DN</code> no servidor e reinicie a API.
          </p>
        )}
        {check.data && (
          <p role="status" className={`mt-3 text-[13px] ${check.data.ok ? 'text-green-800' : 'text-red-700'}`}>
            {check.data.ok ? 'Conexão e conta de serviço funcionando.' : REASON[check.data.reason] || REASON.error}
          </p>
        )}
        <ErrorNote>{check.isError && describeError(check.error)}</ErrorNote>
      </Section>

      <Section title="Política" hint="Estas duas decisões são suas. Ninguém vira administrador ou dono por entrar pelo diretório.">
        <div className="flex flex-col gap-3">
          <label className="flex items-start gap-2 text-[13px] text-ink">
            <input type="checkbox" className="mt-1" checked={allowCreate} disabled={!state.configured}
              onChange={(event) => setAllowCreate(event.target.checked)} />
            <span>
              Permitir criar conta no primeiro login
              <span className="block text-[12px] text-ink-faint">
                Quem tem conta no diretório e ainda não tem no Códice entra e ganha uma conta de leitor. Desligado, só quem já tem conta ou convite entra.
              </span>
            </span>
          </label>
          <label className="flex flex-wrap items-center gap-2 text-[13px] text-ink">
            Reconfirmar a identidade no diretório pelo menos a cada
            <input type="number" min="1" max="336" value={hours} aria-label="Horas até reconfirmar"
              onChange={(event) => setHours(Number(event.target.value))} className="w-20 rounded bg-surface px-2 py-1 text-[13px]" />
            hora(s)
          </label>
          <p className="text-[12px] text-ink-faint">
            Quem foi desativado no diretório perde o acesso no máximo depois desse prazo. Bloquear a conta aqui vale na hora.
          </p>
          <div>
            <Btn tone="primary" onClick={() => save.mutate({ allowCreate, revalidateHours: hours })} disabled={save.isPending}>Salvar política</Btn>
          </div>
          {save.isSuccess && <p role="status" className="text-[13px] text-ink-soft">Política salva.</p>}
          <ErrorNote>{save.isError && describeError(save.error)}</ErrorNote>
        </div>
      </Section>
    </div>
  );
}
