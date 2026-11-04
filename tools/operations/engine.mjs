import { randomBytes } from 'node:crypto'
import path from 'node:path'
import { artifactInput, check, digest, safeCode } from './safety.mjs'
import { JournalStore, publicPlan } from './store.mjs'
import { DockerClient, validateContainer, candidateConfig, containerDigest, sameRuntimeConfig, samePublishedPorts } from './docker.mjs'
import { GitHubClient, verifyBundle } from './github.mjs'
import { validateManagementKey, validateActor } from './auth.mjs'
import { readVersion, verifyHealth } from './health.mjs'
import { requireCapacity, treeBytes, maxBackupBytes, maxTarBytes } from './capacity.mjs'
import { python } from './python.mjs'

export class OperationsEngine {
  constructor(config, dependencies = {}) {
    this.config = config
    this.docker = dependencies.docker || new DockerClient(config.docker_socket)
    this.github = dependencies.github || new GitHubClient(config.github_token_file)
    this.validateManagementKey = dependencies.validateManagementKey || validateManagementKey
    this.validateActor = dependencies.validateActor || validateActor
    this.verifyBundle = dependencies.verifyBundle || verifyBundle
    this.readVersion = dependencies.readVersion || readVersion
    this.verifyHealth = dependencies.verifyHealth || verifyHealth
    this.backup = dependencies.backup || (request => python('backup_data.py', request, 30 * 60_000))
    this.capacity = dependencies.capacity || requireCapacity
    this.dataBytes = dependencies.dataBytes || (directory => treeBytes(directory, maxBackupBytes))
    this.clock = dependencies.clock || (() => Math.floor(Date.now() / 1000))
    this.store = dependencies.store || new JournalStore(config.state_dir)
    this.busy = new Set(); this.mutations = new Set(); this.tasks = new Set()
  }
  async init() { await this.store.init(); return this }
  instance(id) { const value = this.config.instances.find(instance => instance.id === id); check(value, 'INSTANCE_NOT_FOUND', 404); return value }
  get(id) { check(typeof id === 'string' && /^[a-f0-9]{64}$/.test(id), 'PLAN_NOT_FOUND', 404); const value = this.store.plans.get(id); check(value, 'PLAN_NOT_FOUND', 404); return structuredClone(value) }
  authorizePlan(plan, identity, instanceId) {
    check(plan.owner_id === identity.owner_id && (!identity.key_id || plan.key_id === identity.key_id) && (!instanceId || plan.instance_id === instanceId), 'PLAN_NOT_FOUND', 404)
  }
  list(identity, instanceId) {
    const plans = [...this.store.plans.values()].filter(p => p.owner_id === identity.owner_id && (!identity.key_id || p.key_id === identity.key_id) && p.instance_id === instanceId)
      .sort((a, b) => b.created_at - a.created_at)
    const terminal = new Set(['succeeded', 'failed_before_start'])
    return [...plans.filter(p => !terminal.has(p.state)), ...plans.filter(p => terminal.has(p.state)).slice(0, 100)].map(publicPlan)
  }
  async instanceStatus(instance) {
    const result = { id: instance.id, installation: 'docker', edition: 'full', database: 'sqlite', platform: 'linux/amd64', frozen: this.store.frozen.has(instance.id) }
    const unresolved = [...this.store.plans.values()].filter(p => p.instance_id === instance.id && p.target_container_id && !['succeeded', 'failed_before_start'].includes(p.state)).sort((a, b) => b.created_at - a.created_at)[0]
    const reference = unresolved?.target_container_id || this.store.current.get(instance.id)?.target_container_id || instance.container
    try {
      const info = await this.docker.inspectContainer(reference)
      check(/^[a-f0-9]{64}$/.test(info.Id) && /^sha256:[a-f0-9]{64}$/.test(info.Image) && typeof info.State?.Running === 'boolean', 'INSTANCE_UNAVAILABLE', 503)
      return { ...result, state: result.frozen ? 'recovery_required' : this.busy.has(instance.id) ? 'in_progress' : info.State.Running ? 'running' : 'stopped', current_container_id: info.Id, current_image_id: info.Image }
    } catch { return { ...result, state: result.frozen ? 'recovery_required' : this.busy.has(instance.id) ? 'in_progress' : 'unknown', code: 'INSTANCE_UNAVAILABLE' } }
  }
  assertIntegrity(plan) {
    const bound = { id: plan.id, instance_id: plan.instance_id, owner_id: plan.owner_id, key_id: plan.key_id, created_at: plan.created_at, expires_at: plan.expires_at,
      source_container_id: plan.source_container_id, source_config_digest: plan.source_config_digest, source_version: plan.source_version, artifact: plan.artifact, verified_artifact: plan.verified_artifact, registry_digest: plan.registry_digest, platform: plan.platform }
    check(plan.registry_digest === digest(this.instance(plan.instance_id)) && plan.digest === digest(bound) && plan.platform === 'linux/amd64' && plan.source_snapshot?.Id === plan.source_container_id &&
      plan.source_snapshot?.Image === plan.source_image_id && containerDigest(plan.source_snapshot) === plan.source_config_digest &&
      plan.target_image_id === plan.artifact.image_id && ['source_sha', 'tree_sha', 'run_id', 'producer_attempt', 'artifact_id', 'manifest_sha256', 'version'].every(key => plan[key] === plan.artifact[key]), 'PLAN_INTEGRITY_FAILED', 409)
    artifactInput(plan.artifact)
  }
  async prepare(instanceId, artifact, identity) {
    artifact = artifactInput(artifact)
    const instance = this.instance(instanceId)
    check(!this.store.frozen.has(instanceId) && !this.busy.has(instanceId), 'INSTANCE_FROZEN', 409)
    const current = this.store.current.get(instanceId)?.target_container_id || instance.container
    const source = await validateContainer(await this.docker.inspectContainer(current), instance, this.docker)
    const sourceVersion = await this.readVersion(instance)
    const versionPattern = /^0\.2\.0-beta\.([1-9][0-9]{0,8})$/
    const sourceMatch = versionPattern.exec(sourceVersion), targetMatch = versionPattern.exec(artifact.version)
    check(sourceMatch && targetMatch && Number(targetMatch[1]) > Number(sourceMatch[1]), 'VERSION_NOT_FORWARD')
    const verified = await this.github.verify(artifact)
    check(/^[a-f0-9]{64}$/.test(verified.zip_sha256) && Number.isSafeInteger(verified.zip_bytes), 'GITHUB_ARTIFACT_MISMATCH')
    check(this.store.plans.size < 9000 && [...this.store.plans.values()].filter(plan => plan.instance_id === instanceId && ['prepared', 'approved'].includes(plan.state) && plan.expires_at > this.clock()).length < 256, 'PLAN_LIMIT', 409)
    await this.capacity(this.config.state_dir, 4 * 1024 * 1024)
    const now = this.clock()
    const id = randomBytes(32).toString('hex')
    const plan = { id, instance_id: instanceId, owner_id: identity.owner_id, key_id: identity.key_id, state: 'prepared', created_at: now, expires_at: now + 1800,
      source_image_id: source.info.Image, target_image_id: artifact.image_id, source_container_id: source.info.Id, source_config_digest: source.digest, source_version: sourceVersion,
      source_sha: artifact.source_sha, tree_sha: artifact.tree_sha, run_id: artifact.run_id, producer_attempt: artifact.producer_attempt,
      artifact_id: artifact.artifact_id, manifest_sha256: artifact.manifest_sha256, version: artifact.version, platform: 'linux/amd64', backup_created: false, target_start_attempted: false, source_mutation_attempted: false, source_restart_disabled: false,
      checks: { registered_instance: true, source_config: true, artifact_metadata: true }, artifact, verified_artifact: verified, registry_digest: digest(instance), source_snapshot: source.info }
    plan.digest = digest({ id, instance_id: instanceId, owner_id: identity.owner_id, key_id: identity.key_id, created_at: now, expires_at: plan.expires_at,
      source_container_id: source.info.Id, source_config_digest: source.digest, source_version: sourceVersion, artifact, verified_artifact: verified, registry_digest: plan.registry_digest, platform: plan.platform })
    await this.store.save(plan)
    return publicPlan(plan)
  }
  async approve(id, suppliedDigest, actor, instanceId) {
    check(!this.mutations.has(id), 'PLAN_BUSY', 409); this.mutations.add(id)
    try {
      const plan = this.get(id); this.authorizePlan(plan, actor, instanceId); this.assertIntegrity(plan)
      check(plan.state === 'prepared', 'PLAN_NOT_PREPARED', 409)
      check(plan.digest === suppliedDigest, 'PLAN_DIGEST_MISMATCH', 409)
      const now = this.clock(); check(now < plan.expires_at, 'PLAN_EXPIRED', 409)
      check(!this.store.frozen.has(plan.instance_id), 'INSTANCE_FROZEN', 409)
      await this.validateActor(this.instance(plan.instance_id), actor.owner_id)
      await this.validateManagementKey(this.instance(plan.instance_id), { key_id: plan.key_id, owner_id: plan.owner_id, scope: 'operations.execute' })
      plan.state = 'approved'; plan.approved_at = now; plan.approval_expires_at = Math.min(now + 300, plan.expires_at); plan.approved_by = actor.owner_id
      await this.store.save(plan)
      return publicPlan(plan)
    } finally { this.mutations.delete(id) }
  }
  async execute(id, suppliedDigest, identity) {
    check(!this.mutations.has(id), 'PLAN_BUSY', 409); this.mutations.add(id)
    let instanceId, locked = false, claimedBusy = false
    try {
      const plan = this.get(id); this.authorizePlan(plan, identity); this.assertIntegrity(plan)
      instanceId = plan.instance_id
      check(plan.state === 'approved', 'PLAN_NOT_APPROVED', 409)
      check(plan.digest === suppliedDigest, 'PLAN_DIGEST_MISMATCH', 409)
      check(this.clock() < plan.expires_at && this.clock() < plan.approval_expires_at, 'APPROVAL_EXPIRED', 409)
      check(!this.busy.has(instanceId) && !this.store.frozen.has(instanceId), 'INSTANCE_LOCKED', 409)
      this.busy.add(instanceId); claimedBusy = true
      await this.validateManagementKey(this.instance(instanceId), { key_id: plan.key_id, owner_id: plan.owner_id, scope: 'operations.execute' })
      await this.store.lock(instanceId); locked = true
      plan.state = 'queued'; await this.store.save(plan)
      const queued = publicPlan(plan)
      const task = this.run(plan).finally(() => { this.tasks.delete(task) })
      this.tasks.add(task)
      return queued
    } catch (error) {
      if (claimedBusy) this.busy.delete(instanceId)
      if (locked) this.store.frozen.add(instanceId)
      throw error
    } finally { this.mutations.delete(id) }
  }
  async run(plan) {
    const instance = this.instance(plan.instance_id)
    let stoppingAttempted = false
    const transition = async state => { plan.state = state; await this.store.save(plan) }
    try {
      this.assertIntegrity(plan)
      check(!this.capacityBusy, 'STORAGE_BUSY', 409)
      this.capacityBusy = plan.id
      await transition('verifying_artifact')
      const verified = await this.github.verify(plan.artifact)
      check(digest(verified) === digest(plan.verified_artifact), 'GITHUB_ARTIFACT_CHANGED')
      await this.capacity(this.config.state_dir, verified.zip_bytes + maxTarBytes + 16384)
      const dataBytes = await this.dataBytes(instance.data_dir)
      await this.capacity(this.config.backups_dir, dataBytes)
      const stage = await this.store.stage(plan.id)
      await this.github.download(plan.artifact, verified, stage)
      Object.assign(plan.checks, await this.verifyBundle(stage, plan.artifact, this.docker))
      // A producer can lose qualification while its archive is downloading or
      // loading. Recheck the complete trusted metadata before any source change.
      const currentQualification = await this.github.verify(plan.artifact)
      check(digest(currentQualification) === digest(plan.verified_artifact), 'GITHUB_ARTIFACT_CHANGED')
      // Revalidate live privileges and the complete config after slow download.
      check(this.clock() < plan.expires_at && this.clock() < plan.approval_expires_at, 'APPROVAL_EXPIRED', 409)
      await this.validateManagementKey(instance, { key_id: plan.key_id, owner_id: plan.owner_id, scope: 'operations.execute' })
      const source = await validateContainer(await this.docker.inspectContainer(plan.source_container_id), instance, this.docker)
      check(source.info.Id === plan.source_container_id && source.digest === plan.source_config_digest, 'SOURCE_CONFIG_CHANGED', 409)
      check(await this.readVersion(instance) === plan.source_version, 'SOURCE_VERSION_CHANGED', 409)
      const targetImage = await this.docker.inspectImage(plan.target_image_id)
      const desiredCandidate = candidateConfig(source.info, plan.target_image_id, targetImage)
      // All remote reads above can consume time. Check the permit again at
      // the final decision boundary, before journaling any source mutation.
      check(this.clock() < plan.expires_at && this.clock() < plan.approval_expires_at, 'APPROVAL_EXPIRED', 409)
      // A retained old container must never revive against the upgraded DB on
      // a Docker/host restart. Record the irreversible boundary before update.
      plan.source_mutation_attempted = true
      plan.source_restart_disabled = null
      await transition('stopping')
      const expectedOld = structuredClone(source.info)
      expectedOld.HostConfig.RestartPolicy = { Name: 'no', MaximumRetryCount: 0 }
      await this.docker.disableRestart(plan.source_container_id)
      const disabled = await this.docker.inspectContainer(plan.source_container_id)
      check(disabled.State?.Running === true && containerDigest(disabled) === containerDigest(expectedOld), 'SOURCE_RESTART_DISABLE_UNCONFIRMED', 503)
      plan.source_restart_disabled = true; plan.checks.old_restart_disabled = true
      stoppingAttempted = true
      await this.docker.stop(plan.source_container_id)
      const stopped = await this.docker.inspectContainer(plan.source_container_id)
      check(stopped.State?.Running === false && containerDigest(stopped) === containerDigest(expectedOld), 'SOURCE_STOP_UNCONFIRMED', 503)
      await transition('backing_up')
      plan.database_before = await this.backup({ action: 'summary', data_dir: instance.data_dir, sqlite_file: instance.sqlite_file })
      const backup = await this.backup({ action: 'backup', data_dir: instance.data_dir, sqlite_file: instance.sqlite_file, destination: path.join(this.config.backups_dir, plan.id), max_bytes: maxBackupBytes })
      check(digest(backup.before) === digest(backup.after) && digest(backup.before) === digest(backup.copied) && digest(plan.database_before) === digest(backup.before), 'BACKUP_CHANGED', 503)
      plan.database_after_backup = backup.after; plan.backup_created = true; plan.checks.consistent_backup = true
      await transition('creating')
      const candidate = await this.docker.create(`myapi-operations-${instance.id}-${plan.id.slice(0, 16)}`, desiredCandidate)
      check(candidate && /^[a-f0-9]{64}$/.test(candidate.Id), 'CANDIDATE_CREATE_FAILED', 503)
      plan.target_container_id = candidate.Id
      const created = await this.docker.inspectContainer(candidate.Id)
      check(created.Image === plan.target_image_id && sameRuntimeConfig(source.info, created, desiredCandidate.Labels), 'CANDIDATE_CONFIG_MISMATCH', 503)
      plan.target_start_attempted = true; await transition('starting')
      await this.docker.start(candidate.Id)
      await transition('verifying')
      const actual = await this.docker.inspectContainer(candidate.Id)
      check(actual.State?.Running === true && actual.Image === plan.target_image_id && sameRuntimeConfig(source.info, actual, desiredCandidate.Labels) && samePublishedPorts(source.info, actual), 'TARGET_CONFIG_MISMATCH', 503)
      plan.checks.runtime_config = true
      Object.assign(plan.checks, await this.verifyHealth(instance, plan.version))
      let observation
      try { observation = await this.backup({ action: 'live_summary', data_dir: instance.data_dir, sqlite_file: instance.sqlite_file }) }
      catch (error) { observation = { state: 'unknown', code: safeCode(error) } }
      if (!['observed', 'unknown', 'corrupt'].includes(observation?.state)) observation = { state: 'unknown', code: 'DATABASE_OBSERVATION_INVALID' }
      plan.database_after_start = observation
      plan.checks.database_readable = observation.state === 'observed' ? true : observation.state === 'corrupt' ? false : null
      check(observation.state !== 'corrupt', 'DATABASE_CORRUPT', 503)
      plan.checks.original_data_mount = true // Original registered mount retained; not an accounting claim.
      await transition('succeeded'); this.store.current.set(plan.instance_id, structuredClone(plan))
      await this.store.unlock(plan.instance_id)
    } catch (error) {
      plan.code = safeCode(error)
      plan.state = plan.source_mutation_attempted || stoppingAttempted || plan.target_start_attempted ? 'recovery_required' : 'failed_before_start'
      if (plan.state === 'recovery_required') this.store.frozen.add(plan.instance_id)
      try {
        await this.store.save(plan)
        if (plan.state === 'failed_before_start') await this.store.unlock(plan.instance_id)
      } catch { this.store.frozen.add(plan.instance_id) }
      // No old image start, candidate removal, database restore or retry here.
    } finally { this.busy.delete(plan.instance_id); if (this.capacityBusy === plan.id) this.capacityBusy = undefined }
  }
  async drain() { await Promise.allSettled([...this.tasks]) }
}
