# Security Policy

## Threat Model

Senvu is a **local-first** tool. It operates entirely on the local filesystem and makes **no network connections**. This eliminates entire classes of remote attack vectors.

### What Senvu Trusts

- **The local filesystem**: Senvu trusts that the operating system's filesystem APIs behave correctly. It reads and writes files using standard OS calls.
- **The local user**: Senvu runs with the permissions of the invoking user. It does not escalate privileges.

### What Senvu Does NOT Trust

- **Manifest contents from external sources**: Manifests (JSON snapshot files) received from other machines or users are treated as untrusted input. Paths in manifests are validated before use to prevent path traversal attacks.
- **Symlinks**: Senvu detects symlinks and does not follow them during scanning. Symlinks are recorded in manifests but are not dereferenced, preventing symlink escape attacks.

## Security Measures

### Path Traversal Protection

All file paths derived from manifest data or user input are validated against the workspace root using `platform.SafePath()`. Paths that resolve outside the workspace root (via `../`, symlinks, or other traversal techniques) are rejected with an error.

### Symlink Escape Protection

- The scanner detects symlinks via `os.Lstat()` and records them without following
- File operations validate that resolved paths remain within the workspace boundary
- `platform.ValidateDestination()` checks that target paths don't escape the allowed root

### Staged Writes & Rename Replacement

File writes use `platform.AtomicWrite()` — a temporary file is written in the same directory, synced to disk, and then renamed to the target path. On standard POSIX-compliant local filesystems, this provides atomic replacement preventing truncated files from interrupted processes or sudden power loss. On network-attached storage or non-POSIX filesystems, rename atomicity depends on the host environment's filesystem implementation.

### No Network Connectivity

Senvu makes **zero network connections**. There is no:

- Telemetry or analytics
- Update checking
- Cloud service integration
- DNS lookups

This means there is **no remote attack surface**.

### No Data Collection

Senvu does not collect, transmit, or store any user data beyond what is explicitly written to the local `.senvu/` workspace directory. There are no tracking identifiers, no usage metrics, and no crash reports sent anywhere.

## Known Limitations

- **Filesystem semantics:** Staged writes and atomic renames rely on the underlying OS and filesystem semantics; network filesystems (e.g., NFS, SMB) or non-POSIX platforms may not guarantee atomic replacement.
- **Concurrent live modifications:** Senvu detects file modifications that occur mid-hash via post-read stat checks, but continuous concurrent modifications by external programs during a scan or transfer may require re-running the operation once writes quiesce.
- **No encryption**: Manifests and snapshot data are stored as plaintext JSON. Anyone with filesystem access to the `.senvu/` directory can read them. Encryption support is planned for a future release.
- **No signature verification**: Manifests are not cryptographically signed. A manifest received from an external source cannot be verified as authentic. Signature support is planned.
- **No access control**: Senvu relies entirely on OS-level file permissions. It does not implement its own access control.

## Supported Versions

| Version | Supported |
|---------|-----------|
| 0.1.x   | ✅ Current |

## Reporting a Vulnerability

If you discover a security vulnerability in Senvu, please report it responsibly:

1. **Do not** open a public GitHub issue for security vulnerabilities
2. Email the maintainers with a description of the vulnerability, steps to reproduce, and potential impact
3. Allow reasonable time for a fix before public disclosure

We take security seriously and will respond to reports promptly.
