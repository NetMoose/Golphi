# Windows startup-thread probe

This unattended experiment targets an **interactive Windows 10/amd64 desktop**. It creates one hidden native STATIC window, receives actual Win32 `EnumWindows` callbacks (including its own window), checks startup/main/callback OS-thread identity and worker separation, and destroys the window on the startup thread. No clicks are required. It prints the actual Windows build and system DPI/scale after enabling system DPI awareness. The printed scale is the system/primary scale, not all monitors' settings.

Build from the repository root with Go 1.26.5 (no C compiler required):

```sh
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o /tmp/golphi-probe-windows-amd64.exe ./cmd/probe-windows
```

Copy the executable to Windows. On Windows, run this PowerShell from the directory containing it. Keep the generated log and report the printed exit code:

```powershell
$ErrorActionPreference = 'Stop'
$log = Join-Path $PWD 'golphi-windows-probe.txt'
$cpu = Get-CimInstance Win32_Processor | Select-Object -First 1
$os = Get-CimInstance Win32_OperatingSystem
"Host CPU architecture code: $($cpu.Architecture) (9=AMD64, 12=ARM64); OS: $($os.Caption); build: $($os.BuildNumber); OS architecture: $($os.OSArchitecture)" | Tee-Object -FilePath $log
& .\golphi-probe-windows-amd64.exe 2>&1 | Tee-Object -FilePath $log -Append
$probeExit = $LASTEXITCODE
"Probe exit code: $probeExit" | Tee-Object -FilePath $log -Append
if ($probeExit -ne 0) { throw "Native probe failed (exit $probeExit); preserve $log" }
```

A successful result requires `PASS`, exit code 0, nonzero callback count, `own-window-seen=true`, identical startup/main/callback/cleanup thread IDs, a different worker thread ID, and successful `DestroyWindow=1`. Preserve the complete log even on failure. Confirm separately that the host is Windows 10 on native amd64 (Windows 11 also reports major version 10; its build is normally 22000 or greater). Run from a normal logged-in desktop, not a service or headless session. The native DPI query requires Windows 10 version 1607 or later; an unavailable API fails visibly.

This is a focused native-call/callback/thread experiment. It does not implement `Form.Run`, a Win32 message loop, Go-to-UI posting, application events, or full shutdown behavior. A portable unit test and a successful cross-build do **not** establish Windows runtime correctness. Task 1.4 remains open until an actual target-host log records the required evidence.

Documentation: [EnumWindows](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-enumwindows), [CreateWindowExW](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-createwindowexw), [GetDpiForSystem](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-getdpiforsystem). Context7 was attempted first but failed with a transport error; these official Microsoft references were used as fallback.
