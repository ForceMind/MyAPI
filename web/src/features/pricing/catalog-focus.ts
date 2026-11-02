/** Pricing objects are identified by model_name; the wire DTO has no model id. */
export function findModelCatalogAction(modelName: string): HTMLElement | null {
  return Array.from(document.querySelectorAll<HTMLElement>('[data-pricing-model]'))
    .find((element) => element.getAttribute('data-pricing-model') === modelName) ?? null
}
