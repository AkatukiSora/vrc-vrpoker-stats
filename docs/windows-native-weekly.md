# Weekly Native Windows Verification

This repository runs `windows-native-weekly.yml` once per week to validate a
native Windows build.

## Trigger

- `schedule`: every Monday at 03:00 UTC
- `workflow_dispatch`: manual run from Actions tab

## Change-aware execution

The workflow compares:

- current default-branch HEAD (`GITHUB_SHA`)
- head SHA from the previous successful **scheduled** run of the same workflow

If no diff exists, native build is skipped to save CI minutes.

## Native build details

- runner: `windows-latest`
- toolchain: `msys2/setup-msys2` + `mingw-w64-x86_64-gcc`
- build command:

```powershell
$env:CGO_ENABLED = "1"
$env:GOOS = "windows"
$env:GOARCH = "amd64"
$env:CC = "C:\msys64\mingw64\bin\gcc.exe"
go build -ldflags "-s -w" -o vrpoker-stats.exe .
```

Artifacts are uploaded with 7-day retention.
