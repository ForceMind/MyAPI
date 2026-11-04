import type { InternalAxiosRequestConfig } from 'axios'

import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import type { OperationsPlan } from '../operations-api'

export const now = 1_900_000_000
export const plan: OperationsPlan = {
  id: 'a'.repeat(32), digest: 'b'.repeat(64), instance_id: 'gateway-primary', owner_id: 1, key_id: 17,
  state: 'prepared', created_at: now - 10, expires_at: now + 600,
  source_image_id: `sha256:${'1'.repeat(64)}`, target_image_id: `sha256:${'2'.repeat(64)}`,
  source_container_id: 'container-old', source_config_digest: '3'.repeat(64),
  source_sha: '4'.repeat(40), tree_sha: '5'.repeat(40), run_id: '41655135543', producer_attempt: '1', artifact_id: '9007199254740993',
  manifest_sha256: '6'.repeat(64), source_version: '0.2.0-beta.32', version: '0.2.0-beta.33', platform: 'linux/amd64', backup_created: false, target_start_attempted: false,
}

let loginNumber = 0
export function operationsSignIn(role: number = ROLE.SUPER_ADMIN, sid = `root-session-${++loginNumber}`, userID = 1) {
  useAuthStore.getState().auth.setBundle({
    access_token: `session-${sid}`, token_type: 'Bearer', access_expires_at: 4_000_000_000,
    user: { id: userID, username: 'owner', role },
    session: { sid, current: true, login_method: 'password', ip: '127.0.0.1', user_agent: 'synthetic', created_at: 1, last_active_at: 1, expires_at: 4_000_000_000 },
  })
}

export function operationResponse(config: InternalAxiosRequestConfig, data: unknown) {
  return { config, data: { success: true, data }, status: 200, statusText: 'OK', headers: {} }
}
