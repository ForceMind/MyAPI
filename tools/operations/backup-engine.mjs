import { randomBytes } from 'node:crypto'
import { lstat, readdir, realpath } from 'node:fs/promises'
import path from 'node:path'
import { absolute, check, digest, protectedRead, safeCode } from './safety.mjs'
import { selectBackupRetention } from './backup-retention.mjs'
import { backupPrivateDirectory } from './config.mjs'
import { JournalStore } from './store.mjs'
import { DockerClient, containerDigest, validateBackupContainer } from './docker.mjs'
import { maxBackupBytes, requireCapacity, treeBytes } from './capacity.mjs'
import { python } from './python.mjs'

const isId = value => typeof value === 'string' && /^[a-f0-9]{64}$/.test(value)
const overlap = (left, right) => left === right || left.startsWith(right + path.sep) || right.startsWith(left + path.sep)

// Local host-administrator commands only. This engine does not expose an HTTP
// route, acquire model credentials, start a container or overwrite a directory.
// The CLI holds the existing daemon flock for the entire operation.
export class BackupEngine {
  constructor(config, dependencies = {}) {
    this.config = config
    this.docker = dependencies.docker || new DockerClient(config.docker_socket)
    this.store = dependencies.store || new JournalStore(config.state_dir)
    this.bundle = dependencies.bundle || (input => python('backup_data.py', input, 30 * 60_000))
    this.writer = dependencies.writer || (input => python('writer_guard.py', input, 120_000))
    this.capacity = dependencies.capacity || requireCapacity
    this.clock = dependencies.clock || (() => Math.floor(Date.now() / 1000))
  }

  async init() { await this.store.init(); return this }

  instance(id) {
    const instance = this.config.instances.find(value => value.id === id)
    check(instance, 'INSTANCE_NOT_FOUND', 404)
    return instance
  }

  get(id, kind) {
    check(isId(id), 'PLAN_NOT_FOUND', 404)
    const plan = this.store.plans.get(id)
    check(plan?.kind === kind, 'PLAN_NOT_FOUND', 404)
    return structuredClone(plan)
  }

  registryDigest(instance) {
    return digest({ instance, docker_socket: this.config.docker_socket, state_dir: this.config.state_dir, backups_dir: this.config.backups_dir })
  }

  assertPlan(plan) {
    check(plan.intent && plan.id === plan.intent.id && plan.kind === plan.intent.kind && plan.instance_id === plan.intent.instance_id &&
      plan.digest === digest(plan.intent) && plan.intent.registry_digest === this.registryDigest(this.instance(plan.instance_id)), 'PLAN_INTEGRITY_FAILED', 409)
  }

  sourceReference(instance) { return this.store.current.get(instance.id)?.target_container_id || instance.container }

  async scanWriters(instance, binding) {
    const result = await this.writer({ action: 'stopped_scan', data_dir: instance.data_dir, binding })
    check(result?.schema_version === 1 && result.action === 'stopped_scan' && result.status === 'observed' &&
      result.evidence?.old_generation_gone === true && digest(result.binding) === digest(binding), 'WRITER_STATE_UNCONFIRMED', 409)
    return result.evidence
  }

  async assertNoOtherContainer(instance, sourceId) {
    const containers = await this.docker.listContainers()
    check(Array.isArray(containers) && containers.length <= 10000, 'CONTAINER_INVENTORY_UNCONFIRMED', 503)
    const sourceStat = await lstat(instance.data_dir)
    const ids = new Set()
    let sourceSeen = false
    for (const listed of containers) {
      check(listed && isId(listed.Id) && !ids.has(listed.Id), 'CONTAINER_INVENTORY_UNCONFIRMED', 503)
      ids.add(listed.Id)
      const info = await this.docker.inspectContainer(listed.Id)
      check(info?.Id === listed.Id && Array.isArray(info.Mounts) && typeof info.State?.Running === 'boolean', 'CONTAINER_INVENTORY_UNCONFIRMED', 503)
      if (info.Id === sourceId) { sourceSeen = true; continue }
      let sharesData = false
      for (const mount of info.Mounts) {
        check(typeof mount.RW === 'boolean', 'CONTAINER_INVENTORY_UNCONFIRMED', 503)
        if (!mount.RW || mount.Type === 'tmpfs') continue
        check(['bind', 'volume'].includes(mount.Type) && absolute(mount.Source), 'CONTAINER_INVENTORY_UNCONFIRMED', 503)
        const actual = await realpath(mount.Source)
        const stat = await lstat(actual)
        if (overlap(actual, instance.data_dir) || stat.dev === sourceStat.dev && stat.ino === sourceStat.ino) sharesData = true
      }
      if (!sharesData) continue
      const retired = [...this.store.plans.values()].find(plan => (plan.kind ?? 'upgrade') === 'upgrade' && plan.state === 'succeeded' &&
        plan.instance_id === instance.id && plan.source_container_id === info.Id && plan.source_restart_disabled === true && plan.source_snapshot)
      check(retired && info.State.Running === false && info.State.Pid === 0 && info.State.Status === 'exited' && info.State.OOMKilled === false &&
        info.HostConfig?.RestartPolicy?.Name === 'no', 'OTHER_CONTAINER_CAN_WRITE_DATA', 409)
      const expected = structuredClone(retired.source_snapshot)
      expected.HostConfig.RestartPolicy = { Name: 'no', MaximumRetryCount: 0 }
      check(containerDigest(info) === containerDigest(expected), 'RETIRED_CONTAINER_CHANGED', 409)
    }
    check(sourceSeen, 'CONTAINER_INVENTORY_UNCONFIRMED', 503)
  }

  async assertStopped(instance, intent, expectedDigest) {
    const info = await this.docker.inspectContainer(intent.source_container_id)
    check(info?.Id === intent.source_container_id && info.Image === intent.source_image_id && info.State?.Status === 'exited' &&
      info.State.Running === false && info.State.Pid === 0 && info.State.ExitCode === 0 && info.State.OOMKilled === false &&
      info.HostConfig?.RestartPolicy?.Name === 'no' && info.HostConfig.RestartPolicy.MaximumRetryCount === 0 &&
      containerDigest(info) === expectedDigest, 'SOURCE_STOP_UNCONFIRMED', 409)
    check(await this.sourceReferenceMatches(instance, info.Id), 'SOURCE_INSTANCE_CHANGED', 409)
    await this.assertNoOtherContainer(instance, info.Id)
    return this.scanWriters(instance, intent.binding)
  }

  async sourceReferenceMatches(instance, id) {
    const current = await this.docker.inspectContainer(this.sourceReference(instance))
    return current?.Id === id
  }

  view(plan) {
    const intent = plan.intent
    return {
      schema_version: 1, id: plan.id, digest: plan.digest, kind: plan.kind, instance_id: plan.instance_id,
      state: plan.state, expires_at: intent.expires_at, source_container_id: intent.source_container_id,
      source_image_id: intent.source_image_id, backup_id: intent.backup_id ?? plan.id,
      original_restart_policy: intent.original_restart_policy ?? null,
      destination: intent.target_dir ?? path.join(this.config.backups_dir, plan.id),
      retention: { policy: 'keep-all', deletions: 0 },
      effects: plan.kind === 'backup'
        ? ['Stop the selected instance with SIGTERM and no forced kill.', 'Disable its automatic restart; leave it stopped.', 'Preserve a new private backup of its current data.']
        : ['Verify the backup and unchanged stopped source.', 'Copy into the selected new isolated directory.', 'Leave the original data and every container unchanged.'],
      restarts_instance: false, overwrites_directory: false, starts_recovery_instance: false,
      backup_created: typeof plan.backup_created === 'boolean' ? plan.backup_created : null,
      restored: typeof plan.restored === 'boolean' ? plan.restored : null,
      code: plan.code ?? null,
    }
  }

  async savePrepared(kind, instance, details) {
    check(this.store.plans.size < 9000 && !this.store.frozen.has(instance.id), 'INSTANCE_FROZEN', 409)
    const id = randomBytes(32).toString('hex')
    const now = this.clock()
    const intent = { id, kind, instance_id: instance.id, registry_digest: this.registryDigest(instance), created_at: now, expires_at: now + 600, ...details }
    const plan = { id, kind, instance_id: instance.id, state: 'prepared', created_at: now, expires_at: intent.expires_at, intent,
      digest: digest(intent), source_mutation_attempted: false, target_start_attempted: false, backup_created: false, restored: false }
    await this.capacity(this.config.state_dir, 4 * 1024 * 1024)
    await this.store.save(plan)
    return this.view(plan)
  }

  async prepareBackup(instanceId) {
    const instance = this.instance(instanceId)
    check(!this.store.frozen.has(instanceId), 'INSTANCE_FROZEN', 409)
    const source = await validateBackupContainer(await this.docker.inspectContainer(this.sourceReference(instance)), instance, this.docker)
    check(Number.isSafeInteger(source.info.State?.Pid) && source.info.State.Pid > 0 && source.info.State.Paused === false && source.info.State.Restarting === false,
      'SOURCE_PROCESS_UNCONFIRMED', 409)
    const restart = source.info.HostConfig.RestartPolicy
    check(restart && ['', 'no', 'always', 'unless-stopped', 'on-failure'].includes(restart.Name) &&
      Number.isSafeInteger(restart.MaximumRetryCount) && restart.MaximumRetryCount >= 0, 'SOURCE_RESTART_POLICY_UNCONFIRMED', 409)
    await this.assertNoOtherContainer(instance, source.info.Id)
    const observed = await this.writer({ action: 'bind_running', data_dir: instance.data_dir, host_pid: source.info.State.Pid })
    check(observed?.schema_version === 1 && observed.action === 'bind_running' && observed.status === 'observed' &&
      observed.binding?.source_process?.pid === source.info.State.Pid, 'SOURCE_PROCESS_UNCONFIRMED', 409)
    check(Number.isSafeInteger(observed.evidence?.tree_entries) && observed.evidence.tree_entries > 0 && observed.evidence.tree_entries <= 10000,
      'BACKUP_ENTRY_LIMIT_UNCONFIRMED', 409)
    const after = await this.docker.inspectContainer(source.info.Id)
    check(after?.State?.Running === true && after.State.Pid === source.info.State.Pid && containerDigest(after) === source.digest, 'SOURCE_CONFIG_CHANGED', 409)
    return this.savePrepared('backup', instance, { source_container_id: source.info.Id, source_image_id: source.info.Image,
      source_config_digest: source.digest, binding: observed.binding, original_restart_policy: structuredClone(restart) })
  }

  async assertRestoreTarget(target) {
    check(absolute(target) && target !== '/' && Buffer.byteLength(target) <= 4096 && !/[\u0000-\u001f\u007f-\u009f]/u.test(target), 'UNSAFE_RESTORE_TARGET')
    await backupPrivateDirectory(path.dirname(target))
    for (const value of [this.config.state_dir, this.config.backups_dir, ...this.config.instances.map(instance => instance.data_dir)]) {
      check(!overlap(target, value), 'RESTORE_TARGET_OVERLAP')
    }
    try { await lstat(target); check(false, 'RESTORE_TARGET_EXISTS', 409) }
    catch (error) { if (error.code !== 'ENOENT') throw error }
  }

  async verifyBackup(backup) {
    this.assertPlan(backup)
    check(backup.state === 'succeeded' && backup.backup_created === true && backup.bundle?.manifest_sha256 && backup.source_stopped_digest,
      'BACKUP_NOT_COMMITTED', 409)
    const result = await this.bundle({ action: 'bundle_verify', ...this.bundleInput(backup), expected_manifest_sha256: backup.bundle.manifest_sha256 })
    check(result?.verified === true && result.manifest_sha256 === backup.bundle.manifest_sha256, 'BACKUP_VERIFICATION_FAILED', 409)
    return result
  }

  bundleInput(backup) {
    const instance = this.instance(backup.instance_id)
    return { data_dir: instance.data_dir, sqlite_file: instance.sqlite_file, backups_dir: this.config.backups_dir, state_dir: this.config.state_dir,
      bundle_id: backup.id, identity_sha256: digest({ intent: backup.intent, source_stopped_digest: backup.source_stopped_digest }),
      max_bytes: maxBackupBytes, max_entries: 10000 }
  }

  async sourceManifest(instance) {
    const result = await this.bundle({ action: 'stopped_scan', data_dir: instance.data_dir, sqlite_file: instance.sqlite_file, max_bytes: maxBackupBytes, max_entries: 10000 })
    check(result?.schema_version === 1 && typeof result.tree_sha256 === 'string' && /^[a-f0-9]{64}$/.test(result.tree_sha256), 'SOURCE_MANIFEST_UNCONFIRMED', 503)
    return result
  }

  async archiveMounted(directory, manifest, containers) {
    const identities = new Set()
    // Ancestors catch an alias of the backup root; payload identities catch
    // aliases of individual files/subdirectories. Include stopped containers
    // and read-only mounts: their future readers must remain protected.
    const paths = [directory, path.join(directory, 'manifest.json'), path.join(directory, 'COMMITTED'),
      ...manifest.entries.map(entry => path.join(directory, 'data', entry.path))]
    let ancestor = path.dirname(directory)
    while (true) {
      paths.push(ancestor)
      if (ancestor === path.dirname(ancestor)) break
      ancestor = path.dirname(ancestor)
    }
    for (const filename of paths) {
      const stat = await lstat(filename, { bigint: true })
      check(!stat.isSymbolicLink(), 'RETENTION_MOUNT_STATE_UNKNOWN', 409)
      identities.add(`${stat.dev}:${stat.ino}`)
    }
    let used = false
    for (const container of containers) {
      check(isId(container.Id) && Array.isArray(container.Mounts), 'RETENTION_MOUNT_STATE_UNKNOWN', 409)
      for (const mount of container.Mounts) {
        if (mount.Type === 'tmpfs') continue
        check(['bind', 'volume'].includes(mount.Type) && absolute(mount.Source), 'RETENTION_MOUNT_STATE_UNKNOWN', 409)
        const filename = await realpath(mount.Source)
        const stat = await lstat(filename, { bigint: true })
        if (overlap(filename, directory) || identities.has(`${stat.dev}:${stat.ino}`)) used = true
      }
    }
    return used
  }

  async retentionCatalog(instanceId) {
    const instance = this.instance(instanceId)
    await backupPrivateDirectory(this.config.backups_dir)
    const names = (await readdir(this.config.backups_dir)).sort()
    check(names.length <= 10000, 'RETENTION_CATALOG_LIMIT', 409)
    const listed = await this.docker.listContainers()
    check(Array.isArray(listed) && listed.length <= 10000 && new Set(listed.map(item => item.Id)).size === listed.length,
      'RETENTION_MOUNT_STATE_UNKNOWN', 409)
    const containers = []
    for (const item of listed) {
      check(isId(item.Id), 'RETENTION_MOUNT_STATE_UNKNOWN', 409)
      const info = await this.docker.inspectContainer(item.Id)
      check(info?.Id === item.Id, 'RETENTION_MOUNT_STATE_UNKNOWN', 409)
      containers.push(info)
    }
    const references = [...this.store.plans.values()].filter(plan => plan.kind === 'restore').map(plan => {
      this.assertPlan(plan)
      check(isId(plan.intent.backup_id), 'RETENTION_REFERENCE_UNKNOWN', 409)
      return { id: plan.id, backup_id: plan.intent.backup_id, state: plan.state, digest: plan.digest }
    }).sort((left, right) => left.id.localeCompare(right.id))
    const archives = [], protectedIds = [], retainedUnknown = []
    const backups = [...this.store.plans.values()].filter(plan => plan.kind === 'backup' && plan.instance_id === instance.id)
    for (const id of names) {
      const backup = backups.find(plan => plan.id === id && plan.state === 'succeeded')
      if (!backup) { retainedUnknown.push({ name_sha256: digest(id), reason: 'unrecognized-or-uncommitted' }); continue }
      try {
        const verified = await this.verifyBackup(backup)
        const directory = path.join(this.config.backups_dir, id)
        const raw = await protectedRead(path.join(directory, 'manifest.json'), { max: 64 * 1024 * 1024 })
        check(digest(raw) === verified.manifest_sha256, 'RETENTION_MANIFEST_CHANGED', 409)
        const manifest = JSON.parse(raw)
        const root = await lstat(directory, { bigint: true })
        check(root.isDirectory() && root.uid === 0n && (root.mode & 0o7777n) === 0o700n && Array.isArray(manifest.entries), 'RETENTION_ARCHIVE_UNCONFIRMED', 409)
        archives.push({ id, instanceId, manifestSha256: verified.manifest_sha256, treeSha256: verified.tree_sha256,
          identitySha256: verified.identity_sha256, rootDev: root.dev.toString(), rootIno: root.ino.toString(),
          createdUnixSeconds: manifest.created_unix_seconds, logicalBytes: verified.total_bytes })
        let protectedArchive = references.some(reference => reference.backup_id === id)
        if (await this.archiveMounted(directory, manifest, containers)) protectedArchive = true
        try {
          const observed = await this.writer({ action: 'archive_scan', archive_dir: directory })
          check(observed?.status === 'observed' && observed.action === 'archive_scan' && observed.deletion_authorized === false &&
            observed.evidence?.manifest_verified === false && observed.binding?.archive_root?.dev === root.dev.toString() &&
            observed.binding.archive_root.ino === root.ino.toString(), 'RETENTION_ARCHIVE_USAGE_UNKNOWN', 409)
        } catch { protectedArchive = true }
        if (protectedArchive) protectedIds.push(id)
      } catch (error) {
        // Verification failure never makes an old or damaged directory a
        // deletion candidate. Do not infer missing sizes or timestamps.
        retainedUnknown.push({ name_sha256: digest(id), reason: safeCode(error) })
        const index = archives.findIndex(archive => archive.id === id)
        if (index !== -1) archives.splice(index, 1)
      }
    }
    const observedIds = new Set(archives.map(archive => archive.id))
    for (const reference of references) {
      const restore = this.store.plans.get(reference.id)
      if (restore.instance_id === instanceId) check(observedIds.has(reference.backup_id), 'RETENTION_REFERENCE_UNCONFIRMED', 409)
    }
    return { archives, protectedIds, retainedUnknown, references,
      registry_digest: this.registryDigest(instance), backup_journals: backups.map(plan => ({ id: plan.id, digest: plan.digest,
        state: plan.state, consumed_by: plan.prune_consumed_by ?? null })).sort((left, right) => left.id.localeCompare(right.id)) }
  }

  async preparePrune(instanceId, triggerId, policy) {
    check(!this.store.frozen.has(instanceId), 'INSTANCE_FROZEN', 409)
    const instance = this.instance(instanceId)
    const trigger = this.get(triggerId, 'backup')
    check(trigger.instance_id === instanceId && trigger.state === 'succeeded' && trigger.prune_consumed_by === undefined,
      'RETENTION_TRIGGER_UNAVAILABLE', 409)
    check(Number.isSafeInteger(trigger.created_at) && [...this.store.plans.values()].filter(plan =>
      plan.kind === 'backup' && plan.instance_id === instanceId && plan.state === 'succeeded').every(plan =>
      Number.isSafeInteger(plan.created_at) && plan.created_at <= trigger.created_at), 'RETENTION_TRIGGER_NOT_LATEST', 409)
    const catalog = await this.retentionCatalog(instanceId)
    const selection = selectBackupRetention({ instanceId, archives: catalog.archives, protectedIds: catalog.protectedIds,
      triggerId, asOf: this.clock(), ...policy })
    if (selection.policy.mode === 'keep-all' || selection.candidates.length === 0) return { schema_version: 1, kind: 'backup-prune',
      instance_id: instanceId, state: 'no_changes', selection, retained_unknown: catalog.retainedUnknown, deletion_authorized: false }
    const id = randomBytes(32).toString('hex'), now = this.clock()
    const intent = { id, kind: 'backup-prune', instance_id: instanceId, registry_digest: this.registryDigest(instance),
      created_at: now, expires_at: now + 600, trigger_id: triggerId, selection, catalog_digest: digest(catalog) }
    const plan = { id, kind: 'backup-prune', instance_id: instanceId, state: 'prepared', created_at: now, expires_at: intent.expires_at,
      intent, digest: digest(intent), results: [] }
    await this.capacity(this.config.state_dir, 4 * 1024 * 1024)
    await this.store.save(plan)
    return { schema_version: 1, id, digest: plan.digest, kind: plan.kind, instance_id: instanceId, state: plan.state,
      selection, expires_at: intent.expires_at, retained_unknown: catalog.retainedUnknown, deletion_authorized: false }
  }

  async executePrune(id, suppliedDigest, instanceId, triggerId, policy) {
    const plan = this.get(id, 'backup-prune')
    this.assertPlan(plan)
    const selectedPolicy = plan.intent.selection.policy
    check(plan.instance_id === instanceId && plan.intent.trigger_id === triggerId && plan.digest === suppliedDigest &&
      plan.state === 'prepared' && this.clock() < plan.intent.expires_at && !this.store.frozen.has(instanceId) &&
      (policy.keepCount ?? null) === selectedPolicy.keepCount && (policy.keepDays ?? null) === selectedPolicy.keepDays,
      'PLAN_CONFIRMATION_INVALID', 409)
    await this.store.lock(instanceId)
    let deletionAttempted = false
    try {
      plan.state = 'queued'; await this.store.save(plan)
      const catalog = await this.retentionCatalog(instanceId)
      check(digest(catalog) === plan.intent.catalog_digest, 'RETENTION_CATALOG_CHANGED', 409)
      const current = selectBackupRetention({ instanceId, archives: catalog.archives, protectedIds: catalog.protectedIds,
        triggerId, asOf: plan.intent.selection.asOf, ...policy })
      check(current.selectionDigest === plan.intent.selection.selectionDigest, 'RETENTION_SELECTION_CHANGED', 409)
      const trigger = this.get(triggerId, 'backup')
      check(trigger.prune_consumed_by === undefined && trigger.state === 'succeeded', 'RETENTION_TRIGGER_UNAVAILABLE', 409)
      check(this.clock() < plan.intent.expires_at, 'PLAN_EXPIRED', 409)
      // Consume the fresh-backup trigger before the first irreversible step.
      // A crash cannot turn it into an authorization to expand/retry deletion.
      trigger.prune_consumed_by = id
      await this.store.save(trigger)
      plan.state = 'pruning'; await this.store.save(plan)
      for (const candidate of current.candidates) {
        check(![...this.store.plans.values()].some(value => value.kind === 'restore' && value.intent?.backup_id === candidate.id),
          'RETENTION_REFERENCE_CHANGED', 409)
        const backup = this.get(candidate.id, 'backup')
        await this.verifyBackup(backup)
        const directory = path.join(this.config.backups_dir, candidate.id)
        const raw = await protectedRead(path.join(directory, 'manifest.json'), { max: 64 * 1024 * 1024 })
        check(digest(raw) === candidate.manifestSha256, 'RETENTION_MANIFEST_CHANGED', 409)
        const manifest = JSON.parse(raw)
        const listed = await this.docker.listContainers()
        check(Array.isArray(listed) && listed.length <= 10000, 'RETENTION_MOUNT_STATE_UNKNOWN', 409)
        const containers = []
        for (const item of listed) {
          check(isId(item.Id), 'RETENTION_MOUNT_STATE_UNKNOWN', 409)
          const info = await this.docker.inspectContainer(item.Id)
          check(info?.Id === item.Id, 'RETENTION_MOUNT_STATE_UNKNOWN', 409)
          containers.push(info)
        }
        check(!await this.archiveMounted(directory, manifest, containers), 'RETENTION_ARCHIVE_MOUNTED', 409)
        const root = await lstat(directory, { bigint: true })
        check(root.dev.toString() === candidate.rootDev && root.ino.toString() === candidate.rootIno && root.uid === 0n &&
          (root.mode & 0o7777n) === 0o700n, 'RETENTION_ROOT_CHANGED', 409)
        deletionAttempted = true
        plan.results.push({ id: candidate.id, state: 'unconfirmed' })
        await this.store.save(plan)
        const result = await this.bundle({ action: 'bundle_prune', ...this.bundleInput(backup), expected_manifest_sha256: candidate.manifestSha256,
          expected_root: { dev: candidate.rootDev, ino: candidate.rootIno, uid: 0, gid: Number(root.gid), mode: '0700' } })
        check(result?.deleted === true && result.bundle_id === candidate.id && result.manifest_sha256 === candidate.manifestSha256 &&
          result.logical_bytes === candidate.logicalBytes, 'RETENTION_DELETE_UNCONFIRMED', 503)
        plan.results[plan.results.length - 1] = { id: candidate.id, state: 'deleted', logical_bytes: result.logical_bytes }
        backup.state = 'pruned'; backup.pruned_by = id; backup.backup_created = false
        await this.store.save(backup)
        await this.store.save(plan)
      }
      plan.state = 'succeeded'; await this.store.save(plan)
      await this.store.unlock(instanceId)
    } catch (error) {
      plan.code = safeCode(error)
      plan.state = deletionAttempted ? 'recovery_required' : 'failed_before_start'
      if (deletionAttempted) this.store.frozen.add(instanceId)
      await this.store.save(plan)
      if (!deletionAttempted) await this.store.unlock(instanceId)
    }
    return { schema_version: 1, id, digest: plan.digest, kind: 'backup-prune', instance_id: instanceId, state: plan.state,
      results: plan.results, code: plan.code ?? null, source_changed: false, automatic_retry: false }
  }

  async prepareRestore(instanceId, backupId, target) {
    const instance = this.instance(instanceId)
    check(!this.store.frozen.has(instanceId), 'INSTANCE_FROZEN', 409)
    const backup = this.get(backupId, 'backup')
    check(backup.instance_id === instanceId, 'BACKUP_INSTANCE_MISMATCH', 409)
    await this.verifyBackup(backup)
    await this.assertStopped(instance, backup.intent, backup.source_stopped_digest)
    await this.assertRestoreTarget(target)
    const source = await this.sourceManifest(instance)
    check(source.tree_sha256 === backup.bundle.tree_sha256, 'SOURCE_CHANGED_SINCE_BACKUP', 409)
    return this.savePrepared('restore', instance, { source_container_id: backup.intent.source_container_id, source_image_id: backup.intent.source_image_id,
      source_stopped_digest: backup.source_stopped_digest, binding: backup.intent.binding, backup_id: backup.id,
      manifest_sha256: backup.bundle.manifest_sha256, source_tree_sha256: source.tree_sha256, target_dir: target,
      original_restart_policy: backup.intent.original_restart_policy ?? null })
  }

  async execute(id, suppliedDigest, kind, instanceId, details = {}) {
    const plan = this.get(id, kind)
    this.assertPlan(plan)
    check(plan.instance_id === instanceId && plan.digest === suppliedDigest && plan.state === 'prepared' && this.clock() < plan.intent.expires_at,
      'PLAN_CONFIRMATION_INVALID', 409)
    if (kind === 'restore') check(plan.intent.backup_id === details.backupId && plan.intent.target_dir === details.target, 'PLAN_CONFIRMATION_INVALID', 409)
    check(!this.store.frozen.has(instanceId), 'INSTANCE_FROZEN', 409)
    const instance = this.instance(instanceId)
    await this.store.lock(instanceId)
    let mutationAttempted = false
    const transition = async state => { plan.state = state; await this.store.save(plan) }
    try {
      await transition('queued')
      if (kind === 'backup') {
        const source = await validateBackupContainer(await this.docker.inspectContainer(plan.intent.source_container_id), instance, this.docker)
        check(source.digest === plan.intent.source_config_digest && await this.sourceReferenceMatches(instance, source.info.Id), 'SOURCE_CONFIG_CHANGED', 409)
        const binding = await this.writer({ action: 'bind_running', data_dir: instance.data_dir, host_pid: source.info.State.Pid })
        check(binding?.status === 'observed' && digest(binding.binding) === digest(plan.intent.binding), 'SOURCE_PROCESS_CHANGED', 409)
        check(Number.isSafeInteger(binding.evidence?.tree_entries) && binding.evidence.tree_entries > 0 && binding.evidence.tree_entries <= 10000,
          'BACKUP_ENTRY_LIMIT_UNCONFIRMED', 409)
        await this.assertNoOtherContainer(instance, source.info.Id)
        const size = await treeBytes(instance.data_dir, maxBackupBytes)
        await this.capacity(this.config.backups_dir, size * 3 + 1024 * 1024)
        check(this.clock() < plan.intent.expires_at, 'PLAN_EXPIRED', 409)
        // Journal the risk before even attempting to change restart policy.
        mutationAttempted = true; plan.source_mutation_attempted = true; plan.source_restart_disabled = null
        await transition('stopping')
        const expected = structuredClone(source.info)
        expected.HostConfig.RestartPolicy = { Name: 'no', MaximumRetryCount: 0 }
        const stoppedDigest = containerDigest(expected)
        await this.docker.disableRestart(source.info.Id)
        const disabled = await this.docker.inspectContainer(source.info.Id)
        check(disabled?.State?.Running === true && disabled.State.Pid === source.info.State.Pid && containerDigest(disabled) === stoppedDigest,
          'SOURCE_RESTART_DISABLE_UNCONFIRMED', 503)
        plan.source_restart_disabled = true
        await this.store.save(plan)
        await this.docker.stopForBackup(source.info.Id)
        plan.writer_before = await this.assertStopped(instance, plan.intent, stoppedDigest)
        plan.source_stopped_digest = stoppedDigest
        plan.backup_created = null
        await transition('backing_up')
        const input = this.bundleInput(plan)
        const prepared = await this.bundle({ action: 'bundle_prepare', ...input })
        check(prepared?.prepared === true && /^[a-f0-9]{64}$/.test(prepared.manifest_sha256 ?? '') && /^[a-f0-9]{64}$/.test(prepared.tree_sha256 ?? ''), 'BACKUP_PREPARATION_FAILED', 503)
        plan.backup_created = false // A confirmed stage has no commit marker.
        plan.writer_after = await this.assertStopped(instance, plan.intent, stoppedDigest)
        plan.backup_created = null
        await this.store.save(plan)
        plan.bundle = await this.bundle({ action: 'bundle_commit', ...input, expected_manifest_sha256: prepared.manifest_sha256, expected_tree_sha256: prepared.tree_sha256 })
        check(plan.bundle?.committed === true && plan.bundle.manifest_sha256 === prepared.manifest_sha256, 'BACKUP_COMMIT_FAILED', 503)
        plan.backup_created = true
      } else {
        const backup = this.get(plan.intent.backup_id, 'backup')
        check(backup.instance_id === instanceId && backup.bundle?.manifest_sha256 === plan.intent.manifest_sha256, 'BACKUP_INSTANCE_MISMATCH', 409)
        await this.verifyBackup(backup)
        await this.assertStopped(instance, plan.intent, plan.intent.source_stopped_digest)
        await this.assertRestoreTarget(plan.intent.target_dir)
        const source = await this.sourceManifest(instance)
        check(source.tree_sha256 === plan.intent.source_tree_sha256, 'SOURCE_CHANGED_SINCE_BACKUP', 409)
        await this.capacity(path.dirname(plan.intent.target_dir), backup.bundle.total_bytes * 2 + 1024 * 1024)
        mutationAttempted = true
        plan.restored = null
        await transition('backing_up')
        const restored = await this.bundle({ action: 'bundle_restore', ...this.bundleInput(backup), expected_manifest_sha256: plan.intent.manifest_sha256, destination: plan.intent.target_dir })
        check(restored?.restored === true && restored.tree_sha256 === plan.intent.source_tree_sha256, 'RESTORE_VERIFICATION_FAILED', 503)
        await this.assertStopped(instance, plan.intent, plan.intent.source_stopped_digest)
        const after = await this.sourceManifest(instance)
        check(after.tree_sha256 === source.tree_sha256, 'RESTORE_SOURCE_CHANGED', 409)
        plan.restored = true; plan.restore_result = restored
      }
      await transition('succeeded')
      await this.store.unlock(instanceId)
    } catch (error) {
      plan.code = safeCode(error)
      plan.state = mutationAttempted ? 'recovery_required' : 'failed_before_start'
      if (mutationAttempted) this.store.frozen.add(instanceId)
      await this.store.save(plan)
      if (!mutationAttempted) await this.store.unlock(instanceId)
    }
    return this.view(plan)
  }
}
