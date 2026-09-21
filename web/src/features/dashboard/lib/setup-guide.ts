/**
 * Resolve the setup guide visibility for the current user and guide version.
 * Completion is terminal even when a stale local preference says "expanded".
 */
export function resolveSetupGuideExpanded(
  setupStatusReady: boolean,
  setupComplete: boolean,
  manualExpanded: boolean | null
): boolean {
  // The overview starts with active operational information. New and
  // returning users can expand the guide when they need it.
  return setupStatusReady && !setupComplete && (manualExpanded ?? false)
}
