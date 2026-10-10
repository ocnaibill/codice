import { useState } from 'react';
import { describeError, useMetadataProviders, useSetMetadataProvider, useTestMetadataProvider } from '../api/admin';
import { ConfirmDialog } from './ConfirmDialog';
import { Btn, Empty, ErrorNote, Loading, Section } from './ui';
import { LoadError } from '../../../components/ui/LoadError';
import { describeHealth, describeTest, TONE_CLASS } from '../providerHealth';
import { PermissionNote } from '../../../components/ui/PermissionNote';

// What each provider is, where it lives, and what asking it hands over: the owner decides knowing (DEC-045).
const ABOUT = {
  google_books: { host: 'googleapis.com (Google)', keyEnv: 'GOOGLE_BOOKS_API_KEY', note: 'Só funciona com uma chave de API sua: sem ela o Google divide uma cota diária com todo mundo, e ela quase sempre já acabou. Responde com até 20 livros por título, e o Códice escolhe o certo pelo autor do arquivo, que não é enviado.' },
  openlibrary: { host: 'openlibrary.org (Internet Archive)', note: 'Também é a fonte das chaves de autoridade dos autores.' },
  comicvine: { host: 'comicvine.gamespot.com', keyEnv: 'COMICVINE_API_KEY', note: 'Para quadrinhos: acha a série e depois a edição com o número do arquivo (editora, data, capa, sinopse e quem escreveu e desenhou).' },
  anilist: { host: 'graphql.anilist.co (AniList)', note: 'Para mangá: autores, gêneros, ano, sinopse e capa da série. Uso gratuito não comercial; não guardamos mais do que você aceita.' },
  mangadex: { host: 'api.mangadex.org (MangaDex)', note: 'Para mangá: autores, gêneros, público (seinen, shounen…), sinopse e capa da série. Política de uso: não comercial.' },
  wikidata: { host: 'www.wikidata.org e commons.wikimedia.org (Wikimedia)', note: 'Diz que obra é: traduz um título que os outros não conhecem (“A Nuvem” é Thunderhead), e acrescenta o identificador, a série e os gêneros em português. Também lê o perfil dos autores que têm identificador Wikidata (descrição, anos e a foto, que é baixada para o servidor com o crédito e a licença). Dados em domínio público (CC0); as fotos têm a licença de cada uma.' },
  wikipedia: { host: '*.wikipedia.org (Wikimedia)', note: 'Completa a sinopse com o resumo da página da obra, quando nenhum outro provedor trouxe uma, e a biografia dos autores; a fonte e a licença (CC BY-SA 4.0) ficam escritas junto do texto. Só funciona com o Wikidata ligado, que diz qual é a página.' },
};
// The fixed, public question the test asks each provider (nothing of the library is in it).
const QUESTION = {
  google_books: '“Dune”', openlibrary: '“Dune”', wikidata: '“Dune”', comicvine: '“Absolute Batman”', anilist: '“Berserk”', mangadex: '“Berserk”',
  wikipedia: 'a página “Dune (novel)”',
};
const SENDS = {
  title: 'o título da obra',
  author_id: 'o identificador Wikidata de cada autor que você aceita (para ler o perfil dele: descrição, anos, biografia e foto)',
  isbn: 'o ISBN do arquivo, quando ele tem (para achar o livro sem dúvida)',
  page_title: 'só o título da página da obra na Wikipédia, que o Wikidata informou (nunca o título do arquivo)',
  author_key: 'a chave de cada autor que você aceita (para obter os identificadores dele: Wikidata, VIAF, ISNI)',
};

/** What is known of the API key of a provider: the worker says whether it has one, never the key. The key is set
 *  in the environment of the worker, not here. */
function keyNote(provider) {
  const env = ABOUT[provider.id]?.keyEnv;
  if (!provider.key || !env) return null;
  if (provider.keyConfigured === true) return 'Chave de API configurada.';
  if (provider.keyConfigured === false) return `Falta a chave de API: sem ela o ${provider.name} não funciona. Defina ${env} no ambiente do worker e reinicie-o.`;
  return 'O worker ainda não informou se a chave de API existe.';
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
  const test = useTestMetadataProvider();
  const [turningOn, setTurningOn] = useState(null);
  const [testing, setTesting] = useState(null); // the provider that is off and the owner is about to ask a test question
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
          const health = describeHealth(provider);
          const lastTest = describeTest(provider);
          const busy = provider.testing || (test.isPending && test.variables === provider.id);
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
                {health && (
                  <p className={`text-[12px] ${TONE_CLASS[health.tone]}`} data-health={provider.health?.state ?? 'none'}>
                    {health.text}
                  </p>
                )}
                {busy && <p role="status" className="text-[12px] text-ink-soft">Testando…</p>}
                {!busy && lastTest && (
                  <p className={`text-[12px] ${TONE_CLASS[lastTest.tone]}`} data-test={provider.test.state}>
                    {lastTest.text}
                  </p>
                )}
              </div>
              <div className="flex items-center gap-3">
                {isOwner && (
                  <Btn
                    disabled={busy || test.isPending}
                    aria-label={`Testar ${provider.name}`}
                    onClick={() => (provider.enabled ? test.mutate(provider.id) : setTesting(provider))}
                  >
                    {busy ? 'Testando…' : 'Testar'}
                  </Btn>
                )}
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
              </div>
            </li>
          );
        })}
      </ul>
      {!isOwner && <PermissionNote className="mt-3">Só o dono do acervo liga ou desliga os provedores.</PermissionNote>}
      <ErrorNote>{set.isError && describeError(set.error)}</ErrorNote>
      <ErrorNote>{test.isError && describeError(test.error)}</ErrorNote>

      {testing && (
        <ConfirmDialog
          title={`Testar ${testing.name}?`}
          message={
            <p>
              O teste faz uma pergunta a <strong>{ABOUT[testing.id]?.host || testing.name}</strong> com um título fixo e público, {QUESTION[testing.id]}, para ver se ele
              responde. Nada da sua biblioteca é enviado. O provedor continua desligado.{testing.keyConfigured && ' A chave de API configurada no worker vai junto.'}
            </p>
          }
          choices={[{ label: 'Testar', value: true, tone: 'primary' }]}
          onChoose={() => { test.mutate(testing.id); setTesting(null); }}
          onCancel={() => setTesting(null)}
        />
      )}
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
