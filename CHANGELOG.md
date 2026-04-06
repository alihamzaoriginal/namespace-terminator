# Changelog

All notable changes to this project will be documented in this file.

The format is based on Keep a Changelog, and this project aims to follow Semantic Versioning.

## [1.0.0]

### Added
- Initial `nst` CLI implementation for force-terminating Kubernetes namespaces by name.
- Bulk targeting for namespaces stuck in the `Terminating` state.
- Support for `--kubeconfig`, `--context`, `--dry-run`, `--yes`, `--output`, and `--timeout`.
- Text and JSON output modes with non-zero exits for failed or pending namespace termination.
- GoReleaser configuration and GitHub Actions release workflow for Linux, macOS, and Windows builds.
- README usage, installation, and safety documentation.
