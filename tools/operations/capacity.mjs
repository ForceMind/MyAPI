import { lstat, readdir, statfs } from 'node:fs/promises'
import path from 'node:path'
import { check } from './safety.mjs'

const GiB = 1024 ** 3
export async function treeBytes(directory, limit) {
  let bytes = 0, entries = 0
  async function visit(filename) {
    const stat = await lstat(filename)
    check(!stat.isSymbolicLink(), 'UNSAFE_DATA_LINK')
    check(++entries <= 200000, 'STORAGE_ENTRY_LIMIT', 503)
    if (stat.isDirectory()) for (const name of await readdir(filename)) await visit(path.join(filename, name))
    else { check(stat.isFile() && stat.nlink === 1, 'UNSAFE_DATA_FILE'); bytes += stat.size; check(bytes <= limit, 'STORAGE_QUOTA_EXCEEDED', 503) }
  }
  await visit(directory)
  return bytes
}
export async function requireCapacity(directory, requested, quota = 32 * GiB) {
  check(Number.isSafeInteger(requested) && requested >= 0 && requested <= quota, 'STORAGE_QUOTA_EXCEEDED', 503)
  const used = await treeBytes(directory, quota)
  const filesystem = await statfs(directory)
  check(used + requested <= quota && filesystem.bavail * filesystem.bsize >= requested + 2 * GiB, 'STORAGE_CAPACITY_LOW', 503)
}
export const maxBackupBytes = 16 * GiB
export const maxTarBytes = 8 * GiB
