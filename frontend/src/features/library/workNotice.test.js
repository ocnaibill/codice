import { describe, expect, it, vi } from 'vitest';
import { noticeForWorkEvent } from './workNotice';

describe('what the library says of a work', () => {
  it('says that the metadata of a work is updated, with its title, and offers to open it', () => {
    const openWork = vi.fn();
    const notice = noticeForWorkEvent({ type: 'WORK_READY', work_id: 7, title: 'Duna' }, openWork);
    expect(notice).toMatchObject({ tone: 'success', title: 'Metadados atualizados', message: 'Duna', key: 'work-7' });
    expect(notice.action.label).toBe('Ver obra');
    notice.action.onClick();
    expect(openWork).toHaveBeenCalledWith(7);
  });

  it('says so too when the work has no title yet, and offers nothing without its number', () => {
    const notice = noticeForWorkEvent({ type: 'WORK_READY' }, vi.fn());
    expect(notice.message).toBe('A obra está pronta.');
    expect(notice.action).toBeUndefined();
  });

  it('says in Portuguese that a work could not be processed, and keeps the reason as it came, short', () => {
    const notice = noticeForWorkEvent({ type: 'WORK_ERROR', work_id: 3, error: 'invalid epub: no container' }, vi.fn());
    expect(notice).toMatchObject({ tone: 'error', title: 'Não foi possível processar a obra', message: 'invalid epub: no container', key: 'work-3' });
    expect(notice.action).toBeUndefined();
    const long = noticeForWorkEvent({ type: 'WORK_ERROR', work_id: 3, error: 'x'.repeat(500) }, vi.fn());
    expect(long.message).toHaveLength(140);
    expect(long.message.endsWith('…')).toBe(true);
    expect(noticeForWorkEvent({ type: 'WORK_ERROR', work_id: 3, error: 'y'.repeat(140) }, vi.fn()).message).toBe('y'.repeat(140));
  });

  it('has no reason to show when the server gave none', () => {
    expect(noticeForWorkEvent({ type: 'WORK_ERROR', work_id: 3 }, vi.fn()).message).toBeUndefined();
  });

  it('says nothing of what is not news: a work being analysed, an unknown message, nothing', () => {
    expect(noticeForWorkEvent({ type: 'WORK_ANALYZING', work_id: 1 }, vi.fn())).toBeNull();
    expect(noticeForWorkEvent({ type: 'OTHER' }, vi.fn())).toBeNull();
    expect(noticeForWorkEvent(null, vi.fn())).toBeNull();
    expect(noticeForWorkEvent(undefined, vi.fn())).toBeNull();
  });
});

describe('what the library says of a PDF that asks for a password (#89)', () => {
  const locked = (extra = {}) => ({ type: 'WORK_READY', work_id: 9, title: 'meu livro trancado', protected: true, ...extra });

  it('says it is a PDF with a password, that it was kept, what could not be done and that it opens with the password', () => {
    const notice = noticeForWorkEvent(locked(), vi.fn());
    expect(notice.tone).toBe('warning');
    expect(notice.title).toBe('Este PDF tem senha');
    expect(notice.message).toBe('“meu livro trancado” foi guardado, mas o Códice não consegue ler o texto nem fazer a capa, então a busca não o acha. Ele abre no leitor, com a senha.');
    expect(notice.key).toBe('work-9');
  });

  it('is not the news that the metadata were updated, and does not show the technical error of the library', () => {
    const notice = noticeForWorkEvent(locked(), vi.fn());
    expect(notice.title).not.toBe('Metadados atualizados');
    expect(notice.message).not.toMatch(/ValueError|encrypted|document closed/);
  });

  it('offers to open the work, and offers nothing without its number', () => {
    const openWork = vi.fn();
    const notice = noticeForWorkEvent(locked(), openWork);
    expect(notice.action.label).toBe('Ver obra');
    notice.action.onClick();
    expect(openWork).toHaveBeenCalledWith(9);
    expect(noticeForWorkEvent(locked({ work_id: undefined }), openWork).action).toBeUndefined();
  });

  it('says it without a title too, and keeps a long title short', () => {
    expect(noticeForWorkEvent(locked({ title: '' }), vi.fn()).message).toMatch(/^O arquivo foi guardado, mas/);
    const long = noticeForWorkEvent(locked({ title: 'x'.repeat(500) }), vi.fn());
    expect(long.message.length).toBeLessThan(400);
    expect(long.message).toContain('…');
  });

  it('does not take a work that says it is not protected for one that is', () => {
    expect(noticeForWorkEvent({ type: 'WORK_READY', work_id: 1, title: 'Duna', protected: false }, vi.fn()).title).toBe('Metadados atualizados');
  });
});
