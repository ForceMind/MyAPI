export type ModelActionTarget = { kind: 'edit' | 'description' | 'create'; id?: number }

/** Resolve only local, fixed-name action markers; no URL or selector text is accepted. */
export function findModelAction(target: ModelActionTarget): HTMLElement | null {
  if (target.kind === 'create') return document.querySelector<HTMLElement>('[data-model-create]')
  if (!Number.isSafeInteger(target.id) || (target.id ?? 0) <= 0) return null
  return document.querySelector<HTMLElement>(`[data-model-${target.kind}="${target.id}"]`)
}
