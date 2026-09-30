import { NAME_ORDERS, usePreferences, useSetLibraryNameOrder } from '../../auth/api/usePreferences';
import { describeError } from '../api/admin';
import { ErrorNote, Section } from './ui';

/**
 * The library's default for how names of authors are shown (#64). Each account may choose its own in
 * Preferências; this is what applies to whoever has not. Only presentation: nothing stored changes.
 */
export function LibraryNameOrder() {
  const { data: prefs, isLoading } = usePreferences();
  const save = useSetLibraryNameOrder();
  return (
    <Section
      title="Nome dos autores"
      hint="O padrão da biblioteca para como o nome de um autor aparece e para a ordem por autor. Cada conta pode escolher o seu em Preferências. Só muda a apresentação: nada do que está guardado é alterado."
    >
      <fieldset disabled={isLoading || save.isPending} className="flex flex-col gap-2">
        {Object.entries(NAME_ORDERS).map(([value, { label, example }]) => (
          <label key={value} className="flex cursor-pointer items-start gap-3 text-[14px] text-ink">
            <input type="radio" name="library-name-order" className="mt-1" checked={prefs?.library === value} onChange={() => save.mutate(value)} />
            <span>
              {label} <span className="text-ink-faint">({example})</span>
            </span>
          </label>
        ))}
      </fieldset>
      <ErrorNote>{save.isError && describeError(save.error)}</ErrorNote>
    </Section>
  );
}
