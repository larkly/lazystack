# lazystack

A keyboard-driven terminal UI for OpenStack, built with Go.

## Features

- Cloud picker from `clouds.yaml`
- Server list with auto-refresh, filtering, and status colors
- Server detail view
- Server create form with inline filterable pickers
- Delete and reboot with confirmation modals
- Help overlay with context-sensitive keybindings
- Error handling with modal display

## Install

```bash
go install github.com/bosse/lazystack/cmd/lazystack@latest
```

Or build from source:

```bash
go build ./cmd/lazystack
```

## Usage

Ensure you have a valid `clouds.yaml` in one of:

- Current directory
- `$OS_CLIENT_CONFIG_FILE`
- `~/.config/openstack/clouds.yaml`
- `/etc/openstack/clouds.yaml`

Then run:

```bash
./lazystack
```

## Keybindings

### Global

| Key       | Action        |
|-----------|---------------|
| `q`       | Quit          |
| `?`       | Toggle help   |
| `C`       | Switch cloud  |

### Server List

| Key       | Action        |
|-----------|---------------|
| `j/k`     | Navigate      |
| `Enter`   | View detail   |
| `c`       | Create server |
| `d`       | Delete server |
| `r`       | Soft reboot   |
| `R`       | Force refresh |
| `/`       | Filter        |

### Server Detail

| Key       | Action        |
|-----------|---------------|
| `j/k`     | Scroll        |
| `d`       | Delete server |
| `r`       | Soft reboot   |
| `R`       | Hard reboot   |
| `Esc`     | Back to list  |

### Create Form

| Key         | Action        |
|-------------|---------------|
| `Tab`       | Next field    |
| `Shift+Tab` | Prev field    |
| `Enter`     | Open picker   |
| `Ctrl+S`    | Submit        |
| `Esc`       | Cancel        |

## License

Apache 2.0
