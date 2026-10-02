# ttyosk

`ttyosk` is a terminal-based on-screen keyboard with an interactive `zsh` shell above it. The shell runs in a pseudo-terminal, and its output is rendered in the upper half of the terminal window.

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

## Build and Run

From the project directory:

```sh
go build -o ttyosk .
./ttyosk
```

Or run it directly:

```sh
go run .
```

The application launches `zsh` in a PTY. Shell output appears above the virtual keyboard. Resizing the terminal resizes the shell PTY as well.

## Keyboard Controls

- Click a displayed key to type it. The terminal emulator must support SGR mouse reporting for clicks.
- Use the physical arrow keys, or `h`, `j`, `k`, and `l`, to move the selection. Press Space or physical Enter to activate the selected virtual key.
- The virtual Return key submits the current command to the shell.
- Virtual Shift affects the next character only. The `caps` key beside `a` toggles Caps Lock for letters; Shift temporarily reverses Caps Lock for one character. Ctrl and Alt apply their modifier modes. Virtual Esc, Tab, and arrow keys send those keys to the shell.
- Press physical Escape to exit `ttyosk`. The virtual Esc key sends Escape to the shell instead.

## Tests

Run the test suite with:

```sh
go test ./...
```