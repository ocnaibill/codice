import { useEffect, useRef, useState } from 'react';
import { useCategories } from '../../categories/api/useCategories';
import { useWorks } from '../../library/api/useWorks';
import { useGlobalStore } from '../../../store/useGlobalStore';
import { Shelf } from './Shelf';

// How many categories get a shelf on the home, the ones with most works first; the rest are in "Explorar por categoria".
export const MAX_ROWS = 6;
const PER_ROW = 12;
const byWorksThenName = (a, b) => b.works - a.works || a.name.localeCompare(b.name, 'pt');

/** True once the element was on the screen (or near it). Where a browser cannot tell, it is true at once. */
function useSeen() {
  const ref = useRef(null);
  const [seen, setSeen] = useState(typeof IntersectionObserver !== 'function');
  useEffect(() => {
    if (seen || !ref.current) return undefined;
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((entry) => entry.isIntersecting)) {
          setSeen(true);
          observer.disconnect();
        }
      },
      { rootMargin: '300px 0px' }
    );
    observer.observe(ref.current);
    return () => observer.disconnect();
  }, [seen]);
  return [ref, seen];
}

function CategoryRow({ category }) {
  const [ref, seen] = useSeen();
  const openCategory = useGlobalStore((state) => state.openCategory);
  const works = useWorks({ category: category.id, limit: PER_ROW, series: 'collapse', enabled: seen });
  return (
    <div ref={ref} className="min-h-[120px]">
      {seen && (
        <Shelf
          headingId={`category-row-${category.id}`}
          title={category.name}
          total={category.works}
          items={works.data?.data ?? []}
          isLoading={works.isLoading}
          seeAll={{ label: 'Ver categoria', onClick: () => openCategory(category.id) }}
        />
      )}
    </div>
  );
}

/**
 * One shelf per category, one under the other, across the whole width: the works of the categories at the top of the tree that have any,
 * the ones with most works first (DEC-140). A shelf asks for its works only when it comes near the screen, so a library with many
 * categories does not ask for them all at once. Until somebody makes categories and puts works in them there is nothing here.
 */
export function CategoryRows() {
  const { data } = useCategories();
  const rows = (data ?? []).filter((category) => category.parentId == null && category.works > 0).sort(byWorksThenName).slice(0, MAX_ROWS);
  if (rows.length === 0) return null;
  return (
    <div className="flex flex-col gap-10" aria-label="Por categoria">
      {rows.map((category) => (
        <CategoryRow key={category.id} category={category} />
      ))}
    </div>
  );
}
