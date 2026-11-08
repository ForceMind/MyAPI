import { randomBytes } from 'node:crypto'
import { mkdir, open, readdir, rename, unlink } from 'node:fs/promises'
import path from 'node:path'
import { check, privateDirectory, protectedRead } from './safety.mjs'

export const activeStates = new Set(['queued', 'verifying_artifact', 'stopping', 'backing_up', 'creating', 'starting', 'verifying'])
const visible = ['id', 'kind', 'digest', 'instance_id', 'owner_id', 'key_id', 'state', 'created_at', 'expires_at', 'approved_at', 'approval_expires_at', 'approved_by',
  'source_image_id', 'target_image_id', 'source_container_id', 'target_container_id', 'source_config_digest', 'source_version', 'source_sha', 'tree_sha', 'run_id',
  'producer_attempt', 'artifact_id', 'manifest_sha256', 'version', 'platform', 'backup_created', 'target_start_attempted', 'source_mutation_attempted', 'source_restart_disabled', 'code', 'checks']
export function publicPlan(plan) { return Object.fromEntries(visible.filter(k => plan[k] !== undefined).map(k => [k, structuredClone(plan[k])])) }

export class JournalStore {
  constructor(directory) { this.directory = directory; this.plans = new Map(); this.frozen = new Set(); this.current = new Map() }
  async init() {
    await privateDirectory(this.directory)
    const entries = await readdir(this.directory)
    check(entries.length <= 10000, 'JOURNAL_LIMIT', 503)
    for (const name of entries.filter(name => /^[a-f0-9]{64}\.json$/.test(name)).sort()) {
      let plan
      try { plan = JSON.parse(await protectedRead(path.join(this.directory, name), { uid: process.getuid(), max: 4 * 1024 * 1024 })) } catch { check(false, 'JOURNAL_CORRUPT', 503) }
      check(plan.id + '.json' === name && typeof plan.instance_id === 'string', 'JOURNAL_CORRUPT', 503)
      check(plan.kind === undefined || ['upgrade', 'backup', 'restore'].includes(plan.kind), 'JOURNAL_CORRUPT', 503)
      if (activeStates.has(plan.state)) { plan.state = 'interrupted'; plan.code = 'EXECUTION_INTERRUPTED'; await this.save(plan) }
      this.plans.set(plan.id, plan)
      if (['interrupted', 'recovery_required'].includes(plan.state)) this.frozen.add(plan.instance_id)
    }
    const upgrades = [...this.plans.values()].filter(plan => plan.state === 'succeeded' && (plan.kind ?? 'upgrade') === 'upgrade')
    const instances = new Set(upgrades.map(plan => plan.instance_id))
    for (const instance of instances) {
      const succeeded = upgrades.filter(plan => plan.instance_id === instance)
      const outgoing = new Map(), incoming = new Map()
      let valid = true
      for (const plan of succeeded) {
        if (typeof plan.source_container_id !== 'string' || typeof plan.target_container_id !== 'string' || outgoing.has(plan.source_container_id) || incoming.has(plan.target_container_id)) valid = false
        outgoing.set(plan.source_container_id, plan); incoming.set(plan.target_container_id, plan)
      }
      const roots = succeeded.filter(plan => !incoming.has(plan.source_container_id))
      const tips = succeeded.filter(plan => !outgoing.has(plan.target_container_id))
      const visited = new Set()
      let next = roots.length === 1 ? roots[0] : undefined
      while (next && !visited.has(next.id)) { visited.add(next.id); next = outgoing.get(next.target_container_id) }
      valid = valid && roots.length === 1 && tips.length === 1 && !next && visited.size === succeeded.length
      if (valid) this.current.set(instance, tips[0])
      else this.frozen.add(instance)
    }
    // A lock without a journal can be the crash window before queued was flushed.
    for (const name of entries.filter(name => /^instance-[a-z0-9-]+\.lock$/.test(name))) this.frozen.add(name.slice(9, -5))
  }
  async save(plan) {
    const filename = path.join(this.directory, plan.id + '.json')
    const temporary = filename + '.tmp-' + randomBytes(8).toString('hex')
    // One daemon and one operation per instance; stale temporary files are never followed.
    const file = await open(temporary, 'wx', 0o600)
    try { await file.writeFile(JSON.stringify(plan)); await file.sync() } finally { await file.close() }
    await rename(temporary, filename)
    const directory = await open(this.directory, 'r')
    try { await directory.sync() } finally { await directory.close() }
    this.plans.set(plan.id, plan)
  }
  async lock(instance) {
    let file
    try { file = await open(path.join(this.directory, `instance-${instance}.lock`), 'wx', 0o600) } catch { check(false, 'INSTANCE_LOCKED', 409) }
    try { await file.writeFile('locked\n'); await file.sync() } finally { await file.close() }
    const directory = await open(this.directory, 'r')
    try { await directory.sync() } finally { await directory.close() }
  }
  async unlock(instance) { await unlink(path.join(this.directory, `instance-${instance}.lock`)) }
  async stage(id) { const directory = path.join(this.directory, `stage-${id}`); await mkdir(directory, { mode: 0o700 }); return directory }
}
