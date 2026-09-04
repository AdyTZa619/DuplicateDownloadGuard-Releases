from pathlib import Path


def replace_once(path, old, new):
    p = Path(path)
    s = p.read_text(encoding='utf-8')
    if old not in s:
        raise SystemExit(f'pattern missing in {path}: {old[:160]!r}')
    if s.count(old) != 1:
        raise SystemExit(f'pattern count {s.count(old)} in {path}: {old[:160]!r}')
    p.write_text(s.replace(old, new, 1), encoding='utf-8')

replace_once('download_flow_v854.go',
'''\tif live != nil {\n\t\treturn *live, nil\n\t}\n''',
'''\tif live != nil && legacyQueueJobMatchesResultV854(job, *live) {\n\t\treturn *live, nil\n\t}\n''')

# A persistent snapshot remains authoritative. Never replace it after Guard
# merely because a new scan happens to reuse the same numeric ResultID.
replace_once('v8_extra.go',
'''\t\tif refreshed, exists := a.resultByID(rid); exists {\n\t\t\tres = refreshed\n\t\t}\n''',
'''\t\t// Keep the persistent source snapshot authoritative after Guard.\n''')

replace_once('v8_extra.go',
'''\tfor _, job := range q.Jobs {\n\t\tdecision, exists := decisions[job.ResultID]\n\t\tif !exists || job.Status == "completed" || job.Status == "cancelled" || job.Status == "blocked" {\n''',
'''\tfor _, job := range q.Jobs {\n\t\tdecision, exists := decisionForQueueJobV854(job, selectedRows, decisions)\n\t\tif !exists || job.Status == "completed" || job.Status == "cancelled" || job.Status == "blocked" {\n''')

replace_once('v8_extra.go',
'''\t\tfor _, j := range q.Jobs {\n\t\t\tif j.ResultID == res.ID && (j.Status == "queued" || j.Status == "running" || j.Status == "paused") {\n\t\t\t\tdup = true\n\t\t\t\tbreak\n\t\t\t}\n\t\t}\n''',
'''\t\tfor _, j := range q.Jobs {\n\t\t\tif queueJobMatchesResultV854(j, res) && (j.Status == "queued" || j.Status == "running" || j.Status == "paused") {\n\t\t\t\tdup = true\n\t\t\t\tbreak\n\t\t\t}\n\t\t}\n''')

Path('tools/apply_download_identity_v854.py').unlink(missing_ok=True)
Path('.github/workflows/apply-download-identity-v854.yml').unlink(missing_ok=True)
print('stable queue identity patch applied')
