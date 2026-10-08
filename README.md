# kaitui

Mouse-first terminal UI for **[kai](https://github.com/atagulalan/kaijou)** boards (Bubble Tea).

## Quick start

```bash
cd your-project
npx kaitui
# until on npm: npx github:atagulalan/kaitui
```

What `npx kaitui` does:

1. Ensures `./kai` exists (runs `npx kaijou` if missing → also inits `.kai/` when needed)
2. Downloads a platform binary to `./kaitui` from GitHub Releases
3. Launches the TUI

## Requires

- `./kai` + `.kai/` (board) — installed automatically via kaijou when using npx
- A real TTY (terminal)

## Build from source

```bash
go build -ldflags="-s -w" -o kaitui .
go test ./...
```

## Manual install

```bash
cp kaitui /path/to/board-project/kaitui
# ensure ./kai exists there, then:
./kaitui
# or: ./kai tui
```

## UI

- Header: project title · info · refresh / quit
- Board: columns; left-click = detail, right-click = multi-select; drag/wheel scroll
- AI log above prompt (click for full transcript)
- Enter → agent; slash: `/refresh` `/quit` `/clear` `/help`

Agent settings come from `.kai/config` (`agent=…`, `agent.*=…`).

## License

MIT
