# OpenSSF Best Practices evidence

TempestKeep uses the existing [OpenSSF project entry 14460][entry]. Its README
badge displays that entry's live status. The target is the [Passing level][criteria].
OpenSSF Scorecard is a separate automated assessment; a successful Scorecard
run does not award a Best Practices badge.

## Assessment record

Assessment date: 2026-09-05. Starting source revision:
`4b15e5bf5d6646be3d002064583142ba4d929479`.

[.bestpractices.json](../.bestpractices.json) records proposed answers and
supporting URLs for all 67 Passing criteria. These are project statements, not
another project's copied answers. Recheck time-dependent claims on each update.

- The repository is public, active, and accepts GitHub Issues. The issue API
  returned no bug reports or enhancement requests at assessment time, so there
  is no unanswered historical backlog to count.
- The maintainer confirmed no private vulnerability reports in the preceding
  six months. The response-history criterion is therefore not applicable;
  the future response policy is not a claim of measured response times.
- The maintainer confirmed the secure-design and common-error knowledge
  criteria. [Secure development](secure-development.md) records their application.
- The advisory API returned no project advisories. CodeQL reported no open
  alerts. Local vulnerability and secret scans are required below.
- No stable release has been published. Git commits uniquely identify source
  revisions; future releases require immutable SemVer tags and reviewed notes.

## Evidence by area

| Area | Evidence |
|---|---|
| Purpose, installation, and participation | [README](../README.md), [support](../SUPPORT.md), [contribution process](../CONTRIBUTING.md) |
| License and public development | [MIT license](../LICENSE), public Git history, Issues, and pull requests |
| External interfaces | [CLI help and guide](../cmd/tempestkeep/README.md), [MCP guide](mcp.md), [Go reference][go-docs], [public declarations](public-api.txt) |
| Versioning and upgrade impact | [Release process](../RELEASING.md), [changelog](../CHANGELOG.md) |
| Reports and responses | [Support](../SUPPORT.md), [security policy](../SECURITY.md), [private reporting][private-report] |
| Builds, tests, warnings, and analysis | [Makefile](../Makefile), [CI](../.github/workflows/ci.yml), [CodeQL](../.github/workflows/codeql.yml), [lint settings](../.golangci.yml), [properties](testing/properties.md) |
| Secure design and cryptography | [Development guide](secure-development.md), [threat model](threat-model.md), [transport](../pkg/tempest/api/transport.go), [tests](../pkg/tempest/api/transport_test.go) |
| Supply-chain observations | [Scorecard workflow](../.github/workflows/scorecard.yml) and its `scorecard-results` artifact |

## Reproduce the engineering evidence

Use Go 1.27.0 and a C toolchain for race checks. No live WeatherFlow token is
needed:

```sh
make verify
make cover
```

`make verify` checks modules, formatting, imports, docs, vet, tests, races,
three bounded fuzz targets, static analysis, workflows, public API evidence,
vulnerabilities, licenses, SBOM generation, secrets, and pure-Go builds for
the host and Linux ARM64. Missing tools and failed checks must return failure.
`make cover` measures statements, not branches; inspect untested behavior too.

Local result on 2026-09-05: `make verify` passed with Go 1.27.0, including all
three fuzz targets, race checks, zero lint issues, and no reported vulnerabilities
or leaked credentials. A simulated imports-tool failure also correctly caused
`make fmtcheck` to fail. GitHub workflow results must be checked after pushing.

The 2026-09-05 coverage run measured 70.0% of statements overall. Store, model,
collector, API, and MCP packages have substantial coverage, while some command
paths and demo helpers remain uncovered. The optional `test_most` answer stays
Unmet because broad branch and input-field coverage has not been established.
The optional release-tag answer also stays Unmet until a release is tagged.

Before a production release, follow [RELEASING.md](../RELEASING.md). Record the
exact commit, check results, and remaining limitations. Tests and empty scanner
results do not prove that software has no defects.

## Update the existing badge entry

After pushing the evidence and checking GitHub CI, sign in to the existing
owner account and edit [project 14460][entry]. The
[repository automation][json-format] can propose `.bestpractices.json` answers.
Use the form's save-and-continue automation action to refresh them, review the
justifications, and save the assessment. Confirm the site itself shows Passing
before claiming the badge has been earned.

The JSON file and README badge do not update the hosted checklist automatically.
Never replace the live badge with a static image claiming a higher level.

Reassess after security reports, significant features, dependency changes, and
releases. Meet the response policy and resolve findings; do not preserve stale
answers just to retain a badge.

[entry]: https://www.bestpractices.dev/en/projects/14460/passing
[criteria]: https://www.bestpractices.dev/en/criteria/0
[go-docs]: https://pkg.go.dev/github.com/lennrt/tempestkeep/pkg/tempest/api
[private-report]: https://github.com/lennrt/tempestkeep/security/advisories/new
[json-format]: https://github.com/ossf/best-practices-badge/blob/main/docs/bestpractices-json.md
