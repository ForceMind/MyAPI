export type ModelActionTarget =
  | { kind: 'edit' | 'description' | 'create' | 'tools'; id?: number }
  | { kind: 'missing-configure'; modelName: string; index: number }

/** Resolve only local, fixed-name action markers; no URL or selector text is accepted. */
export function findModelAction(target: ModelActionTarget): HTMLElement | null {
  if (target.kind === 'tools') return document.querySelector<HTMLElement>('[data-model-tools]')
  if (target.kind === 'create') return document.querySelector<HTMLElement>('[data-model-create]')
  if (target.kind === 'missing-configure') {
    const actions = [...document.querySelectorAll<HTMLElement>('[data-model-missing-configure]')]
    return actions.find((action) => action.dataset.modelMissingConfigure === target.modelName)
      ?? actions.at(target.index)
      ?? actions.at(-1)
      ?? document.querySelector<HTMLElement>('[data-model-missing-search]')
  }
  if (!Number.isSafeInteger(target.id) || (target.id ?? 0) <= 0) return null
  return document.querySelector<HTMLElement>(`[data-model-${target.kind}="${target.id}"]`)
}
