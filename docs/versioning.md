# Versioning and releases

DebtDrone CLI follows [Semantic Versioning](https://semver.org/) for the public
`debtdrone-cli/v2` Go module, scanner report contract, command-line interface,
configuration format, and published binaries.

## Version policy

- **Patch** (`v2.0.1` → `v2.0.2`): compatible fixes, performance work, and
  internal refactors that do not change documented behavior.
- **Minor** (`v2.0.1` → `v2.1.0`): backwards-compatible analyzers, report
  fields, scanner APIs, flags, or configuration options.
- **Major** (`v2.x` → `v3.0.0`): incompatible public scanner API, report,
  configuration, or CLI behavior. A major release requires a matching Go
  module path and a coordinated SaaS migration; the v2 workflow intentionally
  rejects v3 tags.
- **Pre-release** versions use SemVer suffixes such as `v2.1.0-rc.1` and must
  not replace the latest stable release until explicitly promoted.

Release versions must be canonical: major, minor, patch, and numeric
pre-release identifiers cannot contain leading zeroes. DebtDrone release tags
do not use SemVer build metadata because Go module consumers must select one
unambiguous published tag.

## Maintainer release process

Do not create or push release tags manually. From the GitHub Actions page,
run the **Release** workflow on `main` and provide the intended `v2.x.y`
version. The workflow:

1. rejects invalid versions and unexpected existing tags;
2. runs the complete test suite, `go vet`, and a CLI build;
3. builds a GoReleaser snapshot to validate all release artifacts;
4. creates the annotated tag only after validation succeeds;
5. publishes the GitHub release and package-manager artifacts; and
6. dispatches the released version to the SaaS repository.

The final dispatch requires a `DEBTDRONE_AUTOMATION_TOKEN` repository secret.
Use a fine-grained token scoped to the `debtdrone` repository with **Contents:
read and write**, which GitHub requires for repository-dispatch events. Keep
the existing `GH_PAT` secret for GoReleaser and Homebrew publication.

The SaaS repository never updates directly. Its dispatch workflow verifies the
exact module version and runs the full backend suite before opening a dependency
pull request. Until that PR passes required checks, is reviewed, and is merged,
production remains pinned to the previous scanner version.

### Recovering a failed publication

If validation and tag creation succeeded but the **Publish release** job failed,
rerun the workflow with the same version and enable
`retry_failed_publication`. The workflow reuses the tag only when it points to
an immutable commit that passes the complete validation suite again, then
reruns GoReleaser in replacement mode. This remains safe if `main` has advanced
since the original release attempt.

Do not enable this option when publication succeeded and only the SaaS dispatch
failed. In that case, manually run the SaaS **Scanner Dependency Update**
workflow with the published tag.
