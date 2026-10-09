// What the library says when the server reports on a work (#77): "metadados atualizados" and "não foi possível
// processar". The words are the person's, in Portuguese; the server's own text of an error is kept as the detail.

const MAX_DETAIL = 140;

const clip = (text) => (text.length > MAX_DETAIL ? `${text.slice(0, MAX_DETAIL - 1)}…` : text);

/**
 * The notice for a message of the real-time channel, or null when it is not one that says anything to the person
 * (a work that is only being analysed is not news). `openWork` lets the notice offer to open the work.
 */
export function noticeForWorkEvent(event, openWork) {
  // A PDF that asks for a password is kept and ready to be opened with it, but nothing of it could be read: it is said as it is, and
  // not as the news that its metadata were updated.
  if (event?.type === 'WORK_READY' && event.protected) {
    return {
      tone: 'warning',
      title: 'Este PDF tem senha',
      message: `${event.title ? `“${clip(String(event.title))}” foi guardado` : 'O arquivo foi guardado'}, mas o Códice não consegue ler o texto nem fazer a capa, então a busca não o acha. Ele abre no leitor, com a senha.`,
      key: `work-${event.work_id}`,
      action: event.work_id ? { label: 'Ver obra', onClick: () => openWork(event.work_id) } : undefined,
    };
  }
  if (event?.type === 'WORK_READY') {
    return {
      tone: 'success',
      title: 'Metadados atualizados',
      message: event.title || 'A obra está pronta.',
      key: `work-${event.work_id}`,
      action: event.work_id ? { label: 'Ver obra', onClick: () => openWork(event.work_id) } : undefined,
    };
  }
  if (event?.type === 'WORK_ERROR') {
    return {
      tone: 'error',
      title: 'Não foi possível processar a obra',
      message: event.error ? clip(String(event.error)) : undefined,
      key: `work-${event.work_id}`,
    };
  }
  return null;
}
