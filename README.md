# Simkl watch-provider plugin for Silo

Connects Silo profiles to [Simkl](https://simkl.com) through Silo's `watch_sync_provider.v1` plugin contract. It replaces the Simkl provider that earlier Silo releases built in, and keeps that provider's behavior so existing connections carry over.

## Capabilities

- Imports watched movies and episodes, anime included. Simkl keeps only the last watch of a title, so each title imports as one play.
- Imports resume progress for paused movies and episodes.
- Exports watched plays and unwatched titles. A play Simkl already holds at the same time, to the second, is not sent again.
- Imports and updates Simkl's plan-to-watch list as the Silo watchlist, for movies and series. Simkl has no favorites list.
- Sends live playback start, pause, and stop events.
- Imports movie and series ratings, sends series ratings, and clears movie and series ratings. See [Ratings](#ratings).

## How it reads Simkl

Each sync first reads Simkl's `/sync/activities` and then reads only the lists whose timestamps moved since the last sync, starting from the last timestamp. The first sync reads everything. The plan-to-watch list is read in full on every sync, and a title missing from it counts as removed.

Simkl returns each list in one response. The plugin reads a list once and hands it to Silo in pages of up to 100 titles. A list too large to hand over from one read, more than about 2,500 episodes or 5,000 movies, is read once more for each further part of that size. If the plan-to-watch list or a complete ratings read changes between those reads, the sync stops and the next sync starts over.

Writes are paced to one per second per profile, as Simkl requires. When Simkl rate-limits a request, the plugin waits and retries in place: one second for the per-second limit, five seconds for Simkl's short per-user write lock, at most twice. A daily quota, or a limit that persists, pauses the connection for the time Simkl gives, or one minute when it gives none.

## Ratings

Simkl rates from 1 to 10, the same scale the plugin contract uses.

Simkl keeps anime in its own lists. A rated anime entry marked as a movie imports as a movie, one marked as TV as a series, and any other or missing type as a series without its TMDB id, because that id can belong to an unrelated movie.

A ratings import reads Simkl's full movie, show, and anime rating lists whenever any of their timestamps moved. Silo treats a rating missing from the read as removed on Simkl only when the read is complete for both movies and series. It is not complete when a rated title has no usable id, or when a rated anime entry does not say whether it is a movie or a series, which Simkl's rating lists usually leave out. While an account has such anime ratings, new and changed ratings still import, but removals on Simkl do not.

Movie ratings are not sent to Simkl. Rating a movie on Simkl files it as watched, so the built-in provider sent a movie rating only after the profile had finished the movie. The plugin contract cannot express that check yet, so the plugin holds every movie rating back and Silo lists each held rating as a warning on the sync. Series ratings and rating removals of both kinds are sent.

## Setup

1. Create an app in your Simkl account's [developer settings](https://simkl.com/settings/developer/).
2. Install the plugin. In its settings, enter the app's client ID. The plugin does not use the client secret.
3. Connect each Silo profile from its watch-provider settings. Silo shows a code to enter at simkl.com/pin.

The plugin signs in with Simkl's PIN codes, part of what Simkl calls AUTH V1. Simkl plans to retire AUTH V1 around April 2027, and its PIN codes do not work with the client ID of an AUTH V2 app. If Simkl rejects the client ID for that reason, the plugin says so when a profile connects.

## Upgrading from the built-in Simkl provider

Existing Simkl connections carry over once a Silo server release that maps this plugin to the built-in `simkl` provider is installed. The plugin reuses the stored Simkl tokens, which do not expire, and produces the same item keys, so profiles do not reconnect and Silo keeps its record of what was synced. If that server release does not copy the client ID from the old Simkl server setting, enter it in the plugin's settings.

The first sync after the upgrade reads the whole Simkl library once, because the plugin tracks its read position separately from the built-in provider. Plays, progress, and ratings Silo already has are not imported twice.

Two things change from the built-in provider: movie ratings are no longer sent (see [Ratings](#ratings)), and playback events for different titles are no longer sent strictly one after another per profile. Events for the same movie or series still are.

## Not yet supported

These need a newer plugin contract:

- Syncing Simkl's dropped-shows list.
- Sending a movie rating once the profile has watched the movie.
- Sync warnings from the plugin, such as titles skipped because they have no usable id, or a notice that titles were removed from a Simkl list, which Silo does not import.
- Importing rating removals while the account has anime ratings without a movie or TV type.

Signing in through Simkl AUTH V2 is not supported yet either.

## Development

```bash
make test
make build
./plugin manifest
```

`make build-all` produces static binaries for the platforms declared in `manifest.json`.

## Contributing

Read [CONTRIBUTING.md](CONTRIBUTING.md) before opening a pull request. Changes to
authentication, reconciliation, idempotency, or the watch-sync contract should
start as an issue.

## License

AGPL-3.0-only.
