/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/

/**
 * The recorded outcome of one relay attempt as an i18n key. "Selected"
 * means current when the record was written, not confirmed completion.
 */
export function relayAttemptOutcomeLabel(outcome: string): string {
  switch (outcome) {
    case 'selected':
      return 'Selected at log time'
    case 'refused':
      return 'Refused'
    case 'retryable_refusal':
      return 'Refused; retry allowed'
    case 'failed':
      return 'Failed'
    case 'completed':
      return 'Completed'
    default:
      return 'Unknown outcome'
  }
}
