#!/bin/bash
# 整理工作区：让移动硬盘上的主检出 /Volumes/Work/Prive/MyAPI 直接停在 dev 分支，
# 并移除两个嵌套的 Claude 工作区（dev-work、fetch-latest-repo-2c038a）。
#
# 用法（在终端里，不要在 Claude 会话里）：
#   bash /Volumes/Work/Prive/MyAPI/.claude/worktrees/dev-work/docs/evidence/redesign-20261024/handoff/consolidate-workspace.sh
# 或先把本文件复制到别处再运行（脚本会删除 dev-work 目录，运行中的脚本最好不在其中）：
#   cp /Volumes/Work/Prive/MyAPI/.claude/worktrees/dev-work/docs/evidence/redesign-20261024/handoff/consolidate-workspace.sh ~/consolidate-workspace.sh
#   bash ~/consolidate-workspace.sh
#
# 脚本只做本地整理：不推送、不删除任何分支（除一个已合并的临时分支）、不删除 .git。
# 任何检查不通过就中止，不做修改。macOS 自带 bash 3.2 可运行。

set -eu

ROOT="/Volumes/Work/Prive/MyAPI"
DEVWT="$ROOT/.claude/worktrees/dev-work"
OLDWT="$ROOT/.claude/worktrees/fetch-latest-repo-2c038a"

# 移动硬盘是 exFAT：每条 git 都忽略权限位，并过滤 ._*.idx 的报错噪音。
gq() { git -c core.fileMode=false "$@" 2>/dev/null; }
gv() { git -c core.fileMode=false "$@" 2>&1 | grep -v 'non-monotonic' || true; }

fail() { echo "中止：$1" >&2; exit 1; }

echo "== 1. 检查主检出 =="
cd "$ROOT" || fail "找不到 $ROOT（移动硬盘没有挂载？）"
[ "$(gq rev-parse --show-toplevel)" = "$ROOT" ] || fail "$ROOT 不是 git 仓库的根目录"
echo "当前分支：$(gq rev-parse --abbrev-ref HEAD)"

CHANGED="$(gq status --porcelain --untracked-files=no)"
if [ -n "$CHANGED" ]; then
  echo "$CHANGED" | head -20
  fail "主检出里有已跟踪文件的未提交修改（上面列出前 20 行）。先自行提交或备份，再运行本脚本。"
fi
echo "主检出没有已跟踪文件的未提交修改。"

echo "== 2. 取最新的 origin =="
gv fetch origin
gq rev-parse --verify origin/dev >/dev/null || fail "找不到 origin/dev"
echo "origin/dev = $(gq rev-parse --short origin/dev)"

echo "== 3. 检查 dev-work 工作区的内容都已推送 =="
if [ -d "$DEVWT" ]; then
  DEVWT_HEAD="$(git -c core.fileMode=false -C "$DEVWT" rev-parse HEAD 2>/dev/null)"
  gq merge-base --is-ancestor "$DEVWT_HEAD" origin/dev || fail "dev-work 里有还没推送到 origin/dev 的提交（$DEVWT_HEAD）。先在 dev-work 里推送。"
  DEVWT_DIRTY="$(git -c core.fileMode=false -C "$DEVWT" status --porcelain --untracked-files=no 2>/dev/null)"
  [ -z "$DEVWT_DIRTY" ] || { echo "$DEVWT_DIRTY" | head -20; fail "dev-work 里有已跟踪文件的未提交修改。"; }
  echo "dev-work 的提交都在 origin/dev 上，没有未提交的修改。"
else
  echo "dev-work 不存在，跳过。"
fi

echo "== 4. 检查旧的 fetch-latest 工作区 =="
REMOVE_OLD=no
if [ -d "$OLDWT" ]; then
  OLD_HEAD="$(git -c core.fileMode=false -C "$OLDWT" rev-parse HEAD 2>/dev/null)"
  if gq merge-base --is-ancestor "$OLD_HEAD" origin/main; then
    REMOVE_OLD=yes
    echo "fetch-latest-repo-2c038a 的提交已在 origin/main 上，可以移除。"
  else
    echo "fetch-latest-repo-2c038a 有不在 origin/main 上的提交，保留不动。"
  fi
else
  echo "fetch-latest-repo-2c038a 不存在，跳过。"
fi

echo
echo "将要执行："
[ -d "$DEVWT" ] && echo "  1) 移除工作区 $DEVWT"
[ "$REMOVE_OLD" = yes ] && echo "  2) 移除工作区 $OLDWT，并删除已合并的临时分支 feature/fetch-latest-repo-2c038a"
echo "  3) 在 $ROOT 里切到 dev，并快进到 origin/dev"
echo "  4) 设置本仓库 core.fileMode=false（以后 git status 不再被权限位刷屏）"
echo "不会推送，不会删除 .git，不会删除其他分支。"
printf "输入 yes 继续："
read -r ANSWER
[ "$ANSWER" = "yes" ] || fail "已取消，没有做任何修改。"

echo "== 5. 执行 =="
cd "$ROOT"
if [ -d "$DEVWT" ]; then
  gv worktree remove --force "$DEVWT"
fi
if [ "$REMOVE_OLD" = yes ]; then
  gv worktree remove --force "$OLDWT"
  gv branch -d feature/fetch-latest-repo-2c038a
fi
gv worktree prune

# 切到 dev；若本地还没有 dev 分支就从 origin/dev 建立并跟踪。
if gq rev-parse --verify dev >/dev/null; then
  gv checkout dev
else
  gv checkout -b dev --track origin/dev
fi
[ "$(gq rev-parse --abbrev-ref HEAD)" = "dev" ] || fail "没有切到 dev，请把上面的输出发给维护者处理。"
gv pull --ff-only origin dev
git config core.fileMode false

echo "== 6. 结果 =="
echo "分支：$(gq rev-parse --abbrev-ref HEAD)"
echo "提交：$(gq log --oneline -1)"
echo "origin/dev：$(gq log --oneline -1 origin/dev)"
echo "工作区列表："
gv worktree list
echo
echo "完成。以后项目就在 $ROOT，直接在这里工作（分支 dev）。"
echo "提醒：不要删除 $ROOT/.git，其中有几条本地独有的历史分支提交（远端已不存在）。"
