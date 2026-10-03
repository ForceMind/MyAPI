import { z } from 'zod'

// Match the server's bounded decimal encoding without converting money to Number.
export const usdAmountSchema = z
  .string()
  .min(1)
  .max(128)
  .regex(/^(0|[1-9][0-9]*)(\.[0-9]+)?$/)
export function usdLessThan(left: string, right: string): boolean {
  if (
    !usdAmountSchema.safeParse(left).success ||
    !usdAmountSchema.safeParse(right).success
  ) {
    return false
  }
  const [li = '', lf = ''] = left.split('.')
  const [ri = '', rf = ''] = right.split('.')
  const scale = Math.max(lf.length, rf.length)
  return BigInt(li + lf.padEnd(scale, '0')) < BigInt(ri + rf.padEnd(scale, '0'))
}
