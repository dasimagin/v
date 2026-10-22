// Go functions are bound into the page as window.vStart() and so on. In a plain
// browser (npm run dev) there is no Go, so the screens get sample data.
const dark = { bg: '#21252b', panel: '#282c34', fg: '#d7dae0', muted: '#6b7380', accent: '#61afef', accent2: '#a988e2', dark: true, syntax: { keyword: '#c678dd', func: '#61afef', comment: '#5c6370', string: '#98c379' } }
const light = { bg: '#f3f4f6', panel: '#fafafa', fg: '#1f2430', muted: '#8a909c', accent: '#3f729b', accent2: '#7a55b8', dark: false }

const sample = {
  workdir: '~/sandbox/v',
  tagline: 'Vibing time...',
  palette: dark,
  animate: true,
  git: { branch: 'ui-experiment', dirty: 3 },
  commands: [
    { name: 'pr', args: '[URL | number]', desc: "Read this branch's PR", supported: true },
    { name: 'log', aliases: ['show'], desc: 'Browse the history of all branches, with their tags', supported: true },
    { name: 'workdir', args: '[directory]', desc: 'Find and choose the working directory', supported: true },
    { name: 'branch', aliases: ['switch'], args: '[name]', desc: 'Find and switch Git branches', supported: true },
    { name: 'open', args: '[path]', desc: 'Find and open a file', supported: true },
    { name: 'config', desc: 'Settings', supported: true },
    { name: 'resume', desc: 'Continue a past session in this directory', supported: true },
    { name: 'reset', desc: 'Start a new session', supported: true },
    { name: 'quit', aliases: ['q'], desc: 'Quit v', supported: true },
  ],
}

let options = [
  { key: 'theme', label: 'Color theme', desc: 'The colors of the window', current: 'dark',
    choices: [{ id: 'dark', name: 'One Dark Pro' }, ...['Dracula', 'Nord', 'Gruvbox', 'Solarized Dark', 'Tokyo Night', 'Catppuccin', 'Monokai', 'Everforest'].map((n) => ({ id: n, name: n })), { id: 'light', name: 'Light' }] },
  { key: 'model', label: 'Model', desc: 'The model the agent runs on', current: 'opus',
    choices: [{ id: 'fable', name: 'Fable 5.1' }, { id: 'opus', name: 'Opus 5.5' }, { id: 'sonnet', name: 'Sonnet 5.5' }] },
  { key: 'effort', label: 'Effort', desc: 'How hard the model thinks', current: 'high',
    choices: ['low', 'medium', 'high', 'xhigh', 'max'].map((id) => ({ id, name: id })) },
  { key: 'detail', label: 'Chat detail', desc: "How much of the agent's work the chat shows. The slider above the input changes it too", current: 'commands',
    choices: ['answer', 'edit', 'commands', 'thinking'].map((id) => ({ id, name: id })) },
  { key: 'diff_before', label: 'Diff context before', desc: 'Unchanged lines shown above each change, in every diff', current: '3',
    choices: [0, 1, 2, 3, 5, 8, 10, 15, 20, 30, 50].map((n) => ({ id: String(n), name: n === 1 ? '1 line' : n + ' lines' })) },
  { key: 'diff_after', label: 'Diff context after', desc: 'Unchanged lines shown below each change, in every diff', current: '3',
    choices: [0, 1, 2, 3, 5, 8, 10, 15, 20, 30, 50].map((n) => ({ id: String(n), name: n === 1 ? '1 line' : n + ' lines' })) },
  { key: 'mode', label: 'Permissions', desc: 'Manual asks before every edit and command; Auto goes on its own', current: 'manual',
    choices: [{ id: 'manual', name: 'manual' }, { id: 'auto', name: 'auto' }] },
]

export const start = () => (window.vStart ? window.vStart() : Promise.resolve(sample))
export const tagline = () => (window.vTagline ? window.vTagline() : Promise.resolve('Works on my machine...'))
export const getOptions = () => (window.vOptions ? window.vOptions() : Promise.resolve(options))
export const setOption = (key, id) => {
  if (window.vSetOption) return window.vSetOption(key, id)
  options = options.map((o) => (o.key === key ? { ...o, current: id } : o))
  return Promise.resolve(options)
}
export const palette = (id) => (window.vPalette ? window.vPalette(id) : Promise.resolve(id === 'light' ? light : dark))
export const git = () => (window.vGit ? window.vGit() : Promise.resolve(sample.git))
export const branches = () => (window.vBranches ? window.vBranches() : Promise.resolve(['ui-experiment', 'master']))
export const switchBranch = (name) => (window.vSwitch ? window.vSwitch(name) : Promise.resolve({ branch: name, dirty: 0 }))

const mockLines = [[{ t: 'package', c: 'keyword' }, { t: ' main' }], [], [{ t: '// sample', c: 'comment' }], [{ t: 'func', c: 'keyword' }, { t: ' ' }, { t: 'main', c: 'func' }, { t: '() {}' }]]
export const list = (kind, query) =>
  window.vList
    ? window.vList(kind, query)
    : Promise.resolve({
        dir: '~/sandbox/v',
        crumbs: [{ name: '~', query: '~/' }, { name: 'sandbox', query: '~/sandbox/' }, { name: 'v', query: '' }],
        up: '',
        err: '',
        items: kind === 'log'
          ? [
              { value: 'a1', label: 'Merge the graphical UI', meta: '9d3e2f1 · Denis · 1 hour ago', refs: [{ name: 'master', kind: 'head' }, { name: 'origin/master', kind: 'remote' }, { name: 'v0.3', kind: 'tag' }]},
              { value: 'a2', label: 'Move start-screen jokes to taglines.txt', meta: '07596a8 · Denis · 2 hours ago', refs: []},
              { value: 'a3', label: 'Add the PR screen', meta: '3c1d9aa · Denis · 3 hours ago', refs: [{ name: 'ui-experiment', kind: 'branch' }]},
              { value: 'a4', label: 'Start the graphical UI', meta: '1569d5e · Denis · yesterday', refs: [{ name: 'v0.2', kind: 'tag' }]},
            ]
          : kind === 'branch'
          ? [{ value: 'master', label: 'master', tag: 'current', sub: 'Move start-screen jokes', when: '2 hours ago' }, { value: 'origin/feature', label: 'origin/feature', tag: 'remote', sub: 'Add the UI', when: 'yesterday' }]
          : [{ value: '/x/ui', label: 'ui/', dir: true, query: 'ui/' }, { value: '/x/README.md', label: 'README.md' }, { value: '/x/logo.svg', label: 'logo.svg' }, { value: '/x/shot.png', label: 'shot.png' }, { value: '/x/main.go', label: 'main.go' }, { value: '/x/package.json', label: 'package.json' }].filter((i) => i.label.includes(query.split('/').pop())),
      })
const mockMd = [
  '# Vibing', '', 'A **graphical** front end for `v`, with *tabs*, tables and [a link](https://example.com).', '', '## Features', '', '- Chat with the agent', '- Shell mode', '  - runs in a pty', '1. First', '2. Second', '',
  '> A quote that wraps over a few lines of the preview pane.', '', '| Mode | Key |', '| --- | --- |', '| agent | `*` |', '| shell | `$` |', '', '```go', 'func main() {}', '```', '', '---',
].map((t) => [{ t }])
const mockJson = JSON.stringify({ name: 'vibing-ui', version: '0.1.0', private: true, scripts: { dev: 'vite', build: 'vite build' }, dependencies: { svelte: '^5.0.0', yaml: '^2.8.0' }, files: ['dist', 'README.md'], nested: { deep: { deeper: { deepest: [1, 2, 3, null, true] } } } }, null, 2)
  .split('\n')
  .map((t) => [{ t }])
const mockRows = [
  { k: 'hunk', old: 0, new: 0, t: '@@ -1,3 +1,4 @@ func main() {' },
  { k: 'ctx', old: 1, new: 1, t: 'args, gui := takeFlag(os.Args[1:], "--ui")' },
  { k: 'del', old: 2, new: 0, t: 'run(args)' },
  { k: 'add', old: 0, new: 2, t: 'if gui { runUI(args); return }' },
]
const mockDetail = (kind) =>
  kind === 'log'
    ? { kind: 'commit', hash: 'a1b2c3', short: '07596a8', refs: [{ name: 'master', kind: 'head' }, { name: 'v0.3', kind: 'tag' }], author: 'Denis Simagin', date: '2026-10-21 12:30', rel: '2 hours ago', subject: 'Move start-screen jokes to taglines.txt', body: '~/.v/taglines.txt overrides it.', moreFiles: 0, err: '',
        files: [{ path: 'main.go', add: 12, del: 3, rows: mockRows, more: 0 }, { path: 'docs/taglines.txt', add: 30, del: 0, rows: mockRows, more: 0 }] }
    : { kind: 'branch', ref: 'feature/ui', current: 'master', isCurrent: false, ahead: 4, behind: 1, add: 420, del: 18, err: '',
        files: [{ path: 'ui/src/App.svelte', add: 300, del: 10 }, { path: 'gui.go', add: 120, del: 8 }],
        commits: [{ short: '07596a8', subject: 'Move start-screen jokes', author: 'Denis', rel: '2 hours ago', refs: [{ name: 'feature/ui', kind: 'head' }, { name: 'v0.3', kind: 'tag' }] }, { short: '1569d5e', subject: 'Do not listen to the agent again', author: 'Denis', rel: 'yesterday', refs: [] }, { short: '826a32d', subject: 'Short README for people', author: 'Denis', rel: '3 days ago', refs: [{ name: 'origin/master', kind: 'remote' }] }] }
export const preview = (kind, value) =>
  window.vPreview
    ? window.vPreview(kind, value)
    : Promise.resolve(kind === 'log' || kind === 'branch' ? mockDetail(kind) : value.endsWith('.md') ? mockMd : value.endsWith('.json') ? mockJson : mockLines)
export const file = (path) => (window.vFile ? window.vFile(path) : Promise.resolve({ path, abs: path, lang: 'Go', lines: mockLines, err: '', text: mockLines.map((l) => l.map((s) => s.t).join('')).join('\n'), crlf: false, mtime: 1, edit: true }))
export const imageInfo = (path) =>
  window.vImage
    ? window.vImage(path)
    : Promise.resolve({
        path, name: path.split('/').pop(), mime: path.endsWith('.svg') ? 'image/svg+xml' : 'image/png', size: 48213, width: 192, height: 192, modified: '3 days ago',
        data: 'data:image/svg+xml;base64,' + btoa('<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 192 192"><rect width="192" height="192" rx="40" fill="#61afef"/><path d="M40 50h28l28 70 28-70h28L110 150H82z" fill="#fff"/></svg>'),
      })
export const saveFile = (path, text, crlf, mtime) => (window.vSaveFile ? window.vSaveFile(path, text, crlf, mtime) : Promise.resolve(mtime + 1))
export const highlightText = (path, text) => (window.vHighlight ? window.vHighlight(path, text) : Promise.resolve(text.split('\n').map((t) => [{ t }])))
export const setWorkdir = (path) => (window.vSetWorkdir ? window.vSetWorkdir(path) : Promise.resolve({ ...sample, workdir: path }))

// The agent: events come back through window.vEvent (see onAgent).
export const onAgent = (fn) => {
  window.vEvent = fn
}
export const send = (text, context = '') => {
  if (window.vSend) return window.vSend(text, context)
  // In a plain browser there is no agent: play a short reply.
  const e = (x, ms) => setTimeout(() => window.vEvent?.(x), ms)
  e({ kind: 'text-start' }, 200)
  e({ kind: 'text', text: 'This is a sample reply with `inline code`:\n\n```go\nfunc main() {}\n```\n' }, 400)
  e({ kind: 'thinking-start' }, 100)
  e({ kind: 'thinking', text: 'The user wants the jokes in a file.\nI will edit main.go.' }, 150)
  e({ kind: 'tool', text: 'Bash go test ./...', tool: 'Bash', toolId: 't0', detail: 'go test ./...' }, 500)
  e({ kind: 'tool-result', toolId: 't0', text: 'ok  \tgithub.com/dasimagin/v\t1.2s\n' }, 560)
  e({ kind: 'tool', text: 'Edit main.go', tool: 'Edit', toolId: 't1', detail: '- func main() {}\n+ func main() {\n+ \trunUI()\n+ }' }, 600)
  e({ kind: 'ask', id: 'r1', text: 'Edit main.go', tool: 'Edit' }, 700)
  return Promise.resolve()
}
export const answer = (id, allow) => (window.vAnswer ? window.vAnswer(id, allow) : (setTimeout(() => window.vEvent?.({ kind: 'done', secs: 1.2, turns: 1 }), 300), Promise.resolve()))
export const interrupt = () => (window.vInterrupt ? window.vInterrupt() : Promise.resolve())
export const reset = () => (window.vReset ? window.vReset() : Promise.resolve())
export const resume = (path) =>
  window.vResume
    ? window.vResume(path)
    : Promise.resolve([
        { kind: 'user', text: 'Check out the PR branch on /pr <url>' },
        { kind: 'tool', text: 'Read workspace.go' },
        { kind: 'agent', text: 'Done: `/pr <url>` now checks out the branch.' },
      ])

const mockPR = {
  number: 42, title: 'Add the graphical UI', url: 'https://github.com/dasimagin/v/pull/42', author: 'dasimagin', state: 'open',
  base: 'master', head: 'ui-experiment', draft: false, labels: ['ui'], viewer: 'dasimagin', pending: 1, summary: '', immediate: false, talk: 1,
  files: [
    { path: 'gui.go', adds: 90, dels: 0, threads: 1 },
    { path: 'ui/src/App.svelte', adds: 410, dels: 12, threads: 0 },
    { path: 'ui/src/Chat.svelte', adds: 120, dels: 4, threads: 1 },
    { path: 'ui/src/style.css', adds: 20, dels: 2, threads: 0 },
    { path: 'README.md', adds: 6, dels: 1, threads: 0 },
  ],
  threads: [
    { id: 't1', path: 'gui.go', line: 2, resolved: false, outdated: false, author: 'reviewer', preview: 'Blank line needed here?', count: 2, pending: false },
    { id: 't2', path: 'ui/src/Chat.svelte', line: 12, resolved: true, outdated: false, author: 'reviewer', preview: 'Why a second slider?', count: 1, pending: true },
  ],
}
export const loadPR = (arg) => (window.vPR ? window.vPR(arg) : Promise.resolve(mockPR))
const act = (name, ...args) => (window[name] ? window[name](...args) : Promise.resolve(mockPR))
export const prReply = (thread, body) => act('vPRReply', thread, body)
export const prComment = (path, start, line, body) => act('vPRComment', path, start, line, body)
export const prChecks = () =>
  window.vPRChecks
    ? window.vPRChecks()
    : Promise.resolve([
        { name: 'build', workflow: 'CI', bucket: 'pass', link: 'https://github.com/dasimagin/v/actions/runs/1', description: 'Build succeeded in 1m 2s' },
        { name: 'test (ubuntu)', workflow: 'CI', bucket: 'fail', link: 'https://github.com/dasimagin/v/actions/runs/2', description: 'Tests failed: 2 of 410' },
        { name: 'lint', workflow: 'CI', bucket: 'pending', link: 'https://github.com/dasimagin/v/actions/runs/3', description: 'In progress' },
        { name: 'docs', workflow: 'Pages', bucket: 'skipping', link: '', description: '' },
      ])
export const openURL = (u) => (window.vOpenURL ? window.vOpenURL(u) : window.open(u, '_blank'))
export const prEdit = (title, body) => act('vPREdit', title, body)
export const prTalk = (body, summary) => act('vPRTalk', body, summary)
export const prResolve = (thread, resolved) => act('vPRResolve', thread, resolved)
export const prDelete = (id, conversation) => act('vPRDelete', id, conversation)
export const prSubmit = (verdict, summary) => act('vPRSubmit', verdict, summary)
export const prView = (path) =>
  window.vPRView
    ? window.vPRView(path)
    : Promise.resolve(
        path === ''
          ? { body: 'Adds a window with the start screen, `/open`, `/show` and `/pr`.\n\n```sh\ngo run . --ui\n```', reviews: [{ author: 'reviewer', when: '2h ago', state: 'changes requested', body: 'Please split the omnibox.' }], comments: [{ id: 'c1', mine: true, author: 'dasimagin', when: '1h ago', body: 'Done.' }], lines: [], outdated: [], summary: '' }
          : { body: '', reviews: [], comments: [], outdated: [], lines: [
              { k: 'head', t: 'diff --git a/gui.go b/gui.go' },
              { k: 'hunk', t: '@@ -0,0 +1,4 @@' },
              { k: 'add', no: 1, t: '+package main' },
              { k: 'add', no: 2, t: '+', threads: [{ id: 't1', path: 'gui.go', line: 2, resolved: false, outdated: false, comments: [{ id: 'a', author: 'reviewer', when: '2h ago', body: 'Blank line needed here?' }, { id: 'b', mine: true, pending: true, author: 'dasimagin', when: '1h ago', body: 'Yes, gofmt.' }] }] },
              { k: 'del', old: 1, t: '-func old() {}' },
              { k: 'del', old: 2, t: '-\treturn 1' },
              { k: 'add', no: 3, t: '+func renamed() {' },
              { k: 'add', no: 4, t: '+\treturn 2' },
              { k: 'add', no: 5, t: '+\t// a very long added line that has to wrap in the side by side layout when the column is narrow' },
              { k: 'ctx', old: 3, no: 6, t: ' func main() {}' },
              { k: 'ctx', old: 4, no: 7, t: ' ' },
            ] },
      )
export const quit = () => (window.vQuit ? window.vQuit() : undefined)

// The shell mode: output comes back as shell-out and shell-done events.
export const shell = (id, cmd, cols) => {
  if (window.vShell) return window.vShell(id, cmd, cols)
  const e = (x, ms) => setTimeout(() => window.vEvent?.(x), ms)
  e({ kind: 'shell-out', id, text: 'total 8\n-rw-r--r--  1 you  staff  412 main.go\n' }, 250)
  e({ kind: 'shell-out', id, text: 'drwxr-xr-x  9 you  staff  288 ui\n' }, 450)
  e({ kind: 'shell-done', id, turns: 0 }, 650)
  return Promise.resolve()
}
export const shellStop = (id) => (window.vShellStop ? window.vShellStop(id) : Promise.resolve())
export const branchPRs = () => (window.vBranchPRs ? window.vBranchPRs() : Promise.resolve([{ number: 42, title: 'Add the graphical UI', url: 'https://github.com/dasimagin/v/pull/42', state: 'open' }]))
