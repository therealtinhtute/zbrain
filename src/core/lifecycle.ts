// Note lifecycle state machine. See SPEC §7 AD-4 and V2-ARCH §4.
//
// Transitions:
//   active       -> superseded (via supersedeNote; old note flipped)
//   active       -> archived   (out of retrieval, on disk)
//   archived     -> forgotten  (moved to .trash/<id>.md + tombstone)
//   superseded   -> active     (restore, if no current successor)
//   forgotten    -> active     (restore from .trash/)

import { NoteStatuses, type NoteStatus } from "./note-service";

export class InvalidTransitionError extends Error {
  constructor(from: NoteStatus, to: NoteStatus) {
    super(`Invalid lifecycle transition: ${from} -> ${to}`);
    this.name = "InvalidTransitionError";
  }
}

const ALLOWED: Record<NoteStatus, NoteStatus[]> = {
  active: ["superseded", "archived", "forgotten"],
  superseded: ["active", "archived", "forgotten"],
  archived: ["active", "forgotten"],
  forgotten: ["active"],
};

export function canTransition(from: NoteStatus, to: NoteStatus): boolean {
  if (from === to) return false;
  return (ALLOWED[from] ?? []).includes(to);
}

export function assertTransition(from: NoteStatus, to: NoteStatus): void {
  if (!canTransition(from, to)) {
    throw new InvalidTransitionError(from, to);
  }
}

export function isNoteStatus(value: string): value is NoteStatus {
  return (NoteStatuses as string[]).includes(value);
}
