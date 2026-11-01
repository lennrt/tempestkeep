# Tasks

- [x] Record paired-row contract, compatibility, and ADR 0013 before implementation.
- [x] Add public API regressions; focused tests fail on unpaired hot/cold inputs and unpaired
  outliers before the fix.
- [x] Implement paired quarter-hour means and candidate extrema;
  `GOCACHE=/tmp/tempestkeep-agent2-cache go test ./pkg/tempest/store -count=1 -timeout=2m`
  passes.
- [x] Validate change: `npm run openspec -- validate pair-comfort-readings --strict
  --no-interactive` passes.
- [x] Run complete store race tests including paired-mean and mixed-candidate cases:
  `GOCACHE=/tmp/tempestkeep-agent2-cache go test -race ./pkg/tempest/store -count=1 -timeout=3m`
  passes.
