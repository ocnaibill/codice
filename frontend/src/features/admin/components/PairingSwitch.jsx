import { usePairingEnabled, useSetPairingEnabled } from '../../auth/api/useDevicePairing';
import { serverMessage } from '../../../lib/serverMessage';
import { Section } from './ui';

/** The owner's switch for connecting readers by code (DEC-194): on unless turned off. Turned off, no new code is given; the accesses that exist stay. */
export function PairingSwitch() {
  const { data } = usePairingEnabled();
  const save = useSetPairingEnabled();
  if (!data) return null;
  return (
    <Section title="Conectar leitores por código" hint="O plugin do Códice no Kindle mostra um código, e a pessoa o aprova no site, sem copiar senha nem arquivos.">
      <label className="flex items-start gap-2 text-[13px] text-ink">
        <input type="checkbox" className="mt-1" checked={data.enabled} disabled={save.isPending} onChange={(event) => save.mutate(event.target.checked)} />
        <span>
          Permitir conectar leitores por código
          <span className="block text-[12px] text-ink-faint">Desligado, nenhum leitor novo se conecta assim (os acessos que já existem continuam funcionando, e os arquivos de configuração seguem disponíveis em Aplicativos).</span>
        </span>
      </label>
      {save.isError && <p role="alert" className="mt-2 text-[13px] text-danger">{serverMessage(save.error, 'Não foi possível salvar.')}</p>}
    </Section>
  );
}
