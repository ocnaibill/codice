import React from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api, authenticatedUrl } from '../../../lib/api';
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

/**
 * What Wikidata and Wikipedia say about the person: a short description, when they were born and died, the first paragraphs of the biography
 * and a photo, each with where it came from and under which license (the biography is CC BY-SA 4.0, the photo has its own). It is there when a
 * person confirmed which Wikidata entity the author is; staff can hide it all, or only the photo, when it is the wrong person.
 */
export function PersonProfile({ person }) {
  const staff = isStaff(useMe().data);
  const set = useSetProfile(person.id);
  const profile = person.profile;
  if (!profile) return null;

  const born = formatProfileDate(profile.born);
  const died = formatProfileDate(profile.died);
  // "Nasceu em 8 de outubro de 1920, em Tacoma": the place goes with the date, or alone when the date is not known.
  const place = profile.bornPlace ? `em ${profile.bornPlace}` : '';
  const birth = born ? `Nasceu em ${born}${place ? `, ${place}` : ''}` : place ? `Nasceu ${place}` : '';
  const years = [birth, died && `Morreu em ${died}`].filter(Boolean).join(' · ');
  const image = profile.image;
  const hiddenNote = profile.hidden ? 'O perfil está oculto: só quem administra o vê.' : null;

  return (
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
        {set.isError && <p role="alert" className="text-[12px] text-danger">Não foi possível salvar a escolha.</p>}
      </div>
    </section>
  );
}
