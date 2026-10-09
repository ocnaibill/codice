import { authenticatedUrl } from '../../../lib/api';
import { useCategories } from '../../categories/api/useCategories';
import { useGlobalStore } from '../../../store/useGlobalStore';

const byWorksThenName = (a, b) => b.works - a.works || a.name.localeCompare(b.name, 'pt');

/** The covers of a few works of a category, one over the other, as a stack on the card; a card of a category whose works have none is plain. */
function Stack({ covers }) {
  if (covers.length === 0) return <div aria-hidden="true" className="h-[88px] w-[60px] shrink-0 rounded-sm bg-surface-alt" />;
  return (
    <div aria-hidden="true" className="relative h-[88px] w-[84px] shrink-0">
      {covers.map((cover, index) => (
        <img
          key={cover}
          src={authenticatedUrl(cover)}
          alt=""
          loading="lazy"
          className="absolute top-0 h-[88px] w-[60px] rounded-sm border-[0.5px] border-border-hairline object-cover shadow-sm"
          style={{ left: `${index * 12}px`, zIndex: covers.length - index }}
        />
      ))}
    </div>
  );
}

/**
 * "Explorar por categoria": a row under the shelves of the library with the categories at the top of the tree that have works, the ones
 * with most works first, each a card that opens its page (DEC-140). Until someone makes categories and puts works in them it is not there.
 */
export function CategoryShelf() {
  const { data } = useCategories({ covers: true });
  const openCategory = useGlobalStore((state) => state.openCategory);
  const cards = (data ?? []).filter((category) => category.parentId == null && category.works > 0).sort(byWorksThenName);
  if (cards.length === 0) return null;

  return (
    <section aria-label="Explorar por categoria" className="mt-10">
      <div className="library-section-heading">
        <h2>Explorar por categoria</h2>
      </div>
      <ul className="flex gap-3 overflow-x-auto pb-2">
        {cards.map((category) => (
          <li key={category.id} className="shrink-0">
            <button
              onClick={() => openCategory(category.id)}
              className="flex min-h-[112px] w-[240px] items-center gap-3 rounded-[10px] border-[0.5px] border-border-hairline bg-white p-3 text-left hover:bg-surface/60"
            >
              <Stack covers={category.covers ?? []} />
              <span className="min-w-0 flex-1">
                <span className="block truncate font-display text-lg text-ink">{category.name}</span>
                <span className="block font-body text-[11px] tracking-[0.44px] text-ink-soft">
                  {category.works === 1 ? '1 obra' : `${category.works} obras`}
                </span>
              </span>
            </button>
          </li>
        ))}
      </ul>
    </section>
  );
}
