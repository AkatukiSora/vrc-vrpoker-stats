# Windows Build in CI

This project uses Linux-first CI for day-to-day checks, and keeps a weekly native
Windows verification workflow.

## Why this setup

- The app uses Fyne and OpenGL bindings, which require CGO for desktop builds.
- Linux cross-builds are faster for regular PR/push feedback loops.
- Weekly native Windows checks still validate a true Windows environment.
- Toolchain installation remains explicit in workflow history.

## CI behavior

- `pull_request` and `push` run in `ci.yml`:
  - Linux verify job
  - Linux build job
  - Linux-hosted Windows cross-build job (`mingw-w64`)
- Version tag push (`v*`) additionally runs release packaging and publishes:
  - `vrpoker-stats-<tag>-linux-wayland.tar.gz`
  - `vrpoker-stats-<tag>-windows-amd64.zip`

- Weekly `windows-native-weekly.yml` runs:
  - Native Windows build on `windows-latest`
  - Only when changes exist since the previous successful weekly run

## Linux cross-build toolchain

The regular CI workflow installs MinGW GCC via apt and builds with:

```bash
sudo apt-get install -y --no-install-recommends gcc-mingw-w64-x86-64 pkg-config
CGO_ENABLED=1 GOOS=windows GOARCH=amd64 CC=x86_64-w64-mingw32-gcc go build -ldflags "-s -w" -o vrpoker-stats.exe .
```

## Local reproduction on Linux

Install dependencies and run:

```bash
mise run deps-linux
mise run build-windows
```

This uses:

```bash
CGO_ENABLED=1 GOOS=windows GOARCH=amd64 CC=x86_64-w64-mingw32-gcc go build -ldflags "-s -w" -o vrpoker-stats.exe .
```

## Troubleshooting

- `x86_64-w64-mingw32-gcc not found`
  - Install MinGW (`mise run deps-linux` or `sudo apt-get install -y gcc-mingw-w64-x86-64`) and ensure PATH is correct.
- `build constraints exclude all Go files in .../go-gl/gl`
  - This usually indicates `CGO_ENABLED=0` or missing CGO compiler.
