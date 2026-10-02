// Stable per user-confirmed operation, including retries after an uncertain
// network result. Callers must keep the returned ID until that operation ends.
export function createOperationId(): string {
  return Array.from(crypto.getRandomValues(new Uint8Array(32)), (value) =>
    value.toString(16).padStart(2, '0')
  ).join('')
}
