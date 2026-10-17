<div align="center">

```
██╗          ██╗
╚██╗        ██╔╝
 ╚██╗      ██╔╝
  ╚██╗    ██╔╝
   ╚██╗  ██╔╝
    ╚██╗██╔╝
     ╚███╔╝
      ╚══╝
```

*v is for vibing*

</div>

`v` is a terminal tool for coding together with an AI agent: you and the agent take turns editing the same code.

Right now it is a splash screen. The plan:

- an editor with syntax highlighting (One Dark Pro Darker);
- diff against git `HEAD`: unified and side-by-side;
- an omnibox for commands and prompts;
- an agent on top of Claude Code that uses your Claude subscription login, where every edit it proposes can be reviewed and changed before it lands.

## Build

Requires Go 1.27+ and a truecolor terminal (iTerm2, Ghostty, WezTerm, Kitty).

```sh
make build   # go build -o v .
make run     # build and start
make lint    # golangci-lint (brew install golangci-lint)
```

Or install into `$GOPATH/bin`:

```sh
go install github.com/dasimagin/v@latest
```

Quit with `/q` or `Ctrl-C`.
