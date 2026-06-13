# Versioning

KoalaBye follows Semantic Versioning (SemVer) with the following structure: `vMAJOR.MINOR.PATCH`

## Experimental Releases
Releases with versions `< v1.0.0` are considered experimental/early.
- Breaking changes to APIs, database schema, and workflows may happen before v1.
- Always back up before every update.
- Database migrations should be treated carefully.
- We do not use RC naming unless we explicitly choose to later.

Privacy and diagnostics work in this branch prepares the next experimental release after v0.1.2. The wording does not create a release, tag, or compatibility guarantee.

## Docker Images
When the release workflow is configured to publish images, the expected tags are:
- `ghcr.io/<owner>/koalabye:v0.1.0` (or matching Git tag)
- `ghcr.io/<owner>/koalabye:sha-<shortsha>` (if the workflow supports it)

## Verifying image provenance
You can verify that the published container image was built from the source repository using GitHub Artifact Attestations:

```bash
gh attestation verify oci://ghcr.io/<owner>/koalabye:<version> --repo <owner>/KoalaBye
```
*(Replace `<owner>` with the actual repository owner)*
