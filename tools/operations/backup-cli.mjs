import { loadBackupConfig } from './config.mjs'
import { claimDaemon } from './daemon.mjs'
import { BackupEngine } from './backup-engine.mjs'
import { check } from './safety.mjs'

export async function backupCommand(command, args) {
  check(['backup', 'restore'].includes(command), 'INVALID_BACKUP_COMMAND')
  const value = flag => args[args.indexOf(flag) + 1]
  const config = await loadBackupConfig(value('--config'))
  const guard = await claimDaemon(config)
  guard.onLost(() => { process.stderr.write('OPERATIONS_LOCK_LOST\n'); process.exit(1) })
  try {
    const engine = await new BackupEngine(config).init()
    let result
    if (args.includes('--confirm')) {
      const confirmation = /^([a-f0-9]{64}):([a-f0-9]{64})$/.exec(value('--confirm'))
      check(confirmation, 'PLAN_CONFIRMATION_INVALID')
      result = await engine.execute(confirmation[1], confirmation[2], command, value('--instance'),
        command === 'restore' ? { backupId: value('--backup'), target: value('--target-dir') } : {})
    } else {
      result = command === 'backup' ? await engine.prepareBackup(value('--instance'))
        : await engine.prepareRestore(value('--instance'), value('--backup'), value('--target-dir'))
    }
    if (args.includes('--json')) console.log(JSON.stringify(result))
    else {
      const states = { prepared: '等待确认', succeeded: '校验通过', failed_before_start: '尚未执行数据操作', recovery_required: '结果未确认，已暂停后续操作' }
      console.log(`${command === 'backup' ? '备份' : '隔离恢复'}：${states[result.state] ?? '状态未知'}`)
      console.log(`实例：${result.instance_id}\n目标目录：${result.destination}`)
      if (result.state === 'prepared') {
        if (result.original_restart_policy) {
          const policies = { '': '不自动重启', no: '不自动重启', always: '自动重启', 'unless-stopped': '自动重启，手动停止后保持停止', 'on-failure': '失败时重启' }
          console.log(`原自动重启策略：${policies[result.original_restart_policy.Name] ?? '未知，请先核对'}`)
        }
        console.log(command === 'backup'
          ? '确认后将停止此实例并关闭自动重启。备份后保持停止；本命令不会自动恢复服务。'
          : '只复制到这个尚不存在的新目录。原目录不会被覆盖，也不会启动恢复实例。')
        console.log('备份保留策略：全部保留，不自动删除。请先安排没有其他写入者的维护窗口。')
        console.log(`核对以上内容后，在相同命令末尾追加：--confirm ${result.id}:${result.digest}`)
        console.log('当前只保存了计划，尚未停止、备份或恢复数据。计划有效期为 10 分钟。')
      } else if (result.state === 'succeeded') {
        console.log(command === 'backup' ? '备份及校验通过。原实例保持停止，自动重启已关闭。' : '新目录的恢复及校验通过。原实例和目录保持不变。')
      } else {
        console.log(`未成功：${result.code ?? 'OPERATIONS_FAILED'}。请保留现场，检查状态后人工处理；不要直接重试或回灌旧数据。`)
      }
    }
    if (!['prepared', 'succeeded'].includes(result.state)) process.exitCode = 1
    return result
  } finally { await guard.release() }
}
