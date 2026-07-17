# Release Security

Tagteam release archives are built by the tag-triggered GitHub Actions release
workflow. That workflow publishes SHA-256 checksums, one SPDX JSON SBOM per
archive, keyless Sigstore bundles for the checksum file and each SBOM, and
GitHub repository-bound provenance attestations for the archives and SBOMs.

All third-party workflow actions are pinned to full commit SHAs. Workflow-level
permissions are read-only; only the publishing job receives `contents: write`,
`id-token: write`, and `attestations: write`.

## Verify a release

Set `TAG` to an exact release tag, then download its assets:

```sh
TAG=vX.Y.Z # replace with a release created by the hardened workflow
gh release download "$TAG" --repo cephalopod-ai/tagteam --dir "release-$TAG"
```

Verify the keyless checksum signature. The certificate identity binds the
signature to this repository's release workflow and exact tag:

```sh
cosign verify-blob \
  --certificate-identity "https://github.com/cephalopod-ai/tagteam/.github/workflows/release.yml@refs/tags/$TAG" \
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
  --bundle "release-$TAG/checksums.txt.sigstore.json" \
  "release-$TAG/checksums.txt"
```

After the signature passes, verify the downloaded archives against the signed
checksum file:

```sh
(cd "release-$TAG" && shasum -a 256 -c checksums.txt)
```

Verify GitHub provenance for an archive:

```sh
gh attestation verify "release-$TAG/tagteam_${TAG#v}_linux_amd64.tar.gz" \
  --repo cephalopod-ai/tagteam
```

Each `*.sbom.json` file has a matching `*.sigstore.json` bundle. Verify it with
the same certificate identity and issuer shown above, passing the SBOM bundle
and SBOM paths to `cosign verify-blob`.

Archive smoke tests run on Linux and macOS for every published architecture and
execute `tagteam verify-install` from the downloaded release asset.
