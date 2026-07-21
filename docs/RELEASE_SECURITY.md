# Release security

Tribunal release archives are produced only by the tag-triggered release
workflow. GoReleaser builds static macOS/Linux binaries, writes adjacent binary
SHA-256 manifests, creates archive checksums and SBOMs, and signs checksum/SBOM
artifacts with keyless Cosign. GitHub also attests archives and SBOMs.

After extraction, run:

```bash
./tribunal verify-install
```

The command rejects missing build provenance, dirty/development metadata,
missing manifests, and checksum mismatches. Release publication, tags, and
remote pushes are maintainer actions and are not part of ordinary build work.
