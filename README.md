# gm-tui

A clean terminal UI for **batch image resize and compression** using [GraphicsMagick](http://www.graphicsmagick.org/).

Point it at a folder, set a target size and quality, and let it chew through every JPEG in the tree — either writing to a mirrored `output/` folder, or overwriting files in-place.

```
GM TUI — Batch Image Resize & Compress
Powered by GraphicsMagick

Base directory
│ ~/Pictures/vacation

Resize  (W×H)
│ 1200x1200

JPEG quality  (1–100)
│ 80

Output mode
  ●  Preserve originals  →  write to output/ folder
  ○  Overwrite files in-place  →  gm mogrify

[Tab] next field   [↑↓] change mode   [Enter] run   [Ctrl+C / q] quit
```

---

## Requirements

| Dependency | Notes |
|---|---|
| **Go 1.22+** | Module-based, no GOPATH assumptions |
| **GraphicsMagick** (`gm`) | Must be in `PATH` before running |

### Install GraphicsMagick

```bash
# macOS (Homebrew)
brew install graphicsmagick

# Debian / Ubuntu
sudo apt install graphicsmagick

# Fedora / RHEL
sudo dnf install GraphicsMagick
```

Verify the install:

```bash
gm version
```

---

## Build & Run

### One-liner (no install)

```bash
git clone https://github.com/brunovpinheiro/ImageSlim
cd gm-tui

# Download dependencies
go mod tidy

# Run directly
go run .
```

### Build a binary

```bash
go build -o gm-tui .
./gm-tui
```

### Install to `$GOPATH/bin` (add to your shell PATH)

```bash
go install github.com/brunovpinheiro/ImageSlim@latest
gm-tui
```

> **Note:** adjust the module path in `go.mod` (and the install URL above) to match your own GitHub username/repository before publishing.

---

## Usage

Run `gm-tui` (or `go run .`) and fill in the form:

| Field | Default | Description |
|---|---|---|
| Base directory | `~/Pictures` or `.` | Root folder scanned recursively for `*.jpg` files |
| Resize (W×H) | `1200x1200` | GraphicsMagick geometry string — aspect ratio is preserved |
| JPEG quality | `80` | 1 = smallest file, 100 = best quality |
| Output mode | Preserve | See below |

### Keyboard shortcuts

| Key | Action |
|---|---|
| `Tab` / `Shift+Tab` | Move focus between fields |
| `↑` / `↓` | Change output mode (when mode selector is focused) |
| `Enter` | Start processing |
| `Ctrl+C` | Quit (works on any screen) |
| `q` | Quit (from mode selector, done, or error screens) |
| `r` | Go back to the form and run another job |

---

## Output modes

### Preserve originals *(default)*

Creates an `output/` subdirectory inside the base directory and mirrors the entire folder tree there.  Original files are **never modified**.

Equivalent shell command:

```bash
mkdir -p output && find . -type f -iname "*.jpg" | while IFS= read -r f; do
  out="output/${f#./}"
  mkdir -p "$(dirname "$out")"
  gm convert "$f" -resize 1200x1200 -quality 80 "$out"
done
```

### Overwrite in-place

Runs `gm mogrify` on every matching file, **replacing** them with the resized/recompressed versions.  Use with caution — there is no undo.

Equivalent shell command:

```bash
find . -type f -iname "*.jpg" -exec gm mogrify -resize 1200x1200 -quality 80 {} \;
```

---

## Project structure

```
gm-tui/
├── main.go              # Bubble Tea TUI (form, running, done, error screens)
├── internal/
│   └── gm/
│       └── gm.go        # GraphicsMagick wrapper (Options, Result, Run)
├── go.mod
└── README.md
```

---

## Dependencies

| Package | Role |
|---|---|
| [`charmbracelet/bubbletea`](https://github.com/charmbracelet/bubbletea) | TUI framework (Elm-style) |
| [`charmbracelet/bubbles`](https://github.com/charmbracelet/bubbles) | Text input, spinner, viewport components |
| [`charmbracelet/lipgloss`](https://github.com/charmbracelet/lipgloss) | Terminal styling |

GraphicsMagick itself is invoked as an external subprocess — no image processing happens inside Go.

---

## License

MIT
