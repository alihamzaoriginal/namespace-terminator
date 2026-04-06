# namespace-terminator

`nst` is a small cross-platform CLI for force-terminating Kubernetes namespaces.

It can:
- terminate one or more namespaces by name
- target every namespace currently stuck in `Terminating`
- use your current kubeconfig or an explicit kubeconfig/context
- run on Linux, macOS, and Windows

## Why This Exists

Sometimes a namespace gets stuck in `Terminating` because one or more finalizers never clear. `nst` requests deletion when needed, then clears namespace finalizers through the Kubernetes finalize API.

This is a forceful recovery tool. It can orphan resources that still exist behind the namespace boundary, so use it carefully.

## Install

### Homebrew

Once release automation is enabled, install with:

```bash
brew tap alihamzaoriginal/homebrew-tap
brew install nst
```

### Scoop

Scoop publishing is not enabled yet because the bucket repository has not been created.

### Direct Download

Download the appropriate archive for Linux, macOS, or Windows from the project's GitHub Releases page and place the binary on your `PATH`.

## Usage

Terminate specific namespaces:

```bash
nst payments staging
```

Terminate every namespace stuck in `Terminating`:

```bash
nst --all-terminating
```

Preview what would be targeted:

```bash
nst --all-terminating --dry-run
nst payments staging --dry-run
```

Use a specific kubeconfig or context:

```bash
nst payments --kubeconfig ~/.kube/config --context prod-cluster
```

Skip the bulk confirmation prompt:

```bash
nst --all-terminating --yes
```

Emit machine-readable output:

```bash
nst payments --output json
```

Show help or version:

```bash
nst --help
nst --version
```

## Command Reference

```text
nst <namespace> [<namespace> ...]

Flags:
  --all-terminating   Target all namespaces currently stuck in Terminating
  --context string    Kubeconfig context to use
  --dry-run           Print the namespaces that would be terminated
  --kubeconfig path   Path to a kubeconfig file
  --output string     Output format: text or json
  --timeout duration  Wait time after clearing finalizers (default 15s)
  --version           Print the nst version
  -y, --yes           Skip confirmation for bulk operations
```

## Exit Behavior

- exits `0` when every targeted namespace is deleted successfully
- exits non-zero when any namespace fails to terminate or remains pending after the timeout
- exits non-zero for invalid usage or kubeconfig errors

## How It Works

For each target namespace, `nst`:
1. resolves the target list from explicit names or from namespaces already in `Terminating`
2. sends a namespace delete request if deletion has not started yet
3. clears namespace finalizers through the finalize API
4. waits briefly for the namespace to disappear

## Development

Run locally:

```bash
go run ./cmd/nst --help
```

Run tests:

```bash
go test ./...
```

To publish Homebrew formulas to the external tap repository, add a `TAP_GITHUB_TOKEN` GitHub Actions secret with permission to write to `alihamzaoriginal/homebrew-tap`.

## Security Notes

- Do not commit kubeconfigs, tokens, private keys, or `.env` files to this repository.
- Common secret-bearing files are ignored by `.gitignore`.
- GitHub Actions runs a `gitleaks` secret scan on pushes to `main` and on pull requests.
- You can enable the same check locally with `pre-commit install`, using the repo's `.pre-commit-config.yaml`.
- False-positive tuning lives in the repo root `.gitleaks.toml`.
- The CI workflow runs `gitleaks detect --config .gitleaks.toml` explicitly.

Enable local secret scanning:

```bash
pip install pre-commit
pre-commit install
```
