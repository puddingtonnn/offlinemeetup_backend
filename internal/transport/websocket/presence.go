package websocket

import (
	"encoding/json"

	"github.com/puddingtonnn/offlinemeetup_backend/internal/service"
)

// presenceEvent builds a userOnline / userOffline WS frame. displayName is
// the subject's display name. lastSeen is a unix timestamp, set only for
// offline events.
func presenceEvent(eventType string, userID int64, online bool, displayName string, lastSeen *int64) []byte {
	payload, _ := json.Marshal(WSPresencePayload{UserID: userID, Online: online, DisplayName: displayName, LastSeen: lastSeen})
	data, _ := json.Marshal(WSEvent{Type: eventType, Payload: payload})
	return data
}

// presenceSnapshotEvent builds the presenceSnapshot frame from service statuses.
func presenceSnapshotEvent(statuses []service.PresenceStatus) []byte {
	users := make([]WSPresencePayload, 0, len(statuses))
	for _, st := range statuses {
		var ls *int64
		if st.LastSeen != nil {
			v := st.LastSeen.Unix()
			ls = &v
		}
		users = append(users, WSPresencePayload{UserID: st.UserID, Online: st.Online, DisplayName: st.DisplayName, LastSeen: ls})
	}
	payload, _ := json.Marshal(WSPresenceSnapshotPayload{Users: users})
	data, _ := json.Marshal(WSEvent{Type: EventPresenceSnapshot, Payload: payload})
	return data
}
