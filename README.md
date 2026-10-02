# ttyosk

`ttyosk` is a terminal-based on-screen keyboard with an interactive `zsh` shell above it. The shell runs in a pseudo-terminal, and its output fills the terminal window above the on-screen keyboard.

## Requirements

- Go 1.27.1 or newer.
- Linux or another Unix-like system that supports pseudo-terminals and `SIGWINCH`. Windows is not supported by the current implementation.
- `zsh` installed and available as `zsh` on `PATH`.
- A UTF-8 terminal emulator with ANSI/VT control-sequence and SGR mouse-reporting support. Mouse clicks on the displayed keys require SGR mouse reporting.
- An 80-column by 24-row terminal is recommended. Smaller windows may cause the keyboard layout to wrap or overlap.

## Dependencies

Go modules are downloaded automatically by the Go toolchain. Direct module dependencies are:

- [`github.com/creack/pty`](https://github.com/creack/pty): starts and resizes the shell pseudo-terminal.
- [`github.com/hinshun/vt10x`](https://github.com/hinshun/vt10x): emulates the shell's VT screen and control sequences.
- [`golang.org/x/term`](https://pkg.go.dev/golang.org/x/term): reads terminal dimensions and switches the input terminal to raw mode.

`golang.org/x/sys` is used indirectly by the terminal dependencies. No separate system installation is needed for these Go modules; the first build requires access to the Go module proxy unless the modules are already cached.

## Installation

Install Go, `zsh`, and Git using your distribution's package manager:

### Arch Linux

```sh
sudo pacman -S --needed go zsh git
```

### Fedora

```sh
sudo dnf install golang zsh git
```

### Ubuntu

```sh
sudo apt update
sudo apt install golang-go zsh git
```

Check the installed Go version:

```sh
go version
```

The required version is Go 1.27.1 or newer. If your distribution's package is
older, install a supported Go release from [go.dev/dl](https://go.dev/dl/) and
ensure its `go` command is first on your `PATH`.

Clone and build `ttyosk`:

```sh
git clone https://github.com/SlushyLint/ttyosk.git
cd ttyosk
go build -o ttyosk .
./ttyosk
```

Go downloads the module dependencies during the first build, so internet access
to the Go module proxy is needed unless the dependencies are already cached.
You can also run the program without creating a binary:

```sh
go run .
```

The application launches `zsh` in a PTY. Shell output appears above the virtual
keyboard. Resizing the terminal resizes the shell PTY as well.

## Keyboard Controls

- Click a displayed key to type it. The terminal emulator must support SGR mouse reporting for clicks.
- Use the physical arrow keys, or `h`, `j`, `k`, and `l`, to move the selection. Press Space or physical Enter to activate the selected virtual key.
- The virtual Return key submits the current command to the shell.
- Virtual Shift, Ctrl, and Meta apply to the next keypress; active one-shot modifiers are highlighted blue. Activate a modifier again before using it to turn it off. Ctrl and Meta can be combined or toggled off independently; Meta sends an Escape prefix before the key. The `caps` key toggles Caps Lock for letters and stays highlighted blue while enabled; Shift temporarily reverses Caps Lock for one character. Virtual Esc, Tab, and arrow keys send those keys to the shell.
- Press physical Escape to exit `ttyosk`. The virtual Esc key sends Escape to the shell instead.

## Tests

Run the test suite with:

```sh
go test ./...
```