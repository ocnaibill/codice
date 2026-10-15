import { describe, it, expect } from 'vitest';
import { duration, shareDone, silent, stagesOf, timing } from './wikiAdminText';

const run = (over = {}) => ({ state: 'running', stage: 'map', progress: { map: { done: 10, total: 40 } }, ...over });

describe('the stages of a generation', () => {
  it('marks the ones before as done, the current one with its count, and the rest to come', () => {
    expect(stagesOf(run()).map((s) => `${s.key}:${s.state}:${s.count}`)).toEqual(['text:done:', 'map:current:10 de 40', 'identity:todo:', 'pages:todo:', 'import:todo:']);
    expect(stagesOf(run({ state: 'done', stage: 'import' })).every((s) => s.state === 'done')).toBe(true);
    expect(stagesOf(run({ state: 'queued', stage: '', progress: {} })).every((s) => s.state === 'todo')).toBe(true);
    expect(stagesOf(run({ progress: { map: { done: 50, total: 40 } } }))[1].count).toBe('40 de 40');
  });

  it('counts each stage as a fifth, and the current one by how far it is', () => {
    expect(shareDone(run())).toBeCloseTo((1 + 0.25) / 5);
    expect(shareDone(run({ state: 'queued', stage: '' }))).toBeNull();
    expect(shareDone(run({ state: 'done' }))).toBe(1);
  });
});

describe('the time of a generation', () => {
  const now = Date.parse('2026-10-15T12:30:00Z');
  it('says how long it has run and, once there is a pace, about how long it has left', () => {
    const t = timing(run({ startedAt: '2026-10-15T12:00:00Z' }), now);
    expect(t.elapsed).toBe('30 min');
    expect(t.left).toBe('1 h 30 min'); // a quarter done (0.25 of the run) in 30 min
    expect(timing(run({ startedAt: '2026-10-15T12:00:00Z', stage: 'text', progress: {} }), now).left).toBeUndefined();
    expect(timing(run({ startedAt: null }), now)).toBeNull();
  });

  it('writes minutes and hours', () => {
    expect(duration(10_000)).toBe('1 min');
    expect(duration(59 * 60000)).toBe('59 min');
    expect(duration(120 * 60000)).toBe('2 h');
    expect(duration(125 * 60000)).toBe('2 h 5 min');
  });

  it('notices a running generation that has not said anything for five minutes', () => {
    expect(silent(run({ heartbeatAt: '2026-10-15T12:20:00Z' }), now)).toBe(true);
    expect(silent(run({ heartbeatAt: '2026-10-15T12:28:00Z' }), now)).toBe(false);
    expect(silent(run({ state: 'paused', heartbeatAt: '2026-10-15T12:00:00Z' }), now)).toBe(false);
  });
});
