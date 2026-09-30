import type { OperationAction, OperationKind, OperationRunDetail, OperationRunSummary } from './models';

export type OperationRelatedStatus = NonNullable<OperationRunDetail['related_status']>;

export const operationKindLabels: Record<OperationKind, string> = {
  source_sync: 'Source sync',
  message_embedding: 'Message embedding',
  person_sweep: 'Person fact sweep',
  person_embedding: 'Person embedding',
  person_enrichment: 'Person enrichment',
  carddav_sync: 'CardDAV sync',
  document_extraction: 'Document extraction',
  document_embedding: 'Document embedding',
  visual_embedding: 'Visual embedding'
};

export const operationRelatedLabels: Record<OperationRelatedStatus, string> = {
  listSourceStatus: 'Sources status',
  getDocumentIndexStatus: 'Document index status',
  getDocumentVectorStatus: 'Document vector status',
  getVisualAttachmentStatus: 'Visual attachment status',
  getCardDAVStatus: 'CardDAV settings'
};

export const operationActionLabels: Record<OperationAction, string> = {
  carddav_sync: 'Start CardDAV sync',
  visual_build: 'Build visual index',
  visual_resume: 'Resume visual index'
};

export function operationStatusDot(run: OperationRunSummary) {
  if (run.state === 'running') return 'working' as const;
  if (run.state === 'queued') return 'waiting' as const;
  if (run.state === 'succeeded') return 'idle' as const;
  if (run.state === 'failed') return 'unclean' as const;
  return 'stale' as const;
}

export function titleCase(value: string | undefined): string {
  if (!value) return 'Unspecified';
  return value.charAt(0).toUpperCase() + value.slice(1);
}

export function formatOperationTimestamp(value: string): string {
  const parsed = new Date(value);
  if (!Number.isFinite(parsed.getTime())) return 'Time unavailable';
  return new Intl.DateTimeFormat(undefined, {
    dateStyle: 'medium', timeStyle: 'short'
  }).format(parsed);
}

export function operationDuration(run: OperationRunSummary): string {
  if (!run.finished_at) return run.state === 'running' ? 'In progress' : 'Not available';
  const milliseconds = Date.parse(run.finished_at) - Date.parse(run.started_at);
  if (!Number.isFinite(milliseconds) || milliseconds < 0) return 'Not available';
  const totalSeconds = Math.floor(milliseconds / 1_000);
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  if (minutes === 0) return `${seconds} ${seconds === 1 ? 'second' : 'seconds'}`;
  return `${minutes} ${minutes === 1 ? 'minute' : 'minutes'}${seconds ? ` ${seconds} ${seconds === 1 ? 'second' : 'seconds'}` : ''}`;
}
