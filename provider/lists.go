package provider

import (
	"context"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
)

// changeWatchlist adds titles to Simkl's "plan to watch" list (to set) or
// removes them from the user's lists (to empty) in one request. Simkl has no
// separate favorites concept; plan to watch is its watchlist. Like the
// built-in provider, every title the request carried counts as applied.
func (s *Server) changeWatchlist(ctx context.Context, acct account, events []*pluginv1.WatchSyncEvent, path, to string, results map[string]*pluginv1.WatchSyncApplyResult) *pluginv1.WatchSyncFault {
	var payload simklListPayload
	sent := make([]string, 0, len(events))
	for _, event := range events {
		ids := localItemIDs(event)
		if ids == (simklIDs{}) {
			results[event.GetEventId()] = rejected(event.GetEventId(), invalidRequestFault("Simkl needs an IMDb, TMDB, or TVDB id for this title"))
			continue
		}
		ref := simklListItem{To: to, IDs: ids}
		switch event.GetMedia().GetMediaType() {
		case pluginv1.WatchSyncMediaType_WATCH_SYNC_MEDIA_TYPE_MOVIE:
			payload.Movies = append(payload.Movies, ref)
		case pluginv1.WatchSyncMediaType_WATCH_SYNC_MEDIA_TYPE_SERIES:
			payload.Shows = append(payload.Shows, ref)
		default:
			results[event.GetEventId()] = rejected(event.GetEventId(), invalidRequestFault("Simkl lists hold movies and series only"))
			continue
		}
		sent = append(sent, event.GetEventId())
	}
	if len(sent) == 0 {
		return nil
	}
	if _, fault := s.simkl.post(ctx, acct, path, payload, nil); fault != nil {
		return fault
	}
	for _, eventID := range sent {
		results[eventID] = applied(eventID)
	}
	return nil
}

// localItemIDs returns the ids a list or rating item is sent to Simkl with: its
// own external ids, falling back to the id its provider item key encodes.
func localItemIDs(event *pluginv1.WatchSyncEvent) simklIDs {
	external := event.GetMedia().GetExternalIds()
	ids := idsFromLocal(external["imdb"], external["tmdb"], external["tvdb"])
	if ids == (simklIDs{}) {
		ids = idsFromProviderItemKey(event.GetProviderItemKey())
	}
	return ids
}
