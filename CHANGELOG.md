# Changelog

## Unreleased

Not released yet: it is built against a go-kt package, `metrics/page`, that no go-kt release has.

### Added

- `serve`: asks a LeoNTP time server how it is doing each time the page is read and serves the answers at `/metrics` in the Prometheus text format, with a health probe at `/healthz`. It only ever asks, over the NTP port
- `print`: asks the time server once and prints the page, as a check that the address is right and the server answers
- the readings: whether the server answered, its serial number and firmware version, how long it has been up, NTP and `ntpq` requests answered, how long the GPS has been locked, satellites in use, the status byte the maker does not document, and from the ordinary NTP answer the stratum, leap second warning, precision, when the clock was last set, how far it is from the host's clock and how long the answer took
- `--labels` puts labels of your choosing on every reading, `hostname=ntp,site=home` say
- a server that does not answer is asked once more, then reported as `leontp_up` 0 with the readings it did not give left off the page
- a LeoNTP 1200 is asked in the 48 bytes it answers; `--short-question` asks in the 8 bytes the maker's script sends, for the older unit
- every setting is a flag, a `LEONTP_METRICS_*` environment variable, or a line in a `.leontp-metrics` file
- `sdk/leontp`: a Go client for the two questions, with nothing beneath it but the standard library
- `api-defs`: the statistics question and its answer laid out as the maker's own script has them, which is the only description there is, and what a 1200 does that the script does not say
- an Alpine Docker image at `ghcr.io/katbyte/leontp-metrics`, serving on port 9123, plus `docker-compose.yml`
