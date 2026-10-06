/*
 * Bounded, dependency-free checks for the release workflow structure.
 * Copyright (C) 2026 ForceMind
 */

function jobBlock(source, name) {
  const start = source.indexOf(`  ${name}:\n`)
  if (start < 0) return ''
  const rest = source.slice(start + name.length + 4)
  const end = rest.search(/^  [A-Za-z][\w-]*:\n/m)
  return end < 0 ? rest : rest.slice(0, end)
}

function normalizeWorkflow(source) {
  const withoutComments = source
    .split('\n')
    .map((line) => line.replace(/(^|\s)#.*$/, '$1'))
    .join('\n')
  return withoutComments.replace(/(\sif:)\s*\n\s+([^\n]+)/g, '$1 $2')
}

function count(source, pattern) {
  return [...source.matchAll(pattern)].length
}

function jobBlocks(source) {
  const starts = [...source.matchAll(/^  ([A-Za-z][\w-]*):\n/gm)]
  return starts.map((match, index) => ({
    name: match[1],
    body: source.slice(match.index, starts[index + 1]?.index),
  }))
}

function isPublisher(body) {
  return /softprops\/action-gh-release|\bgh\s+release\s+create\b|\bgh\s+api\b[^\n]*\b(POST|PATCH|PUT)\b[^\n]*(releases|packages)|\bcurl\b[^\n]*\b(POST|PATCH|PUT)\b[^\n]*(releases|packages)|\bnpm\s+publish\b|\bpnpm\s+publish\b|\byarn\s+(npm\s+)?publish\b|docker\/build-push-action|\bdocker\s+push\b|\bdocker\s+buildx\s+build\b[^\n]*--push|\boras\s+push\b|\bdocker\s+buildx\s+imagetools\s+create\b|\bcosign\s+sign\b/.test(body)
}

export function checkWorkflowContracts({ release, npm, docker }) {
  release = normalizeWorkflow(release)
  npm = normalizeWorkflow(npm)
  docker = normalizeWorkflow(docker)
  const releaseFinalize = jobBlock(release, 'finalize')
  const dockerBuild = jobBlock(docker, 'build_single_arch')
  const dockerPromote = jobBlock(docker, 'promote_latest')
  const npmPublish = jobBlock(npm, 'publish')
  const allJobs = [
    ...jobBlocks(release).map((job) => ({ ...job, workflow: 'release' })),
    ...jobBlocks(npm).map((job) => ({ ...job, workflow: 'npm' })),
    ...jobBlocks(docker).map((job) => ({ ...job, workflow: 'docker' })),
  ]
  const publisherJobs = allJobs.filter((job) => isPublisher(job.body))
  const allowedWriters = {
    release: new Set(['finalize']),
    npm: new Set(['publish']),
    docker: new Set(['build_single_arch', 'create_manifests', 'promote_latest']),
  }
  return [
    {
      name: 'one GitHub Release publisher finalizes all platform artifacts',
      ok:
        count(release, /softprops\/action-gh-release/g) === 1 &&
        releaseFinalize.includes('needs: [prepare, linux, macos, windows]') &&
        ['myapi-release-linux', 'myapi-release-macos', 'myapi-release-windows'].every((name) => release.includes(name)),
    },
    {
      name: 'npm publisher has explicit environment and release gate',
      ok:
        npmPublish.includes('environment: npm') &&
        npmPublish.includes("inputs.confirm == 'PUBLISH'") &&
        npmPublish.includes("vars.MYAPI_ENABLE_NPM_PUBLISH == 'true'") &&
        count(npm, /npm publish --access public/g) === 1,
    },
    {
      name: 'GHCR publisher has explicit environment and release gate',
      ok:
        !/^\s{2}push:/m.test(docker) &&
        dockerBuild.includes('environment: ghcr-release') &&
        dockerBuild.includes("inputs.confirm == 'PUBLISH'") &&
        dockerBuild.includes("vars.MYAPI_ENABLE_GHCR_RELEASE == 'true'") &&
        dockerPromote.includes('needs: [create_manifests]'),
    },
    {
      name: 'prerelease never updates stable latest unconditionally',
      ok:
        releaseFinalize.includes("make_latest: ${{ needs.prepare.outputs.prerelease == 'true' && 'false' || 'true' }}") &&
        npmPublish.includes("if: ${{ steps.dist-tag.outputs.tag == 'latest' }}") &&
        dockerPromote.includes("!contains(inputs.tag, '-')"),
    },
    {
      name: 'all publisher jobs have explicit environment and publish gate',
      ok: publisherJobs.length > 0 &&
        publisherJobs.every(
          (job) =>
            /environment:\s*(github-release|npm|ghcr-release)/.test(job.body) &&
            /if:\s*\$\{\{[^\n]*inputs\.confirm\s*==\s*'PUBLISH'/.test(job.body),
        ),
    },
    {
      name: 'workflow parser rejects anchors and unclassified package writers',
      ok:
        !/^  ['"][^'"]+['"]:/m.test(`${release}\n${npm}\n${docker}`) &&
        !/(^|\s)[&*][A-Za-z][\w-]*|<<:\s*\*/m.test(`${release}\n${npm}\n${docker}`) &&
        allJobs.every((job) => {
          const writer =
            /contents:\s*write|packages:\s*write|id-token:\s*write/.test(job.body) ||
            /GITHUB_TOKEN|GH_TOKEN|NPM_TOKEN|\bPAT\b/.test(job.body)
          return !writer || (allowedWriters[job.workflow].has(job.name) && isPublisher(job.body))
        }),
    },
  ]
}
