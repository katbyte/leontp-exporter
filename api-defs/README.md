# api-defs

What is publicly known about asking a LeoNTP time server how it is doing, and what a 1200 does that nothing published says.

## The documents

The maker publishes no description of the protocol. Both manuals say one thing about monitoring: "We offer a Python script to remotely monitor the unit". That script is the document, so `sdk/leontp` is written by hand and held to it.

| File | Source | What it is |
|---|---|---|
| `LeoNTP-stats-full.py` | `LeoNTP-stats.zip` on <https://www.leontp.com/firmware/>, dated August 2016 | asks once and prints every field: the description of the question and of the answer |
| `LeoNTP-stats.py` | the same zip | asks every second and feeds a graph; reads three of the fields |

Both are for Python 2 and carry no licence, so neither is kept in git: download the zip and unpack the two files here to read them beside this.

Read for this and not kept here:

- [LeoNTP 1200 instruction manual V1.0](https://www.leontp.com/1200/manuals/LeoNTP%201200%20Time%20Server%20Instruction%20Manual%20V1.0.pdf), December 2022
- [LeoNTP user guide V1.35](https://www.leontp.com/firmware/LeoNTP-InstructionManual-Latest.pdf), for the older unit
- [LBE-1200 datasheet](https://leobodnar.com/files/datasheets/LBE-1200-Datasheet-V1.0_-_Initial_Version-29-07-2025.pdf), July 2025
- [the 1200's firmware list](https://www.leontp.com/1200/manuals/) and [the same with 2.07 on it](https://store.uputronics.com/blogs/news/leontp-firmware-and-data)
- [a public unit's statistics page](http://leobodnar.com/LeoNTP/ntp1.php), which is the full script's output

## The statistics question

Sent to UDP port 123, the NTP port, which is the only one the unit listens on.

| Byte | Value | What it is |
|---|---|---|
| 0 | `0x27` | NTP version 4, mode 7: the mode NTP leaves for private use |
| 1 | `0` | sequence |
| 2 | `0x10` | implementation: the maker's own |
| 3 | `1` | request code: "just 1 for now", the script says |
| 4-7 | `0` | "unused for now" |

The script sends these 8 bytes and no more. A 1200 wants them followed by 40 zero bytes, see below.

## The statistics answer

48 bytes. What follows the first 16 is written low byte first, unlike the rest of NTP.

| Bytes | Script's name | What it is |
|---|---|---|
| 0 | | `0xa7`: an answer, version 4, mode 7 |
| 1-15 | | not read by the script; on a 1200 they hold `00 06 e7`, eight zero bytes and `GPS` |
| 16-19 | `ref_ts0` | the fraction of the second of the unit's clock |
| 20-23 | `ref_ts1` | the unit's clock, in seconds since 1900 |
| 24-27 | `uptime` | seconds since power on |
| 28-31 | `NTP_served` | NTP requests answered since power on |
| 32-35 | `CMD_served` | mode 6 (`ntpq`) requests answered since power on |
| 36-39 | `lock_time` | seconds the GPS has been locked |
| 40 | `flags` | "GPS flags"; the script reads it and prints nothing |
| 41 | `numSV` | satellites in use |
| 42-43 | `ser_num` | serial number |
| 44-47 | `FW_ver` | firmware version: the lowest byte is the part after the point and the next the part before, each read as written in hex, so `07 02` is 2.07 |

Captured from a 1200 on firmware 2.07, serial 416:

```text
a7 00 06 e7 00 00 00 00 00 00 00 00 47 50 53 00
d0 2f 86 67 af 06 77 ee 99 fd 0c 00 8d 64 02 00
00 00 00 00 f8 7c 00 00 01 07 a0 01 07 02 00 00
```

## What a 1200 does

Found by asking one on firmware 2.07.

- **It answers only a question of exactly 48 bytes.** The script's 8 bytes get nothing back, and neither does anything built on the script. The same holds for the ordinary NTP question: one of 52 or 68 bytes, as a client sending an extension or a signature would, is not answered.
- **A statistics question is not counted as a request.** `NTP_served` stands still while only statistics are asked for.
- **There is no `ntpq`.** A mode 6 question is not answered and `CMD_served` stays 0.
- **Request code 1 is the only one.** Every code from 0 to 255 answers, but only 1 returns a true reading: the unit keeps one answer and hands back whatever is in it, so another code gives the last stats when one was asked for just before, and half-written or stale bytes (a `flags` of `0xee`, a serial that is not the unit's) when the asking is quick and mixed. Nothing is gained by asking any code but 1, and the request's other fields (its sequence byte, its four unused bytes) change nothing in the answer. Walking all 256 codes left the unit's uptime climbing unbroken, so it minds none of them.
- **`flags` is 1 on a healthy, locked unit.** What else it can be is not known.
- **The answer's header repeats the ordinary answer's.** Bytes 2 and 3 are its poll (6) and precision (-25), bytes 12 to 15 its reference `GPS`.
- **The ordinary answer** gives stratum 1, no leap warning, precision -25 (30 nanoseconds), root delay and root dispersion 0, the reference `GPS`, and a reference time on the whole second that moves every second.
- **Nothing else is there.** No web page, no SNMP, and none of the usual TCP ports is open.

## What others have found

- **The older unit's firmware version has more in it.** A public unit's page prints `4a01.23` for firmware 1.23: the two bytes above the version are not zero there, as they are on a 1200. What they hold is not known.
- **Root dispersion is always 0, holdover included.** The maker's support said this is meant and that the precision is varied in its place (Steven Sommars, [time-nuts, November 2021](https://febo.com/pipermail/time-nuts_lists.febo.com/2021-November/104321.html)). The same post reports answers "off by N-seconds", seldom, with no cause found.
- **Stratum is 1 while locked and for 24 hours without GPS, then 16**, by both manuals.
- **The older unit answered `ntpq` on firmware 1.12 only**, by the maker's firmware list.
- **Firmware 2.07 of the 1200 fixed a leap second warning given in error**, by its one line in the firmware list.

## Other clients

All three send the script's 8 bytes, so none gets an answer from a 1200.

- [lwlcom/leontp_exporter](https://github.com/lwlcom/leontp_exporter): Go, a Prometheus exporter, archived
- [sean-foley/leo-ntp-monitor](https://github.com/sean-foley/leo-ntp-monitor): Python, to InfluxDB
- [BritishAmateurTelevisionClub/eshail-ghy-leontp](https://github.com/BritishAmateurTelevisionClub/eshail-ghy-leontp): Python, to rrd graphs

## Not known

- what each bit of `flags` means
- what the older unit keeps above its firmware version
- whether the older unit answers the 48 byte question a 1200 needs

The maker's support is the place to ask: the manual gives `orders@uputronics.com`.

## Refreshing it

1. When a new firmware is out, read its line in the firmware list and download the script again; if it changed, bring the two tables above in line with it.
2. Run `leontp-exporter print --server <address>` against a unit on the new firmware and compare with the answer above.
3. Write what changed under "What a 1200 does", with the firmware it was seen on.
