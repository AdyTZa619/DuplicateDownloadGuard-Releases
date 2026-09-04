from pathlib import Path

p = Path('download_core_v855.go')
s = p.read_text(encoding='utf-8')
sig = 'func (a *App) markDownloadedResultV855(res Result, path string) {'
start = s.find(sig)
if start < 0:
    raise SystemExit('markDownloadedResultV855 not found')
brace = s.find('{', start)
depth = 0
end = None
for i in range(brace, len(s)):
    if s[i] == '{':
        depth += 1
    elif s[i] == '}':
        depth -= 1
        if depth == 0:
            end = i + 1
            break
if end is None:
    raise SystemExit('could not locate function end')
replacement = '''func (a *App) markDownloadedResultV855(res Result, path string) {\n\t// v8.6 no longer trusts/reconciles only the transient ResultID. The final\n\t// verified artifact is inserted into the local index, all rows with the\n\t// same stable source or exact remote hash are updated, and the durable\n\t// remote→local relationship is recorded in Content Graph.\n\ta.postDownloadReconcileV860(res, path)\n}'''
if 'a.postDownloadReconcileV860(res, path)' in s[start:end]:
    print('post-download reconciliation already integrated')
else:
    s = s[:start] + replacement + s[end:]
    p.write_text(s, encoding='utf-8', newline='\n')
    print('markDownloadedResultV855 routed through v8.6 reconciliation')
