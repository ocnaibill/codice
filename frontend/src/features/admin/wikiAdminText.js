// What the tab of the wiki (DEC-185) says about a generation: its stages, how far it is, and how long it may still take.

/** The stages of a generation, in order. */
export const STAGES = [
  ['text', 'Preparar o texto'],
  ['map', 'Mapear capítulos'],
  ['identity', 'Identidades'],
  ['pages', 'Escrever páginas'],
  ['import', 'Importar'],
];

export const RUN_STATE = {
  queued: 'Na fila',
  running: 'Gerando',
  paused: 'Pausada',
  done: 'Concluída',
  failed: 'Falhou',
  cancelled: 'Cancelada',
};

export const WIKI_STATE = {
  ready: 'Pronta',
  partial: 'Parcial',
  generating: 'Gerando',
  failed: 'Falhou',
};

export const KIND_NAMES = {
  deepseek: 'DeepSeek',
  openai: 'Compatível com OpenAI',
  ollama: 'Ollama',
};

/**
 * Each stage of a run as it stands: done (before the current one, or the whole run finished), current, or still to come, with its
 * count when the worker gave one ("23 de 47").
 */
export function stagesOf(run) {
  const current = STAGES.findIndex(([key]) => key === run.stage);
  return STAGES.map(([key, label], i) => {
    const p = run.progress?.[key];
    const count = p && Number.isFinite(p.total) && p.total > 0 ? `${Math.min(p.done ?? 0, p.total)} de ${p.total}` : '';
    let state = 'todo';
    if (run.state === 'done' || (current >= 0 && i < current)) state = 'done';
    else if (i === current) state = 'current';
    return { key, label, state, count };
  });
}

/** The share of the run done (0 to 1), counting each stage as a fifth and the current one by its count; null before it starts. */
export function shareDone(run) {
  if (run.state === 'done') return 1;
  const current = STAGES.findIndex(([key]) => key === run.stage);
  if (current < 0) return null;
  const p = run.progress?.[run.stage];
  const inStage = p && p.total > 0 ? Math.min(p.done ?? 0, p.total) / p.total : 0;
  return (current + inStage) / STAGES.length;
}

/** "3 min", "1 h 20 min": a length of time, rounded to the minute. */
export function duration(ms) {
  const minutes = Math.max(1, Math.round(ms / 60000));
  if (minutes < 60) return `${minutes} min`;
  const h = Math.floor(minutes / 60);
  const m = minutes % 60;
  return m ? `${h} h ${m} min` : `${h} h`;
}

/**
 * How long a running generation has been at it and, once the mapping (the longest stage, one request per chapter) has counted some
 * chapters, a rough time left by the pace of what was done. Nothing is promised before there is a pace to go by.
 */
export function timing(run, now = Date.now()) {
  if (!run.startedAt) return null;
  const elapsed = now - new Date(run.startedAt).getTime();
  const out = { elapsed: duration(elapsed) };
  if (run.state === 'running') {
    const share = shareDone(run);
    if (share && share > 0.05 && share < 1) out.left = duration((elapsed * (1 - share)) / share);
  }
  return out;
}

/** Whether a running generation has not said anything for a while: the worker may have stopped. */
export function silent(run, now = Date.now(), limitMs = 5 * 60 * 1000) {
  if (run.state !== 'running' || !run.heartbeatAt) return false;
  return now - new Date(run.heartbeatAt).getTime() > limitMs;
}
