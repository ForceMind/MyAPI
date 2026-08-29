/**
 * Resolve the setup guide visibility for the current user and guide version.
 * Completion is terminal even when a stale local preference says "expanded".
 */
export function resolveSetupGuideExpanded(
  setupStatusReady: boolean,
  setupComplete: boolean,
  manualExpanded: boolean | null
): boolean {
  return setupStatusReady && !setupComplete && (manualExpanded ?? true)
}
