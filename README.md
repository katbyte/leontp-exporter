# leontp-metrics - what a LeoNTP time server says about itself, as a Prometheus metrics page

[![GitHub release](https://img.shields.io/github/v/release/katbyte/leontp-metrics?color=blueviolet)](https://github.com/katbyte/leontp-metrics/releases/latest)
[![Go Version](https://img.shields.io/github/go-mod/go-version/katbyte/leontp-metrics?color=00ADD8)](https://github.com/katbyte/leontp-metrics/blob/main/go.mod)
[![License](https://img.shields.io/github/license/katbyte/leontp-metrics?color=blue)](https://github.com/katbyte/leontp-metrics/blob/main/LICENSE)
![build](https://github.com/katbyte/leontp-metrics/actions/workflows/build.yaml/badge.svg)
![tests](https://github.com/katbyte/leontp-metrics/actions/workflows/pr-tests.yaml/badge.svg)
![lint](https://github.com/katbyte/leontp-metrics/actions/workflows/pr-golangci-lint.yaml/badge.svg)
[![coverage](https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/katbyte/leontp-metrics/badges/coverage.json)](https://github.com/katbyte/leontp-metrics/actions/workflows/coverage.yaml)

> **Not released yet.** The image and the Homebrew formula below do not exist until the first release is tagged.

One small program that asks a [LeoNTP](https://www.leontp.com/1200/manuals/) GPS time server how it is doing and reports the answers as a [Prometheus](https://prometheus.io) metrics page: how long it has been up and locked to GPS, how many satellites it is using, how many requests it has answered, and the stratum and leap warning it gives its clients.

It only ever asks the server questions, and only over its NTP port. Written for and tested against a LeoNTP 1200 on firmware 2.07.

### What else is in the box

- **A Go SDK.** [`sdk/leontp`](sdk/leontp) asks the two questions and reads the answers, with nothing beneath it but the standard library.
- **What is known about the protocol.** [`api-defs`](api-defs) lays out the question and the answer as the maker's own script, the only description there is, has them, and what a 1200 does that the script does not say.

## What the box answers

A 1200 has no web page, no SNMP and no `ntpq` support. It listens on UDP port 123 and answers two things there:

- **The ordinary NTP question**, which gives its stratum, leap warning, precision and the time.
- **A question of the maker's own for its statistics**: uptime, requests answered, GPS lock time, satellites in use, serial number and firmware version. It is what the maker's [Python stats script](https://www.leontp.com/firmware/) asks.

A 1200 answers a question only when it is exactly 48 bytes. The maker's script sends 8, written for the older unit, and gets nothing back from a 1200; neither does anything built on it.

## What is on the page

Every reading starts `leontp_`.

| Reading | Labels | What it says |
|---|---|---|
| `leontp_up` | | 1 when the time server answered both questions, 0 when not |
| `leontp_device_info` | `serial`, `firmware` | always 1; the unit's serial number and firmware version |
| `leontp_uptime_seconds` | | how long it has been powered on |
| `leontp_ntp_requests_total` | | NTP requests answered since it was powered on; a scrape is not counted as one |
| `leontp_control_requests_total` | | `ntpq` questions answered since it was powered on; a 1200 answers none |
| `leontp_gps_lock_seconds` | | how long the GPS has been locked; it starts again from 0 each time the fix is lost and found |
| `leontp_gps_satellites` | | satellites the GPS receiver is using |
| `leontp_status_flags` | | a status byte the maker does not document, as a number: 1 on a healthy, locked unit |
| `leontp_stratum` | | the stratum it gives its clients: 1 while it has a GPS fix or is holding over from one, which the manual says it does for 24 hours, then 16 |
| `leontp_leap_indicator` | | the leap second warning it gives its clients: 0 none, 1 a second to be added, 2 one to be dropped, 3 clock not set |
| `leontp_precision_seconds` | | how fine it says its clock is; the maker's support has said this is what changes in holdover |
| `leontp_reference_timestamp_seconds` | | when it says its clock was last set |
| `leontp_clock_offset_seconds` | | its clock less the clock of the machine running leontp-metrics, with the network delay taken out |
| `leontp_round_trip_seconds` | | how long an NTP question and its answer spent on the network |
| `leontp_metrics_build_info` | `version`, `commit`, `goversion` | always 1; the version of leontp-metrics that wrote the page |

The page holds nothing but these: none of the Go runtime's or the process's own readings.

### Labels of your own

`LEONTP_METRICS_LABELS` puts labels on every reading, for a store that tells one source from another by them:

```env
LEONTP_METRICS_LABELS=hostname=ntp,site=home
```

```text
leontp_gps_satellites{hostname="ntp",site="home"} 8
```

A label named as one a reading already has (`serial`, `firmware`, `version`, `commit`, `goversion`) stops the program as it starts, saying which.

`job` and `instance` can be set this way too, but whatever scrapes the page sets its own and renames the page's to `exported_job` and `exported_instance`. To keep the page's, tell the scraper to: `honor_labels: true` in Prometheus, vmagent and Alloy's `prometheus.scrape`.

### When the server stops answering

A question that gets no answer is asked once more. After that `leontp_up` is 0 and the readings that question would have given are left off the page rather than shown as zero.

## Installation

```bash
docker pull ghcr.io/katbyte/leontp-metrics:latest
```

or

```bash
go install github.com/katbyte/leontp-metrics@latest
```

or

```bash
brew install katbyte/tap/leontp-metrics
```

## Configuration

All options can be passed as command-line flags, environment variables, or via a configuration file.

| Variable | Flag | Description |
|---|---|---|
| `LEONTP_METRICS_SERVER` | `--server` | the time server's name or address, e.g. `10.0.0.10`; port 123 unless one is given |
| `LEONTP_METRICS_LISTEN` | `--listen` | the address the page is served on, default `:9123` |
| `LEONTP_METRICS_LABELS` | `--labels` | labels to put on every reading, written `name=value,name=value` |
| `LEONTP_METRICS_TIMEOUT` | `--timeout` | how long to wait for an answer before asking once more, default `1s` |
| `LEONTP_METRICS_SHORT_QUESTION` | `--short-question` | ask for statistics in the 8 bytes the maker's script sends, for the older LeoNTP; not tested against one |
| `LEONTP_METRICS_LOG` | | log level (`WARN` default): `INFO` adds what is being reported on and when the server comes back |
| `LEONTP_METRICS_CONFIG` | `--config` | read settings from this file and no other, in place of `./.leontp-metrics` or `~/.leontp-metrics` |

### Configuration File

You can place a `.leontp-metrics` file in your home directory `~/.leontp-metrics` or in your current directory `./.leontp-metrics`. Keys match the long flag names using the `env` format, with an underscore between words:

```env
SERVER=10.0.0.10
LABELS=hostname=ntp
SHORT_QUESTION=false
```

A flag or an environment variable still wins over the file. `--config` (or `LEONTP_METRICS_CONFIG`) names the file to read, and it is then the only one read; a file that is named and is not there stops the program from starting.

## Usage

```bash
leontp-metrics print --server 10.0.0.10   # ask once and print the page: a check that the server answers
leontp-metrics serve --server 10.0.0.10   # serve the page at /metrics, with a health probe at /healthz
```

### Docker

[`docker-compose.yml`](docker-compose.yml) runs the released image with the time server's address in it:

```bash
docker compose up -d
curl -s http://127.0.0.1:9123/metrics
```

The example publishes the port on `127.0.0.1` only, since the page needs no password and whatever scrapes it usually runs on the same host.

Check that a container on the host can reach the time server before relying on it: `docker run --rm -e LEONTP_METRICS_SERVER=10.0.0.10 ghcr.io/katbyte/leontp-metrics print`. If that prints `leontp_up 0` while the same command outside a container works, run the container with `network_mode: host` and `LEONTP_METRICS_LISTEN=127.0.0.1:9123`.

### Scraping it

Grafana Alloy:

```alloy
prometheus.scrape "leontp" {
  targets         = [{"__address__" = "127.0.0.1:9123"}]
  job_name        = "leontp"
  scrape_interval = "30s"
  forward_to      = [prometheus.remote_write.default.receiver]
}
```

Prometheus, vmagent or VictoriaMetrics' own scraper:

```yaml
scrape_configs:
  - job_name: leontp
    scrape_interval: 30s
    static_configs:
      - targets: ['127.0.0.1:9123']
```

The time server is asked each time the page is read, so every scrape is two small questions to it.

A few things to ask of it:

```promql
leontp_up == 0                                 # the time server is not answering
leontp_stratum != 1                            # it no longer vouches for its time
resets(leontp_gps_lock_seconds[1d])            # how many times the GPS fix was lost today
leontp_gps_satellites < 5                      # the antenna has a poor view of the sky
resets(leontp_uptime_seconds[1d]) > 0          # it restarted
rate(leontp_ntp_requests_total[5m])            # requests a second
abs(leontp_clock_offset_seconds) > 0.5         # the time server and the host disagree by half a second
```

## The SDK

```go
import "github.com/katbyte/leontp-metrics/sdk/leontp"

client, err := leontp.New("10.0.0.10", leontp.Options{})
if err != nil {
	return err
}

stats, err := client.Stats(ctx) // uptime, requests answered, GPS lock time, satellites, serial, firmware
if err != nil {
	return err
}

clock, err := client.Clock(ctx) // stratum, leap warning, precision, and how far its clock is from this machine's
if err != nil {
	return err
}

fmt.Println(stats.Firmware, stats.Satellites, clock.Stratum, clock.Offset)
```

`Options` sets how long a question waits for its answer, how many times it is asked, and `ShortQuestion` for the older unit. A client keeps nothing between questions, so one can be used from many places at once.

The statistics question is the one question the maker documents. A question added later goes in a file of its own beside [`stats.go`](sdk/leontp/stats.go): its request code, what the answer holds, and a method that asks it.

## Development

```bash
make            # fmt + build
make test       # the tests, under the race detector
make check-all  # build + tests + every linter + depscheck
```

The tests need no time server: each runs against one the test makes up, which sends back what a real 1200 on firmware 2.07 sent. The whole page is kept in [`lib/readings/testdata/page.txt`](lib/readings/testdata/page.txt), so a reading that is renamed shows up as a difference there; `go test ./lib/readings -update` rewrites it once the change is meant.

Dependencies are vendored, and the dev tools are pinned in `.tools/go.mod` and the makefile; `make tools` installs them into `.tools/bin`.
