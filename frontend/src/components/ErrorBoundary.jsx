import React from 'react';

/**
 * What the person sees when a screen breaks while it is drawn: in Portuguese, in the colors of the app, with a way out.
 * `where` says what broke ("o leitor"); `onBack`, when it is given, is a way back to where they were; `onReload` is what
 * the reload button does (the page is loaded again by default). The technical message is there, folded, for who has to
 * say what happened.
 */
export class ErrorBoundary extends React.Component {
  constructor(props) {
    super(props);
    this.state = { hasError: false, error: null };
  }

  static getDerivedStateFromError(error) {
    return { hasError: true, error };
  }

  componentDidCatch(error, errorInfo) {
    console.error('ErrorBoundary caught:', error, errorInfo);
  }

  render() {
    if (!this.state.hasError) return this.props.children;
    const { where = 'esta tela', onBack, onReload = () => window.location.reload() } = this.props;
    const message = this.state.error?.message;
    return (
      <div role="alert" className="flex h-full min-h-[50dvh] flex-col items-center justify-center gap-4 bg-surface p-8 text-center">
        <span aria-hidden="true" className="flex size-12 items-center justify-center rounded-full bg-danger-soft text-danger">
          <svg viewBox="0 0 24 24" className="size-6" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round">
            <path d="M12 8v5M12 16.5v.01" />
            <circle cx="12" cy="12" r="9" />
          </svg>
        </span>
        <div className="max-w-md">
          <h2 className="font-display text-xl font-semibold text-ink">Algo deu errado</h2>
          <p className="mt-1 text-[14px] leading-snug text-ink-soft">
            Tivemos um problema ao mostrar {where}. Recarregar costuma resolver; se continuar, avise quem cuida do servidor.
          </p>
        </div>
        <div className="flex flex-wrap items-center justify-center gap-2">
          <button onClick={onReload} className="min-h-11 rounded-lg bg-brand px-5 text-sm font-semibold text-white transition-[filter] hover:brightness-110">
            Recarregar
          </button>
          {onBack && (
            <button onClick={onBack} className="min-h-11 rounded-lg bg-surface-alt px-5 text-sm font-semibold text-ink transition-[filter] hover:brightness-95">
              Voltar
            </button>
          )}
        </div>
        {message && (
          <details className="max-w-md text-left text-[12px] text-ink-faint">
            <summary className="cursor-pointer text-center">Detalhes técnicos</summary>
            <pre className="mt-2 whitespace-pre-wrap break-words rounded-lg bg-surface-alt p-3 font-mono text-[11px] text-ink-soft">{message}</pre>
          </details>
        )}
      </div>
    );
  }
}
