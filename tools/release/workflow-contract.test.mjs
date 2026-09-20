import assert from 'node:assert/strict'
import test from 'node:test'

import { checkWorkflowContracts } from './workflow-contract.mjs'

const valid = {
  release: `  finalize:\n    needs: [prepare, linux, macos, windows]\n    uses: softprops/action-gh-release\nmyapi-release-linux\nmyapi-release-macos\nmyapi-release-windows`,
  npm: `  publish:\n    environment: npm\n    if: \${{ inputs.confirm == 'PUBLISH' && vars.MYAPI_ENABLE_NPM_PUBLISH == 'true' }}\n    run: npm publish --access public\n    if: \${{ steps.dist-tag.outputs.tag == 'latest' }}`,
  docker: `on:\n  workflow_dispatch:\n  build_single_arch:\n    environment: ghcr-release\n    if: \${{ inputs.confirm == 'PUBLISH' && vars.MYAPI_ENABLE_GHCR_RELEASE == 'true' }}\n  promote_latest:\n    needs: [create_manifests]\n    if: \${{ !contains(inputs.tag, '-') }}`,
}

test('workflow structure rejects extra publishers and missing gates', () => {
  const mutations = [
    { field: 'release', value: `${valid.release}\nuses: softprops/action-gh-release` },
    { field: 'npm', value: valid.npm.replace('environment: npm', '') },
    { field: 'docker', value: valid.docker.replace("vars.MYAPI_ENABLE_GHCR_RELEASE == 'true'", 'true') },
    { field: 'docker', value: valid.docker.replace("if: \${{ !contains(inputs.tag, '-') }}", '') },
    { field: 'docker', value: `${valid.docker}\n  shadow:\n    permissions:\n      packages: write` },
    { field: 'release', value: `${valid.release}\n  alias: &release` },
    { field: 'npm', value: valid.npm.replace('environment: npm', '# environment: npm') },
    { field: 'docker', value: `${valid.docker}\n  'quoted-job':\n    permissions:\n      packages: write\n    run: docker push example` },
    { field: 'docker', value: `${valid.docker}\n  alias-job:\n    <<: *release` },
    { field: 'npm', value: `${valid.npm}\n  oidc-job:\n    permissions:\n      id-token: write\n    run: pnpm publish` },
    { field: 'docker', value: `${valid.docker}\n  push-job:\n    permissions:\n      packages: write\n    run: docker buildx build --push .` },
    { field: 'release', value: `${valid.release}\n  api-job:\n    permissions:\n      contents: write\n    run: curl -X POST /releases` },
  ]
  for (const mutation of mutations) {
    const input = { ...valid, [mutation.field]: mutation.value }
    assert.equal(checkWorkflowContracts(input).some((contract) => !contract.ok), true)
  }
})

test('workflow structure handles a multiline gate and rejects its commented replacement', () => {
  const multiline = {
    ...valid,
    docker: valid.docker.replace(
      "if: \${{ inputs.confirm == 'PUBLISH' && vars.MYAPI_ENABLE_GHCR_RELEASE == 'true' }}",
      "if:\n      \${{ inputs.confirm == 'PUBLISH' && vars.MYAPI_ENABLE_GHCR_RELEASE == 'true' }}"
    ),
  }
  const ghcrGate = (input) =>
    checkWorkflowContracts(input).find(
      (contract) => contract.name === 'GHCR publisher has explicit environment and release gate'
    )?.ok
  assert.equal(ghcrGate(multiline), true)
  multiline.docker = multiline.docker.replace(
    "if:\n      \${{ inputs.confirm == 'PUBLISH' && vars.MYAPI_ENABLE_GHCR_RELEASE == 'true' }}",
    "# if: \${{ inputs.confirm == 'PUBLISH' && vars.MYAPI_ENABLE_GHCR_RELEASE == 'true' }}"
  )
  assert.equal(ghcrGate(multiline), false)
})
