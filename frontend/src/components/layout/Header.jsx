import iconSearch from '../../assets/icons/header-search.svg';
import iconSync from '../../assets/icons/header-sync.svg';
import iconBell from '../../assets/icons/header-bell.svg';
import iconAvatar from '../../assets/icons/header-avatar.svg';
import { useState } from 'react';
import { useGlobalStore } from '../../store/useGlobalStore';

export function Header({ searchQuery, onSearchChange, onLogout, onChangePassword, canAdmin = false, onOpenAdmin }) {
  const [menuOpen, setMenuOpen] = useState(false);
  const openUploadModal = useGlobalStore((state) => state.openUploadModal);
  return (
    <header className="sticky top-0 z-30 flex h-16 items-center justify-between gap-4 bg-[rgba(249,249,253,0.85)] px-4 sm:px-6 shadow-[0px_1px_8px_0px_rgba(0,0,0,0.04)] backdrop-blur-md">
      <div className="relative flex-1 max-w-[672px]">
        <img src={iconSearch} alt="" className="pointer-events-none absolute left-[14.5px] top-1/2 size-[15px] -translate-y-1/2" />
        <input
          type="text"
          value={searchQuery}
          onChange={(e) => onSearchChange?.(e.target.value)}
          placeholder="Buscar livros, mangás, autores, tags ou ISBN... [Ctrl+K]"
          className="w-full rounded-lg bg-surface py-[10px] pl-10 pr-4 font-body text-[13px] tracking-[0.065px] text-ink-soft placeholder-ink-soft/70 shadow-[inset_0px_1px_2px_0px_rgba(0,0,0,0.05)] outline-none"
        />
      </div>

      <div className="flex items-center gap-3">
        {canAdmin && (
        <button
          onClick={openUploadModal}
          className="flex items-center gap-2 rounded bg-brand px-3 py-2 text-white shadow-[0px_1px_1.5px_rgba(0,0,0,0.1)] hover:brightness-110"
          title="Adicionar um livro, mangá ou áudio"
        >
          <span className="font-body text-[13px] leading-none">+</span>
          <span className="hidden sm:inline font-body text-[11px] tracking-[0.44px]">Adicionar</span>
        </button>
        )}
        {canAdmin && (
          <button
            onClick={onOpenAdmin}
            className="rounded bg-surface-alt px-3 py-2 font-body text-[11px] tracking-[0.44px] text-ink shadow-[0px_1px_1.5px_rgba(0,0,0,0.04)] hover:brightness-95"
            title="Administração do acervo"
          >
            Administração
          </button>
        )}
        <button className="hidden sm:flex items-center gap-2 rounded bg-surface-alt px-3 py-2 shadow-[0px_1px_1.5px_rgba(0,0,0,0.04)] hover:brightness-95">
          <img src={iconSync} alt="" className="size-3" />
          <span className="font-body text-[11px] tracking-[0.44px] text-ink">Sincronizar</span>
        </button>
        <button className="flex size-9 items-center justify-center rounded bg-surface-alt hover:brightness-95" title="Notificações">
          <img src={iconBell} alt="" className="h-[17px] w-[13px]" />
        </button>
        <span className="hidden sm:block h-5 w-px bg-[rgba(219,193,182,0.6)]" />
        <div className="relative">
          <button
            onClick={() => setMenuOpen((open) => !open)}
            aria-haspopup="menu"
            aria-expanded={menuOpen}
            title="Minha conta"
            className="flex size-8 items-center justify-center rounded-full bg-brand shadow-[0px_1px_1.5px_rgba(0,0,0,0.1)] hover:brightness-110"
          >
            <img src={iconAvatar} alt="Minha conta" className="size-3" />
          </button>
          {menuOpen && (
            <div role="menu" className="absolute right-0 top-10 z-40 w-44 overflow-hidden rounded-lg bg-white py-1 shadow-xl">
              {onChangePassword && (
                <button role="menuitem" onClick={() => { setMenuOpen(false); onChangePassword(); }}
                  className="block w-full px-4 py-2 text-left font-body text-[13px] text-ink hover:bg-surface">
                  Alterar senha
                </button>
              )}
              <button role="menuitem" onClick={() => { setMenuOpen(false); onLogout?.(); }}
                className="block w-full px-4 py-2 text-left font-body text-[13px] text-ink hover:bg-surface">
                Sair
              </button>
            </div>
          )}
        </div>
      </div>
    </header>
  );
}
