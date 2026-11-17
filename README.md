<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="assets/logo-dark.svg">
    <img src="assets/logo.svg" alt="fairshift" width="420">
  </picture>
</p>

<p align="center"><b>Fair schedules for teams with too many rules to plan by hand.</b></p>

---

fairshift is a generic, self-hosted staff scheduling tool. You describe your teams, people, contracts, opening hours and house rules: fixed openers and closers, rotating duties, staggered start times, office hours, team meetings with cover from other teams, frozen part-time shifts, and so on. fairshift then builds a schedule that:

- meets everyone's contracted hours,
- follows every hard rule,
- spreads the unpopular work (early starts, closes, weekends) fairly across people and teams,
- explains which rules conflict when no legal plan exists, and which one to relax.

It is built for anyone with schedules that are hard to plan by hand: daycares, care homes, clinics, shops, warehouses, dispatch centres.

## Quick start

With Docker (or Podman), using the released image:

```sh
docker compose up -d          # MySQL + fairshift on http://localhost:8080
```

Change the `change-me` passwords in [compose.yaml](compose.yaml) first. fairshift has no login yet, so only expose it on a trusted network.

Or run from source with Go 1.27+:

```sh
make db       # MySQL 8.4 in a container
make run      # http://127.0.0.1:8080
```

Open the app and set everything up there: **Settings** (name, language, plan cycle, opening hours), **Teams**, **Staff** (with expected hours and fixed shifts), **Activities** (opener, closer, office hour …) and **Rules**. Plans are edited in the browser and exported to Excel.

To try it with example data instead: `make demo` (replaces all data).

| Variable | Default |
|---|---|
| `FAIRSHIFT_DSN` | `fairshift:fairshift@tcp(127.0.0.1:3306)/fairshift` |
| `FAIRSHIFT_ADDR` | `127.0.0.1:8080` (`:8080` in the image) |

## Releases

Pushing a `v*` tag (or creating a GitHub release) builds and pushes `ghcr.io/staal0/fairshift` with the version tags and `latest`.

## Status

Early. Setup editors, rule editor, validator, plan editing, charts and Excel export work; an automatic solver is next.

## Stack

Go · MySQL 8 · server-rendered UI (html/template) · English and Danish · Excel export with live formulas.

## License

[MIT](LICENSE)
