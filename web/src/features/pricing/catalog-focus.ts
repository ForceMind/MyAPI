/** Keep control characters out of the CSS selector used by history focus.
 * JSON string encoding preserves the exact name, including quotes and lone surrogates.
 * This is a DOM marker only; routes and pricing data keep the original model_name.
 */
export function modelCatalogFocusKey(modelName: string): string {
  return JSON.stringify(modelName)
}

/** Pricing objects are identified by model_name; the wire DTO has no model id. */
export function findModelCatalogAction(modelName: string): HTMLElement | null {
  const focusKey = modelCatalogFocusKey(modelName)
  return [...document.querySelectorAll<HTMLElement>('[data-pricing-model]')]
    .find((element) => element.getAttribute('data-pricing-model') === focusKey) ?? null
}
