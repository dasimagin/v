# Built-in themes

VS Code color themes, embedded into `v` as is. The only change is the
top-level `name`, which `v` shows in `/config`: "One Dark Pro Darker" (upstream
"One Dark Pro"); the extensions set their labels in package.json instead. Drop
any other dark VS Code theme JSON into `~/.v/themes/` to make it show up in
`/config`. Light themes are not supported: `v` is built for dark ones and skips
a theme whose background is light.

| File | Theme | Source | License |
|---|---|---|---|
| `one-dark-pro.json` | One Dark Pro | [Binaryify/OneDark-Pro](https://github.com/Binaryify/OneDark-Pro) 3.20.3 | MIT, [LICENSE-one-dark-pro.txt](LICENSE-one-dark-pro.txt) |
| `one-dark-pro-darker.json` | One Dark Pro Darker | same | same |
| `github-dark-default.json` | GitHub Dark Default | [primer/github-vscode-theme](https://github.com/primer/github-vscode-theme) 6.3.5 | MIT, [LICENSE-github-theme.txt](LICENSE-github-theme.txt) |
