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

Console mode can create directories allowed by the running account's filesystem
permissions. Service mode additionally requires the destination to remain in
`Config.Directory` and retains the existing protected-path ACL and pinned-handle
checks for all file operations. It does not relax Service permissions to write
to arbitrary shared folders or sibling identity/runtime directories.

Invalid destinations return `FAILED` / `INVALID_DESTINATION_PATH`. Directory
creation and file-write failures return `WRITE_FAILED`; errors contain no local
paths. Expired URLs use `TOKEN_EXPIRED`, and the free-space check uses `DISK_FULL`.
Size and SHA-256 verification still precede final replacement, so failed
verification leaves any existing destination file intact.

Run `go test ./...` for the download and existing-system regression tests.
