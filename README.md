# SpokeSpan

SpokeSpan is a small Go command-line tool for documenting bicycle-wheel spoke tension. It keeps one versioned JSON ledger on disk and turns a complete set of readings into a balanced or adjust report.

## Requirements

- Go 1.22 or newer

## Workflow

Start a survey with its wheel configuration. The ledger path may be supplied globally or to an individual command.

```text
go run ./cmd/spokespan start --ledger wheel.json --spokes 4 --min-tension 90 --max-tension 110 --max-spread 8 --label "rear wheel"
```

The command prints the generated survey ID. Record one positive reading for every spoke number:

```text
go run ./cmd/spokespan measure --ledger wheel.json --id SURVEY_ID --spoke 1 --tension 98
go run ./cmd/spokespan measure --ledger wheel.json --id SURVEY_ID --spoke 2 --tension 101
```

Once every spoke has a reading, finalize and inspect the immutable report:

```text
go run ./cmd/spokespan finalize --ledger wheel.json --id SURVEY_ID
go run ./cmd/spokespan show --ledger wheel.json --id SURVEY_ID
```

Add `--json` to `start`, `measure`, `finalize`, or `show` for machine-readable output. The default ledger is `spokespan-ledger.json` in the current directory.

The report is balanced when every reading is within the configured inclusive tension range and the observed maximum-minus-minimum spread is no greater than the configured maximum spread. Otherwise it is marked adjust. A finalized survey rejects later readings and finalization attempts.

## Checks

```text
go test ./...
go run ./cmd/spokespan smoke
```
