import { LibraryIcon } from '../../../components/ui/LibraryIcon';
import { useCollectionFavoriteToggle } from '../api/useCollections';

/** The heart of a collection or a list (#208): to favorite the whole of it, as the heart of a card does for a work. */
export function CollectionFavoriteButton({ collection, className = 'library-favorite' }) {
  const toggle = useCollectionFavoriteToggle(collection.id);
  const label = collection.isFavorite ? 'Remover dos favoritos' : 'Adicionar aos favoritos';
  return (
    <button
      type="button"
      className={className}
      onClick={() => toggle.mutate(!collection.isFavorite)}
      disabled={toggle.isPending}
      aria-pressed={!!collection.isFavorite}
      aria-label={`${label}: ${collection.name}`}
      title={label}
    >
      <LibraryIcon name="heart" />
    </button>
  );
}
