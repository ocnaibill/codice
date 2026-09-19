import { Sidebar } from './Sidebar';
import { Header } from './Header';

export function AppShell({ searchQuery, onSearchChange, onGoHome, onLogout, canAdmin, onOpenAdmin, children }) {
  return (
    <div className="flex min-h-screen w-full bg-surface">
      <Sidebar onGoHome={onGoHome} />
      <div className="flex min-w-0 flex-1 flex-col">
        <Header searchQuery={searchQuery} onSearchChange={onSearchChange} onAvatarClick={onLogout} canAdmin={canAdmin} onOpenAdmin={onOpenAdmin} />
        <main className="flex-1">{children}</main>
      </div>
    </div>
  );
}
