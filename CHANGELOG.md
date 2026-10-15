# Changelog

## 0.2.0 (2026-10-15)

The same program under the name nearly every Prometheus exporter goes by: leontp-metrics is now leontp-exporter. What it reports is unchanged, but a setup made for 0.1 has to be changed to run it.

- **The program, image and formula are renamed**: `leontp-exporter`, `ghcr.io/katbyte/leontp-exporter` and `brew install katbyte/tap/leontp-exporter`. The old image and formula stay as they were, at 0.1.1.
- **Every setting is renamed with it**: `LEONTP_METRICS_SERVER` is now `LEONTP_EXPORTER_SERVER`, and the same for the rest. The settings file is now `.leontp-exporter`. The old names are no longer read.
- **The readings keep their names, but for the one saying which version wrote the page**: `leontp_metrics_build_info` is now `leontp_exporter_build_info`, named for the program as Prometheus's own tools name theirs. It comes from go-kt 0.9.0, which this is now built on.

## 0.1.1 (2026-10-12)

The same program as 0.1.0: only what is written down about the time server has changed.

- **The statistics question is the only one a 1200 has.** All 256 request codes were asked and only the one the SDK already asks gives a true reading, so the page holds everything the time server has to say about itself. Set out in [`api-defs`](api-defs/README.md).

## 0.1.0 (2026-10-12)

The first release: a metrics page for a LeoNTP GPS time server, and a Go package for asking one how it is doing.

- **How the time server is doing, as a page Prometheus can read**: whether it is answering, how long it has been up and locked to GPS, how many satellites it is using, how many requests it has answered, and whether it still vouches for its time.
- **Works with a LeoNTP 1200**, which the maker's own monitoring script gets no answer from.
- **`serve` keeps the page up** for a scraper to read, and `print` shows it once as a check that the time server can be reached.
- **Labels of your own on every reading**, to tell one time server from another.
- **`sdk/leontp`**: a Go package that asks a LeoNTP the same questions, for a program of your own.
- Ships as a Docker image, a Homebrew formula and ready-built programs.
