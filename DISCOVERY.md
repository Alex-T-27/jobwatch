# Company discovery

JobWatch searches the public [urlscan search API](https://urlscan.io/docs/api/)
for career-board URLs. It does not use another internship tracker's listings,
submit URLs to be scanned, or require an AI service.

## Run

Run these from `/home/alex-tran/project/jobwatch` after `go build ./...`.

Preview discovery only, without changing files or contacting Discord:

```sh
./job-tracker -discover-only -dry-run
```

Import verified companies into `companies.json`, without contacting Discord:

```sh
./job-tracker -discover-only
```

Normal `./job-tracker` runs discover at startup and again after 24 hours,
between job-scanning sweeps. The interval also applies after a failed attempt.
Restarting the program triggers another attempt. Use `-no-discovery` to watch
only configured companies. `-dry-run` previews discovery and one job sweep
without writing the config, scan history, or sent log.

## Limits and behavior

- Four index searches: Ashby, Lever, and both Greenhouse board hostnames.
- Each search requests at most 100 results from the last 30 days.
- At most 20 unknown boards are checked and 10 added per attempt.
- Search requests time out after 10 seconds, board validation after 5 seconds,
  and the entire discovery attempt after 45 seconds.
- Known boards are skipped, preserving their original identifier spelling and
  existing notification keys. Imports only add entries; they never remove them.
- Candidates are validated through the same vendor adapters used by polling.
  Empty boards are deferred until they have postings that can be verified.
- Search access denial or throttling stops further index queries. Vendor
  throttling stops validation. Other failed boards are skipped.
- The config is re-read before merging, then replaced atomically. Invalid edits,
  cancellation, or a deadline failure leave the config unchanged.
- Batch scans reload the config between full sweeps. Discovery failure does not
  stop scanning known companies. Existing filtering and send caps still apply.

## Limitations

This is bounded discovery, not an exhaustive crawl. Only indexed pages in the
search window can be found. Repeated or dead links can consume the request cap.
Regional API variants and unusual board identifiers are not imported yet.

An added board is a working source, not a promise of a US internship or visa
sponsorship. JobWatch still applies its per-posting filters afterward.

The public API worked without an account when verified. Anonymous access has
limited quotas and no uptime guarantee; repeated restarts can exhaust them.
See urlscan's [API guidance and quotas](https://urlscan.io/docs/api/).

Discovery runs inside the JobWatch process. It does not install a background
service. On a cloud deployment, the config and state files need persistent,
writable storage. Run one JobWatch process per config/state directory.
