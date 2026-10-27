import type { WeighingState } from '@/types/weighing';

// Device readings and unrelated list refreshes do not change a frozen review.
export function sameWeighingReview(before: WeighingState, after: WeighingState, taskId?: string): boolean {
  if (!after.sampleReady || !after.capture || before.capture?.id !== after.capture.id || before.owner !== after.owner || after.pending) return false;
  if (taskId) {
    const previous = before.tasks.find(task => task.id === taskId);
    const current = after.tasks.find(task => task.id === taskId);
    return !!previous && !!current && previous.version === current.version && previous.status === current.status;
  }
  return before.draftRequestId === after.draftRequestId
    && JSON.stringify(before.draft) === JSON.stringify(after.draft)
    && JSON.stringify(before.materials.find(item => item.id === before.draft.materialId)) === JSON.stringify(after.materials.find(item => item.id === after.draft.materialId))
    && [before.draft.partyId, before.draft.carrierId].every(id => JSON.stringify(before.parties.find(item => item.id === id)) === JSON.stringify(after.parties.find(item => item.id === id)));
}
