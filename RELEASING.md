# Releasing Soro

The first intended tag is `v0.1.0`. Do not create it until the checklist below
passes on the release commit and CI.

## Checklist

1. Confirm `CHANGELOG.md` describes user-visible changes and migrations.
2. Confirm generated applications compile without a local `replace` directive
   when pointed at the intended module version.
3. Run formatting, PostgreSQL tests with coverage, race detection, vet,
   staticcheck, and govulncheck. If a local ARM64 kernel cannot start
   ThreadSanitizer because of its VMA layout, require the amd64 CI race job to
   pass; do not lower host ASLR settings.
4. Run the benchmark suite and compare it with the previous recorded release;
   investigate material regressions rather than enforcing unstable thresholds.
5. Build the CLI with an injected version and verify it:

   ```sh
   go build -ldflags "-X main.version=0.1.0" -o ./dist/soro ./cmd/soro
   ./dist/soro version
   ```

6. Smoke-test `soro new`, resource generation, database migration, server boot,
   routes, jobs, seeds, and OpenAPI generation against PostgreSQL 17.
7. Tag `v0.1.0`, publish checksums/binaries, and verify Go module discovery.
8. Start the next `Unreleased` changelog section.

Release artifacts must not contain credentials, `.env` files, coverage profiles,
or local `replace` directives.
