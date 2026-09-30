import { describe, expect, it } from 'vitest';

import type { OperationRunSummary } from './models';
import {
  formatOperationTimestamp,
  operationActionLabels,
  operationDuration,
  operationKindLabels,
  operationRelatedLabels,
  operationStatusDot,
  titleCase
} from './presentation';

function run(patch: Partial<OperationRunSummary> = {}): OperationRunSummary {
  return {
    id: 'run', kind: 'source_sync', lane: 'messages', state: 'succeeded', trigger: 'manual',
    started_at: '2026-08-30T10:00:00Z', finished_at: '2026-08-30T10:00:00Z', counters: [], ...patch
  };
}

function finishedAfter(seconds: number): OperationRunSummary {
  return run({ finished_at: new Date(Date.parse('2026-08-30T10:00:00Z') + seconds * 1_000).toISOString() });
}

describe('operation presentation', () => {
  it('names every kind, related status and action', () => {
    expect(operationKindLabels.person_sweep).toBe('Person fact sweep');
    expect(operationKindLabels.carddav_sync).toBe('CardDAV sync');
    expect(operationRelatedLabels.getCardDAVStatus).toBe('CardDAV settings');
    expect(operationActionLabels.visual_resume).toBe('Resume visual index');
  });

  it('maps each run state to its status dot', () => {
    expect(operationStatusDot(run({ state: 'running' }))).toBe('working');
    expect(operationStatusDot(run({ state: 'queued' }))).toBe('waiting');
    expect(operationStatusDot(run({ state: 'succeeded' }))).toBe('idle');
    expect(operationStatusDot(run({ state: 'failed' }))).toBe('unclean');
    expect(operationStatusDot(run({ state: 'cancelled' }))).toBe('stale');
    expect(operationStatusDot(run({ state: 'partial' }))).toBe('stale');
  });

  it('title-cases values and names a missing one', () => {
    expect(titleCase('manual')).toBe('Manual');
    expect(titleCase(undefined)).toBe('Unspecified');
    expect(titleCase('')).toBe('Unspecified');
  });

  it('formats valid timestamps and names invalid ones', () => {
    expect(formatOperationTimestamp('not a time')).toBe('Time unavailable');
    expect(formatOperationTimestamp('2026-08-30T10:00:00Z')).not.toBe('Time unavailable');
  });

  it('spells out durations', () => {
    expect(operationDuration(finishedAfter(0))).toBe('0 seconds');
    expect(operationDuration(finishedAfter(1))).toBe('1 second');
    expect(operationDuration(finishedAfter(61))).toBe('1 minute 1 second');
    expect(operationDuration(finishedAfter(120))).toBe('2 minutes');
    expect(operationDuration(run({ state: 'running', finished_at: undefined }))).toBe('In progress');
    expect(operationDuration(run({ state: 'queued', finished_at: undefined }))).toBe('Not available');
    expect(operationDuration(finishedAfter(-5))).toBe('Not available');
  });
});
