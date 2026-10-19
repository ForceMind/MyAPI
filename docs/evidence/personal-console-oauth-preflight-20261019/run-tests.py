import json
import os
import pathlib
import subprocess
import time

commands = json.loads(pathlib.Path('/tmp/myapi-oauth-preflight-final-commands.json').read_text())
results = []
for command in commands:
    group = command['group']
    stdout = pathlib.Path('/tmp/myapi-oauth-preflight-' + group + '.stdout')
    env = os.environ.copy()
    env.update(command['env'])
    start = time.monotonic()
    with stdout.open('wb') as output:
        try:
            run = subprocess.run(command['argv'], cwd=command['cwd'], env=env,
                                 stdout=output, stderr=subprocess.STDOUT, timeout=180)
            code = run.returncode
        except subprocess.TimeoutExpired:
            code = 124
    pathlib.Path('/tmp/myapi-oauth-preflight-' + group + '.exit').write_text(str(code) + '\n')
    result = {'group': group, 'exit_code': code, 'duration_seconds': round(time.monotonic() - start, 3),
              'stdout': str(stdout), 'selected_top_level': len(command['selected_top_level'])}
    results.append(result)
    pathlib.Path('/tmp/myapi-oauth-preflight-final-results.json').write_text(json.dumps(results, indent=2) + '\n')
    print(json.dumps(result), flush=True)
    if code == 124:
        break
raise SystemExit(0 if all(r['exit_code'] == 0 for r in results) else 1)
