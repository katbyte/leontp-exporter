# Changelog

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
