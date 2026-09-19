import iconAllWorks from '../../assets/icons/nav-all-works.svg';
import iconFiction from '../../assets/icons/nav-fiction.svg';
import iconComics from '../../assets/icons/nav-comics.svg';
import iconAudiobooks from '../../assets/icons/nav-audiobooks.svg';
import iconArticles from '../../assets/icons/nav-articles.svg';
import iconReading from '../../assets/icons/nav-reading.svg';
import iconFavorites from '../../assets/icons/nav-favorites.svg';
import iconLists from '../../assets/icons/nav-lists.svg';
import iconHistory from '../../assets/icons/nav-history.svg';

const LIBRARIES = [
  { key: 'all', label: 'Todas as Obras', icon: iconAllWorks, format: null, active: true },
  { key: 'fiction', label: 'Ficção & Literatura', icon: iconFiction, format: 'EPUB' },
  { key: 'comics', label: 'Quadrinhos & Mangás', icon: iconComics, format: 'CBZ' },
  { key: 'audiobooks', label: 'Áudiolivros', icon: iconAudiobooks, format: 'M4B' },
  { key: 'articles', label: 'Textos & Artigos', icon: iconArticles, format: 'MD' },
];

const COLLECTION = [
  { key: 'reading', label: 'Em Leitura', icon: iconReading },
  { key: 'favorites', label: 'Favoritos', icon: iconFavorites },
  { key: 'lists', label: 'Listas & Séries', icon: iconLists },
  { key: 'history', label: 'Histórico', icon: iconHistory },
];

export function Sidebar({ onGoHome }) {
  return (
    <aside className="hidden lg:sticky lg:top-0 lg:flex lg:h-screen lg:w-[269px] lg:shrink-0 flex-col bg-surface shadow-[0px_1px_8px_0px_rgba(0,0,0,0.04)] overflow-y-auto">
      <div className="flex h-16 items-center px-4">
        <button onClick={onGoHome} className="flex flex-col items-start text-left">
          <span className="font-display font-semibold text-[24px] tracking-[-0.6px] leading-6 text-brand">
            Códice
          </span>
          <span className="font-body text-[11px] tracking-[0.55px] uppercase text-ink-soft leading-4">
            acervo e arquivamento
          </span>
        </button>
      </div>

      <nav className="flex flex-col gap-4 p-2">
        <div className="flex flex-col gap-1">
          <div className="flex items-center justify-between px-3 py-1">
            <span className="font-body font-bold text-[11px] tracking-[0.55px] uppercase text-ink-faint">
              Bibliotecas
            </span>
            <span className="font-body text-[11px] tracking-[0.44px] text-ink-faint">
              [{LIBRARIES.length}]
            </span>
          </div>
          {LIBRARIES.map((item) => (
            <button
              key={item.key}
              onClick={item.active ? onGoHome : undefined}
              className={`flex w-full items-center justify-between rounded px-3 py-2 text-left transition-colors ${
                item.active
                  ? 'bg-brand-light text-white shadow-[0px_1px_1.5px_rgba(0,0,0,0.05)]'
                  : 'text-ink-soft hover:bg-surface-alt'
              }`}
            >
              <span className="flex items-center gap-2">
                <img src={item.icon} alt="" className="size-4" />
                <span className={`font-body text-[16px] leading-6 ${item.active ? 'font-bold text-white' : 'text-[13px] leading-5 tracking-[0.065px]'}`}>
                  {item.label}
                </span>
              </span>
              {item.format && (
                <span
                  className={`font-body text-[11px] tracking-[0.44px] uppercase ${
                    item.active ? 'text-white/80' : 'text-ink-faint'
                  }`}
                >
                  {item.format}
                </span>
              )}
            </button>
          ))}
        </div>

        <div className="flex flex-col gap-1">
          <div className="px-3 py-1">
            <span className="font-body font-bold text-[11px] tracking-[0.55px] uppercase text-ink-faint">
              Sua Coleção
            </span>
          </div>
          {COLLECTION.map((item) => (
            <button
              key={item.key}
              className="flex w-full items-center gap-2 rounded px-3 py-2 text-left text-ink-soft transition-colors hover:bg-surface-alt"
            >
              <img src={item.icon} alt="" className="size-4" />
              <span className="font-body text-[13px] leading-5 tracking-[0.065px]">{item.label}</span>
            </button>
          ))}
        </div>
      </nav>
    </aside>
  );
}
