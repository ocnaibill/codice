import React from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, authenticatedUrl } from '../../../lib/api';
import { serverMessage } from '../../../lib/serverMessage';
import { isStaff, useMe } from '../../auth/api/useMe';
import { formatProfileDate } from '../profileDates';

/** Hides, or shows again, the profile of a person or only the photo (staff, DEC-146). */
function useSetProfile(personId) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (body) => (await api.put(`/admin/people/${personId}/profile`, body)).data,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['person', personId] }),
  });
}

/** Writes by hand the profile of a person (staff, DEC-167): the texts, as JSON. */
function useWriteProfile(personId) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (body) => (await api.put(`/admin/people/${personId}/profile`, body)).data,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['person', personId] }),
  });
}

/** Sends the photo of a person, with the credit and the license it is shown under. */
function useSendPhoto(personId) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ file, credit, license }) => {
      const form = new FormData();
      form.append('image', file);
      form.append('credit', credit);
      form.append('license', license);
      return (await api.post(`/admin/people/${personId}/profile/photo`, form)).data;
    },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['person', personId] }),
  });
}

/** Throws the profile away; the one on Wikidata, if the person has an identifier, is read again. */
function useResetProfile(personId) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async () => api.delete(`/admin/people/${personId}/profile`),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['person', personId] }),
  });
}

/** The last search of the person on Wikidata (staff, DEC-168): null when none was made; it is read again while it waits for the worker. */
function useProfileSearch(personId, enabled) {
  return useQuery({
    queryKey: ['person', personId, 'profile-search'],
    enabled,
    staleTime: 0,
    queryFn: async () => {
      try {
        return (await api.get(`/admin/people/${personId}/profile/search`)).data;
      } catch (error) {
        if (error?.response?.status === 404) return null;
        throw error;
      }
    },
    refetchInterval: (query) => (query.state.data?.state === 'pending' ? 1500 : false),
  });
}

function useAskSearch(personId) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (query) => (await api.post(`/admin/people/${personId}/profile/search`, { query })).data,
    onSuccess: (data) => queryClient.setQueryData(['person', personId, 'profile-search'], data),
  });
}

function useLinkProfile(personId) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (wikidataId) => (await api.post(`/admin/people/${personId}/profile/link`, { wikidataId })).data,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['person', personId] }),
  });
}

const SEARCH_STATES = {
  off: 'A Wikidata está desligada. Quem é dono do acervo a liga em Administração › Provedores.',
  failed: 'A Wikidata não respondeu. Tente de novo em instantes.',
};
const LINK_OUTCOMES = {
  queued: 'Pronto: o perfil dessa pessoa chega em instantes.',
  present: 'Esta pessoa já tem o perfil desta entrada da Wikidata.',
  kept: 'O perfil escrito à mão continua. Descarte-o para ler o da Wikidata.',
};

/**
 * Looks the person up on Wikidata by name (DEC-168): the candidates come with what tells one from another, and a person of the staff says which
 * one is the author. Nothing is linked by the search itself.
 */
function ProfileSearch({ person, onClose }) {
  const search = useProfileSearch(person.id, true);
  const ask = useAskSearch(person.id);
  const link = useLinkProfile(person.id);
  const [query, setQuery] = React.useState(person.name ?? person.displayName ?? '');
  const [message, setMessage] = React.useState('');
  const [chosen, setChosen] = React.useState('');
  const result = search.data;
  const waiting = ask.isPending || result?.state === 'pending';

  const submit = (event) => {
    event.preventDefault();
    setMessage('');
    setChosen('');
    ask.mutate(query.trim(), { onError: (error) => setMessage(serverMessage(error, 'Não foi possível pedir a busca.')) });
  };
  const choose = (candidate) => {
    setMessage('');
    link.mutate(candidate.id, {
      onSuccess: (data) => { setChosen(candidate.id); setMessage(LINK_OUTCOMES[data?.profile] ?? LINK_OUTCOMES.queued); },
      onError: (error) => setMessage(serverMessage(error, 'Não foi possível ligar o perfil.')),
    });
  };

  return (
    <section aria-label="Buscar o perfil" className="flex flex-col gap-3 rounded-xl border border-border-hairline bg-white p-4 shadow-sm">
      <form onSubmit={submit} className="flex flex-wrap items-end gap-3">
        <label className="flex min-w-[220px] flex-1 flex-col gap-1 text-sm text-ink-soft">
          Buscar na Wikidata por
          <input value={query} maxLength={200} onChange={(event) => setQuery(event.target.value)} className={FIELD} />
        </label>
        <button type="submit" disabled={waiting || !query.trim()} className={PRIMARY}>Buscar</button>
        <button type="button" onClick={onClose} className={BUTTON}>Fechar</button>
      </form>
      <p className="text-xs text-ink-faint">
        Só o texto buscado sai do servidor. A busca não liga nada: você escolhe quem é o autor, e o perfil dele é lido depois.
      </p>
      {waiting && <p role="status" className="animate-pulse text-sm text-ink-soft">Buscando na Wikidata…</p>}
      {!waiting && result && SEARCH_STATES[result.state] && <p role="status" className="text-sm text-ink">{SEARCH_STATES[result.state]}</p>}
      {!waiting && result?.state === 'done' && result.results.length === 0 && (
        <p role="status" className="text-sm text-ink">Ninguém com esse nome na Wikidata. Tente outra grafia, ou escreva o perfil à mão.</p>
      )}
      {!waiting && result?.state === 'done' && result.results.length > 0 && (
        <ul aria-label="Pessoas encontradas" className="flex flex-col gap-2">
          {result.results.map((c) => {
            const years = [c.born && `nasc. ${formatProfileDate(c.born)}`, c.died && `morte ${formatProfileDate(c.died)}`].filter(Boolean).join(' · ');
            return (
              <li key={c.id} className="flex flex-wrap items-center gap-3 rounded-lg bg-surface px-3 py-2">
                <div className="min-w-[200px] flex-1 text-sm">
                  <p className="font-semibold text-ink">{c.label}</p>
                  <p className="text-ink-soft">{c.description || 'Sem descrição'}</p>
                  <p className="font-mono text-[11px] text-ink-faint">
                    {[years, c.photo && 'com foto', c.wikipedia && 'com Wikipédia'].filter(Boolean).join(' · ')}
                    {years || c.photo || c.wikipedia ? ' · ' : ''}
                    <a href={`https://www.wikidata.org/wiki/${c.id}`} target="_blank" rel="noopener noreferrer" className="underline hover:text-brand">Ver na Wikidata ({c.id})</a>
                  </p>
                </div>
                <button type="button" disabled={link.isPending || chosen === c.id} onClick={() => choose(c)} className={PRIMARY} aria-label={`É esta pessoa: ${c.label}, ${c.id}`}>
                  É esta pessoa
                </button>
              </li>
            );
          })}
        </ul>
      )}
      {message && <p role={link.isError || ask.isError ? 'alert' : 'status'} className={`text-sm ${link.isError || ask.isError ? 'text-danger' : 'text-ink'}`}>{message}</p>}
    </section>
  );
}

const FIELD = 'min-h-11 w-full rounded-lg border border-border-hairline bg-surface px-3 text-sm text-ink';
const BUTTON = 'min-h-9 rounded-lg border border-border-hairline bg-surface px-3 text-xs text-ink hover:bg-surface-alt disabled:opacity-40';
const PRIMARY = 'min-h-10 rounded-lg bg-brand px-4 text-xs font-semibold text-white hover:bg-brand-light disabled:opacity-40';
const PHOTO_TYPES = ['image/jpeg', 'image/png', 'image/webp'];
const PHOTO_MAX = 5 * 1024 * 1024;

/**
 * The profile of a person, written by hand (DEC-167): for an author no provider knows, or that one of them has wrong. The texts go in one
 * request and the photo, if one was chosen, in another with its credit and license; what is left empty is taken away.
 */
function ProfileEditor({ person, profile, onDone }) {
  const write = useWriteProfile(person.id);
  const send = useSendPhoto(person.id);
  const [values, setValues] = React.useState({
    description: profile?.description ?? '',
    born: profile?.born ?? '',
    died: profile?.died ?? '',
    bornPlace: profile?.bornPlace ?? '',
    bio: profile?.bio ?? '',
  });
  const [credit, setCredit] = React.useState(profile?.image?.credit ?? '');
  const [license, setLicense] = React.useState(profile?.image?.license ?? '');
  const [file, setFile] = React.useState(null);
  const [message, setMessage] = React.useState('');
  const busy = write.isPending || send.isPending;
  const set = (key) => (event) => setValues((v) => ({ ...v, [key]: event.target.value }));

  const chooseFile = (event) => {
    const chosen = event.target.files?.[0] ?? null;
    setMessage('');
    if (chosen && !PHOTO_TYPES.includes(chosen.type)) {
      setFile(null);
      setMessage('A foto deve ser JPEG, PNG ou WebP.');
      return;
    }
    if (chosen && chosen.size > PHOTO_MAX) {
      setFile(null);
      setMessage('A foto tem até 5 MB.');
      return;
    }
    setFile(chosen);
  };

  const submit = async (event) => {
    event.preventDefault();
    setMessage('');
    try {
      // With a new photo the credit and the license go with it; without one they are texts of the profile like the others.
      await write.mutateAsync(file ? values : { ...values, imageCredit: credit, imageLicense: license });
      if (file) await send.mutateAsync({ file, credit, license });
      onDone();
    } catch (error) {
      setMessage(serverMessage(error, 'Não foi possível salvar o perfil.'));
    }
  };

  return (
    <form onSubmit={submit} aria-label="Escrever o perfil" className="flex flex-col gap-3 rounded-xl border border-border-hairline bg-white p-4 shadow-sm">
      <label className="flex flex-col gap-1 text-sm text-ink-soft">
        Descrição curta
        <input value={values.description} maxLength={500} onChange={set('description')} placeholder="Ex.: escritor brasileiro" className={FIELD} />
      </label>
      <div className="grid gap-3 sm:grid-cols-3">
        <label className="flex flex-col gap-1 text-sm text-ink-soft">
          Nascimento
          <input value={values.born} maxLength={16} onChange={set('born')} placeholder="1962, 1962-11 ou 1962-11-12" className={FIELD} />
        </label>
        <label className="flex flex-col gap-1 text-sm text-ink-soft">
          Morte
          <input value={values.died} maxLength={16} onChange={set('died')} placeholder="deixe vazio se vive" className={FIELD} />
        </label>
        <label className="flex flex-col gap-1 text-sm text-ink-soft">
          Local de nascimento
          <input value={values.bornPlace} maxLength={255} onChange={set('bornPlace')} className={FIELD} />
        </label>
      </div>
      <label className="flex flex-col gap-1 text-sm text-ink-soft">
        Biografia
        <textarea value={values.bio} maxLength={6000} rows={6} onChange={set('bio')} className="rounded-lg border border-border-hairline bg-surface px-3 py-2 text-sm text-ink" />
        <span className="font-mono text-[11px] text-ink-faint">{values.bio.length} de 6000</span>
      </label>
      <fieldset className="flex flex-col gap-2 rounded-lg border border-border-hairline p-3">
        <legend className="px-1 text-sm text-ink-soft">Foto</legend>
        <label className="flex flex-col gap-1 text-sm text-ink-soft">
          Enviar uma foto (JPEG, PNG ou WebP, até 5 MB)
          <input type="file" accept="image/jpeg,image/png,image/webp" onChange={chooseFile} className="text-sm text-ink" />
        </label>
        <div className="grid gap-3 sm:grid-cols-2">
          <label className="flex flex-col gap-1 text-sm text-ink-soft">
            Crédito da foto
            <input value={credit} maxLength={300} onChange={(e) => setCredit(e.target.value)} placeholder="Quem fez a foto" className={FIELD} />
          </label>
          <label className="flex flex-col gap-1 text-sm text-ink-soft">
            Licença da foto
            <input value={license} maxLength={300} onChange={(e) => setLicense(e.target.value)} placeholder="Ex.: CC BY 4.0, uso autorizado" className={FIELD} />
          </label>
        </div>
      </fieldset>
      <p className="text-xs text-ink-faint">
        O que fica escrito aqui é seu: o Códice não lê esta pessoa de novo nos provedores. Deixe um campo vazio para tirá-lo.
      </p>
      {message && <p role="alert" className="text-sm text-danger">{message}</p>}
      <div className="flex flex-wrap gap-2">
        <button type="submit" disabled={busy} className={PRIMARY}>Salvar</button>
        <button type="button" onClick={onDone} className={BUTTON}>Cancelar</button>
      </div>
    </form>
  );
}

/**
 * What Wikidata and Wikipedia say about the person: a short description, when they were born and died, the first paragraphs of the biography
 * and a photo, each with where it came from and under which license (the biography is CC BY-SA 4.0, the photo has its own). It is there when a
 * person confirmed which Wikidata entity the author is; staff can hide it all, or only the photo, when it is the wrong person.
 */
export function PersonProfile({ person }) {
  const staff = isStaff(useMe().data);
  const set = useSetProfile(person.id);
  const reset = useResetProfile(person.id);
  const profile = person.profile;
  const [editing, setEditing] = React.useState(false);
  const [searching, setSearching] = React.useState(false);
  const [discarding, setDiscarding] = React.useState(false);
  const [resetMessage, setResetMessage] = React.useState('');
  if (!profile && !staff) return null;
  if (editing) return <ProfileEditor person={person} profile={profile} onDone={() => setEditing(false)} />;
  if (!profile) {
    // Nothing was read for the person: staff are told, so the emptiness does not look like a defect, and can look it up or write it.
    return (
      <div className="flex flex-col gap-3">
        <section aria-label="Perfil" className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-dashed border-border-hairline bg-white/60 p-4">
          <p className="min-w-[220px] flex-1 text-sm text-ink-soft">
            Sem perfil: nenhum provedor trouxe a foto e a biografia desta pessoa. Você pode buscá-lo na Wikidata ou escrevê-lo.
          </p>
          <div className="flex flex-wrap gap-2">
            <button type="button" onClick={() => setSearching(!searching)} aria-pressed={searching} className={BUTTON}>Buscar o perfil</button>
            <button type="button" onClick={() => setEditing(true)} className={BUTTON}>Escrever o perfil</button>
          </div>
        </section>
        {searching && <ProfileSearch person={person} onClose={() => setSearching(false)} />}
      </div>
    );
  }

  const born = formatProfileDate(profile.born);
  const died = formatProfileDate(profile.died);
  // "Nasceu em 8 de outubro de 1920, em Tacoma": the place goes with the date, or alone when the date is not known.
  const place = profile.bornPlace ? `em ${profile.bornPlace}` : '';
  const birth = born ? `Nasceu em ${born}${place ? `, ${place}` : ''}` : place ? `Nasceu ${place}` : '';
  const years = [birth, died && `Morreu em ${died}`].filter(Boolean).join(' · ');
  const image = profile.image;
  const hiddenNote = profile.hidden ? 'O perfil está oculto: só quem administra o vê.' : null;

  return (
    <div className="flex flex-col gap-3">
    <section aria-label="Perfil" className={`flex flex-col gap-4 rounded-xl border border-border-hairline bg-white p-4 sm:flex-row ${profile.hidden ? 'opacity-70' : ''}`}>
      {image && (
        <figure className="w-32 shrink-0 sm:w-40">
          <img
            src={authenticatedUrl(image.url)}
            alt={`Foto de ${person.displayName}`}
            className={`w-full rounded-lg object-cover ${profile.imageHidden ? 'opacity-50' : ''}`}
          />
          <figcaption className="mt-1 text-[11px] leading-snug text-ink-faint">
            Foto{image.credit ? `: ${image.credit}` : ''}
            {image.license ? ` · ${image.license}` : ''}
            {image.pageUrl && (
              <>
                {' · '}
                <a href={image.pageUrl} target="_blank" rel="noopener noreferrer" className="underline hover:text-brand">Wikimedia Commons</a>
              </>
            )}
          </figcaption>
        </figure>
      )}
      <div className="flex min-w-0 flex-col gap-2">
        {profile.description && <p className="text-sm font-medium text-ink">{profile.description}</p>}
        {years && <p className="text-[13px] text-ink-soft">{years}</p>}
        {profile.bio && (
          <>
            <p className="whitespace-pre-line text-sm leading-relaxed text-ink">{profile.bio}</p>
            {profile.bioSource && (
              <p className="text-[11px] text-ink-faint">
                Fonte:{' '}
                <a href={profile.bioSource.url} target="_blank" rel="noopener noreferrer" className="underline hover:text-brand">
                  Wikipédia ({profile.bioSource.language})
                </a>
                , {profile.bioSource.license}
              </p>
            )}
          </>
        )}
        {hiddenNote && <p role="status" className="text-[12px] text-warning">{hiddenNote}</p>}
        {staff && (
          <div className="mt-1 flex flex-wrap gap-2">
            <button
              type="button"
              disabled={set.isPending}
              onClick={() => set.mutate({ hidden: !profile.hidden })}
              className="min-h-9 rounded-lg border border-border-hairline bg-surface px-3 text-xs text-ink hover:bg-surface-alt disabled:opacity-40"
            >
              {profile.hidden ? 'Mostrar o perfil' : 'Ocultar o perfil'}
            </button>
            <button type="button" onClick={() => setSearching(!searching)} aria-pressed={searching} className={BUTTON}>Buscar o perfil</button>
            <button type="button" onClick={() => setEditing(true)} className={BUTTON}>Editar o perfil</button>
            <button type="button" onClick={() => setDiscarding(!discarding)} aria-pressed={discarding} className={BUTTON}>Descartar o perfil</button>
            {image && (
              <button
                type="button"
                disabled={set.isPending}
                onClick={() => set.mutate({ imageHidden: !profile.imageHidden })}
                className="min-h-9 rounded-lg border border-border-hairline bg-surface px-3 text-xs text-ink hover:bg-surface-alt disabled:opacity-40"
              >
                {profile.imageHidden ? 'Mostrar a foto' : 'Ocultar a foto'}
              </button>
            )}
          </div>
        )}
        {staff && profile.manual && <p className="text-[11px] text-ink-faint">Escrito à mão: o Códice não o lê de novo nos provedores.</p>}
        {staff && discarding && (
          <div role="alertdialog" aria-label="Descartar o perfil" className="flex flex-wrap items-center gap-3 rounded-lg bg-surface px-3 py-2 text-xs text-ink-soft">
            <span className="min-w-[200px] flex-1">
              Descartar o perfil? O que foi escrito se perde{profile.wikidataId ? ' e o perfil da Wikidata é lido de novo' : ''}.
            </span>
            <button
              type="button"
              disabled={reset.isPending}
              onClick={() =>
                reset.mutate(undefined, {
                  onSuccess: () => setDiscarding(false),
                  onError: (error) => setResetMessage(serverMessage(error, 'Não foi possível descartar o perfil.')),
                })
              }
              className={PRIMARY}
            >
              Descartar
            </button>
            <button type="button" onClick={() => setDiscarding(false)} className={BUTTON}>Cancelar</button>
          </div>
        )}
        {resetMessage && <p role="alert" className="text-[12px] text-danger">{resetMessage}</p>}
        {set.isError && <p role="alert" className="text-[12px] text-danger">Não foi possível salvar a escolha.</p>}
      </div>
    </section>
    {staff && searching && <ProfileSearch person={person} onClose={() => setSearching(false)} />}
    </div>
  );
}
