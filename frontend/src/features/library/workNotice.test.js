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
