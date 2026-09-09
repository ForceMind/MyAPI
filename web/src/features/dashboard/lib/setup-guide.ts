/**
 * Resolve the setup guide visibility for the current user and guide version.
 * Completion is terminal even when a stale local preference says "expanded".
 */
export function resolveSetupGuideExpanded(
  setupStatusReady: boolean,
  setupComplete: boolean,
  manualExpanded: boolean | null
): boolean {
  // The overview prioritizes operational status. New or returning users can
  // still expand the guide, but it should not displace the dashboard on load.
  return setupStatusReady && !setupComplete && (manualExpanded ?? false)
}
