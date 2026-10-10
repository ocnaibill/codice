import { act } from 'react';
import { describe, it, expect, afterEach, beforeEach, vi } from 'vitest';
import { mount } from '../../admin/testUtils';
import { GreetingStats } from './GreetingStats';

let view;

const stats = {
  worksTotal: 1420, catalogedPercent: 100, inProgressCount: 4, inProgressBreakdown: { livros: 2, mangas: 1, audio: 1 },
  completedThisMonth: 12, completedBreakdown: { livros: 8, mangas: 3, audio: 1 }, totalReadingSeconds: 78 * 3600,
};

describe('GreetingStats', () => {
  beforeEach(() => {
    vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval', 'Date'], shouldAdvanceTime: false });
    vi.setSystemTime(new Date('2026-10-10T10:43:00-03:00'));
  });
  afterEach(() => {
    view.unmount();
    vi.useRealTimers();
  });

  it('greets the signed-in account by name, and asks what is wanted', async () => {
    view = await mount(<GreetingStats userName="ana" stats={null} isLoading />);
    expect(view.text()).toContain('ana\u00a0:)'); // a no-break space: the smiley is never alone on a line
    expect(view.text()).toContain('O que queremos hoje?');
    expect(view.text()).not.toContain('Bianco');
  });

  it('greets without a name while the account is still loading', async () => {
    view = await mount(<GreetingStats userName={undefined} stats={null} isLoading />);
    expect(view.text()).not.toContain('undefined');
    expect(view.text()).not.toContain(':)');
  });

  it('greets by the time of the day', async () => {
    for (const [hour, word] of [[6, 'Bom dia'], [11, 'Bom dia'], [12, 'Boa tarde'], [17, 'Boa tarde'], [18, 'Boa noite'], [23, 'Boa noite']]) {
      vi.setSystemTime(new Date(2026, 9, 10, hour, 0, 0));
      view = await mount(<GreetingStats userName="ana" stats={null} isLoading />);
      expect(document.body.querySelector('#library-heading').textContent, String(hour)).toContain(word);
      view.unmount();
    }
    view = await mount(<GreetingStats userName="ana" stats={null} isLoading />);
  });

  it('says the date and the time, and the time goes on', async () => {
    vi.setSystemTime(new Date(2026, 9, 10, 10, 43, 0));
    view = await mount(<GreetingStats userName="ana" stats={null} isLoading />);
    expect(view.text()).toContain('10 de outubro de 2026.');
    const time = document.body.querySelector('time');
    expect(time.textContent).toMatch(/^10:43/);
    expect(time.getAttribute('datetime')).toBe(new Date(2026, 9, 10, 10, 43, 0).toISOString());
    await act(async () => { vi.setSystemTime(new Date(2026, 9, 10, 10, 44, 10)); vi.advanceTimersByTime(30_000); });
    expect(document.body.querySelector('time').textContent).toMatch(/^10:44/);
  });

  it('stops asking for the time when it is gone', async () => {
    view = await mount(<GreetingStats userName="ana" stats={null} isLoading />);
    const before = vi.getTimerCount();
    view.unmount();
    expect(vi.getTimerCount()).toBeLessThan(before);
    view = await mount(<div />);
  });

  it('titles the home with the greeting, the one heading of the page', async () => {
    view = await mount(<GreetingStats userName="ana" stats={null} isLoading />);
    expect(document.body.querySelectorAll('h1')).toHaveLength(1);
    expect(document.body.querySelector('#library-heading').textContent).toMatch(/^(Bom dia|Boa tarde|Boa noite), ana\u00a0:\)$/);
  });

  it('shows three cards beside the greeting: kept, being read, and the activity of the month', async () => {
    view = await mount(<GreetingStats userName="ana" stats={stats} />);
    const cards = [...document.body.querySelectorAll('.home-metrics .library-stat')];
    expect(cards).toHaveLength(3);
    expect(cards[0].textContent).toContain('Obras preservadas');
    expect(cards[0].textContent).toContain('1.420');
    expect(cards[0].textContent).toContain('100% catalogadas');
    expect(cards[1].textContent).toContain('Em andamento');
    expect(cards[1].textContent).toContain('4');
    expect(cards[1].textContent).toContain('2 livros • 1 mangá • 1 áudio');
    expect(cards[2].textContent).toContain('Sua atividade');
    expect(cards[2].textContent).toContain('12');
    expect(cards[2].textContent).toContain('obras completas neste mês');
    expect(cards[2].textContent).toContain('78h');
    expect(cards[2].textContent).toContain('de leitura, no total');
    expect(cards[2].textContent).toContain('8 livros • 3 mangás • 1 áudio');
  });

  it('says what it does when there is nothing yet to tell', async () => {
    view = await mount(<GreetingStats userName="ana" stats={{ worksTotal: 0, inProgressCount: 0, completedThisMonth: 0, totalReadingSeconds: 0 }} />);
    const text = document.body.querySelector('.home-metrics').textContent;
    expect(text).toContain('Uma nova leitura espera');
    expect(text).toContain('Cada página conta');
    expect(text).toContain('0% catalogadas');
    expect(text).toContain('0min');
  });

  it('shows a skeleton while the numbers come, and says it and offers to try again when they could not', async () => {
    view = await mount(<GreetingStats userName="ana" stats={null} isLoading />);
    expect(document.body.querySelector('.home-metrics').getAttribute('aria-label')).toBe('Carregando estatísticas');
    expect(document.body.querySelector('.home-metrics .library-stat')).toBeNull();
    expect(document.body.querySelectorAll('.home-metrics > *')).toHaveLength(3); // as many places as there will be cards
    view.unmount();
    const retry = vi.fn();
    view = await mount(<GreetingStats userName="ana" stats={null} error onRetry={retry} />);
    expect(document.body.querySelector('[role="alert"]').textContent).toContain('Não foi possível carregar as estatísticas.');
    await view.click(view.button('Tentar novamente'));
    expect(retry).toHaveBeenCalled();
  });
});
