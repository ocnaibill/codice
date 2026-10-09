import { useState } from 'react';
import { describeError, useMetadataProviders, useSetMetadataProvider } from '../api/admin';
import { ConfirmDialog } from './ConfirmDialog';
import { Empty, ErrorNote, Loading, Section } from './ui';
import { LoadError } from '../../../components/ui/LoadError';
import { PermissionNote } from '../../../components/ui/PermissionNote';

// What each provider is, where it lives, and what asking it hands over: the owner decides knowing (DEC-045).
const ABOUT = {
  google_books: { host: 'googleapis.com (Google)', keyEnv: 'GOOGLE_BOOKS_API_KEY' },
  openlibrary: { host: 'openlibrary.org (Internet Archive)', note: 'Também é a fonte das chaves de autoridade dos autores.' },
  comicvine: { host: 'comicvine.gamespot.com', keyEnv: 'COMICVINE_API_KEY' },
  anilist: { host: 'graphql.anilist.co (AniList)', note: 'Para mangá: autores, gêneros, ano, sinopse e capa da série. Uso gratuito não comercial; não guardamos mais do que você aceita.' },
  mangadex: { host: 'api.mangadex.org (MangaDex)', note: 'Para mangá: autores, gêneros, público (seinen, shounen…), sinopse e capa da série. Política de uso: não comercial.' },
  wikidata: { host: 'www.wikidata.org (Wikimedia)', note: 'Diz que obra é: traduz um título que os outros não conhecem (“A Nuvem” é Thunderhead), e acrescenta o identificador, a série e os gêneros em português. Dados em domínio público (CC0).' },
  wikipedia: { host: '*.wikipedia.org (Wikimedia)', note: 'Completa a sinopse com o resumo da página da obra, quando nenhum outro provedor trouxe uma; a fonte e a licença (CC BY-SA 4.0) ficam escritas junto do texto. Só funciona com o Wikidata ligado, que diz qual é a página.' },
};
const SENDS = {
  title: 'o título da obra',
  page_title: 'só o título da página da obra na Wikipédia, que o Wikidata informou (nunca o título do arquivo)',
  author_key: 'a chave de cada autor que você aceita (para obter os identificadores dele: Wikidata, VIAF, ISNI)',
};

/** What is known of the API key of a provider: the worker says whether it has one, never the key. The key is set
 *  in the environment of the worker, not here. */
function keyNote(provider) {
  const env = ABOUT[provider.id]?.keyEnv;
  if (!provider.key || !env) return null;
  if (provider.keyConfigured === true) {
    return provider.key === 'optional' ? 'Chave de API configurada: o limite de uso é maior.' : 'Chave de API configurada.';
  }
  if (provider.keyConfigured === false) {
    return provider.key === 'optional'
      ? `Sem chave de API: funciona, com limite de uso menor. Para aumentar, defina ${env} no ambiente do worker.`
      : `Falta a chave de API: sem ela o ${provider.name} não funciona. Defina ${env} no ambiente do worker e reinicie-o.`;
  }
  return provider.key === 'required' ? 'O worker ainda não informou se a chave de API existe.' : null;
}

// A provider that cannot work without a key the worker does not have is not turned on to find out later.
const blocked = (provider) => provider.key === 'required' && provider.keyConfigured === false && !provider.enabled;

const sendsText = (provider) => provider.sends.map((s) => SENDS[s] || s).join(', ');

/**
 * Which external services may be asked for suggestions about a work (#68, DEC-045). They are all off until the
 * owner turns each on: asking one sends the title of the work to a third party. Owner and admin see the choice;
 * only the owner changes it.
 */
export function ProvidersTab({ isOwner }) {
  const { data, isLoading, isError, error, refetch, isRefetching } = useMetadataProviders();
  const set = useSetMetadataProvider();
  const [turningOn, setTurningOn] = useState(null);
  const providers = data?.data || [];

  return (
    <Section
      title="Provedores de metadados"
      hint="Serviços externos que sugerem autor, sinopse, capa e outros dados de uma obra. Todos começam desligados: ligar um envia o título de cada obra analisada a esse serviço. As sugestões nunca mudam nada sozinhas. Extração do arquivo e detecção de idioma seguem locais, ligados ou não."
    >
      {isLoading && <Loading />}
      {isError && <LoadError error={error} onRetry={refetch} retrying={isRefetching}>Não foi possível carregar os provedores.</LoadError>}
      {!isLoading && !isError && providers.length === 0 && <Empty>Nenhum provedor.</Empty>}
      <ul className="divide-y divide-border-hairline">
        {providers.map((provider) => {
          const about = ABOUT[provider.id] || {};
          const note = keyNote(provider);
          return (
            <li key={provider.id} className="flex flex-wrap items-start justify-between gap-3 py-4">
              <div className="min-w-0">
                <p className="text-[14px] font-medium text-ink">
                  {provider.name}
                  <span className={`ml-2 font-mono text-[10px] uppercase ${provider.enabled ? 'text-success' : 'text-ink-faint'}`}>
                    {provider.enabled ? 'ligado' : 'desligado'}
                  </span>
                </p>
                <p className="text-[12px] text-ink-soft">Recebe: {sendsText(provider)}. Endereço: {about.host || '—'}.</p>
                {about.note && <p className="text-[12px] text-ink-faint">{about.note}</p>}
                {note && (
                  <p className={`text-[12px] ${blocked(provider) ? 'text-danger' : 'text-ink-faint'}`}>{note}</p>
                )}
              </div>
              <label className="flex min-h-10 items-center gap-2 text-[13px] text-ink">
                <input
                  type="checkbox"
                  checked={provider.enabled}
                  disabled={!isOwner || set.isPending || blocked(provider)}
                  onChange={(event) => (event.target.checked ? setTurningOn(provider) : set.mutate({ id: provider.id, enabled: false }))}
                  aria-label={`${provider.name}: ${provider.enabled ? 'ligado' : 'desligado'}`}
                />
                {provider.enabled ? 'Ligado' : 'Desligado'}
              </label>
            </li>
          );
        })}
      </ul>
      {!isOwner && <PermissionNote className="mt-3">Só o dono do acervo liga ou desliga os provedores.</PermissionNote>}
      <ErrorNote>{set.isError && describeError(set.error)}</ErrorNote>

      {turningOn && (
        <ConfirmDialog
          title={`Ligar ${turningOn.name}?`}
          message={
            <p>
              A partir de agora, é enviado a <strong>{ABOUT[turningOn.id]?.host || turningOn.name}</strong>: {sendsText(turningOn)}.
              O nome do arquivo, o conteúdo do livro e as notas não são enviados. {ABOUT[turningOn.id]?.note}
              {turningOn.keyConfigured && ' A chave de API configurada no worker vai junto.'} Você pode desligar quando quiser; o que já foi enviado não volta.
            </p>
          }
          choices={[{ label: 'Ligar', value: true, tone: 'primary' }]}
          onChoose={() => { set.mutate({ id: turningOn.id, enabled: true }); setTurningOn(null); }}
          onCancel={() => setTurningOn(null)}
        />
      )}
    </Section>
  );
}
