# Download destination policy

`DOWNLOAD_FILE.destination_path` is an optional absolute directory on the agent.
An omitted or empty value keeps `Config.Directory`. A supplied value is used for
disk-space checks, temporary files and the verified final file. Failures never
fall back to the default directory.

The agent rejects relative paths, paths for another OS, UNC/device paths,
control characters, dot/traversal components and existing symlink/reparse-point
ancestors (including Windows junctions). Filenames must be single local names;
Windows device names and alternate data streams are rejected. Paths are not
expanded through environment variables or a shell.

Service mode supports an external default and multiple allowed destination roots:

```env
DOWNLOAD_DIRECTORY=D:\Downloads
DOWNLOAD_ALLOWED_ROOTS=D:\Downloads;D:\Lessons;C:\Shared
```

An empty allowlist permits only the default tree in Service mode. An explicit
allowlist uses semicolon-separated absolute local directories, and must contain
the default directory (or an ancestor of it). A command may choose any allowed
root or descendant, such as `D:\Lessons\Room101`. Relative DOWNLOAD_DIRECTORY
still resolves under the runtime root; allowed roots are always absolute.
Invalid configuration is rejected during provisioning before enrollment and at
Service startup. Drive roots, private runtime state and installed Agent binaries
cannot be allowed; runtime downloads may use a subdirectory of runtime `data`.

External roots retain existing/inherited NTFS permissions; the Service account
must be able to create and write files there. Missing directories are created on
use. Temporary-file creation checks write access before the HTTP request. Windows
directory handles remain pinned throughout the job; reparse points, hard-linked
files and path escapes are rejected at file I/O boundaries. Runtime files retain
the original private ACL checks. SHA-256 verification precedes final replacement.
Console mode without an explicit allowlist retains its existing destination policy.

For an installed Service, edit `%ProgramData%\ThesisAgentDev\.env` and restart it.
For a new installation, edit `install/config/service.env` first. Update/reinstall
with an existing identity preserves the installed config. Uninstall deletes the
Agent's runtime downloads but retains external download folders and their files.

Invalid destinations return `FAILED` / `INVALID_DESTINATION_PATH`. Directory
creation and file-write failures return `WRITE_FAILED`; errors contain no local
paths. Expired URLs use `TOKEN_EXPIRED`, and the free-space check uses `DISK_FULL`.
Size and SHA-256 verification still precede final replacement, so failed
verification leaves any existing destination file intact.

Run `go test ./...` for the download and existing-system regression tests.
