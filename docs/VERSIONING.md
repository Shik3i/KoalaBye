# Versioning

KoalaBye follows Semantic Versioning (SemVer) with the following structure: `vMAJOR.MINOR.PATCH`

## Tags

### Stable Tags
Stable releases are tagged like `v0.1.0`. These tags are intended for production use.

### Release Candidate (RC) Tags
Pre-release versions are tagged like `v0.1.0-rc.1`.
- These are intended for private, local, or staging use.
- They help verify deployment operations, database migrations, and general stability before a stable launch.
- RC tags should not be considered polished public production launches.

## Docker Images
When the release workflow is configured to publish images, the expected tags are:
- `v0.1.0-rc.1` (or matching Git tag)
- `<commit-SHA>` (if the workflow supports it)
- `latest` (only applied to stable releases later, not to RCs)

Note: The CI workflows must explicitly support publishing these tags for them to appear in the container registry.
