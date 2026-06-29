# Ward

Ward is a zero-dependency, lightweight process supervisor written in Go.

You define your processes in a TOML file. Ward keeps them running, tells you when they crash and lets you tail their output in real time.

---

## Why not just use...

**Docker Compose** - requires Docker, which on a $5 VPS costs more RAM than your application. Ward is a single 8MB binary.

**supervisord** - requires Python, hasn't been meaningfully maintained in years, and its config syntax is painful. Ward uses TOML.

**systemd** - two unit files per process, no decent `logs -f` equivalent, and writing them correctly requires reading documentation every time. Ward has one config file for everything.

---

## Quick Start

```toml
# ~/.config/ward/ward.toml

[[process]]
name = "api"
command = "/usr/local/bin/my-api"
args = ["--port", "8080"]
restart = "always"
```

```bash
ward up
```

That's it. Ward starts your process, restarts it if it crashes, and keeps its output available.

---

## Installation

```bash
make build
sudo make install
```

Or download a prebuilt binary from the releases page.

---

## Configuration

```toml
[settings]
socket_path = "/tmp/ward.sock"
db_path     = "~/.local/share/ward/ward.db"
pid_path    = "/tmp/ward.pid"

[[process]]
name          = "api"
command       = "/usr/local/bin/my-api"
args          = ["--port", "8080"]
dir           = "/home/user/myapp"
env           = { PORT = "8080", LOG_LEVEL = "info" }
restart       = "always"      # always | on-failure | never
restart_delay = "5s"
max_restarts  = 0             # 0 = unlimited
grace_period  = "10s"
stdout        = "inherit"     # inherit | discard | /absolute/path/to/file
stderr        = "inherit"
autostart     = true
```

`restart = "always"` restarts on any exit. `on-failure` only restarts on non-zero exit codes. `never` runs the process once and doesn't touch it again.

`stdout = "inherit"` pipes output through the Ward daemon, which you can redirect however you like when starting it. Set it to an absolute path to write directly to a file.

---

## Commands

```bash
# Daemon
ward up               # start the daemon (foreground)
ward down             # stop the daemon and all processes
ward reload           # hot-reload config without downtime

# Processes
ward status           # show all processes
ward status api       # show detailed status for one process
ward start api
ward stop api
ward restart api

# Logs
ward logs api         # last 50 lines
ward logs api -n 100  # last 100 lines
ward logs -f api      # live stream, survives crashes and restarts
```

---

## Running on Boot (systemd)

```bash
mkdir -p ~/.config/systemd/user
cp ward.service ~/.config/systemd/user/ward.service
systemctl --user enable ward
systemctl --user start ward
```

Check Ward's own output:
```bash
journalctl --user -u ward -f
```