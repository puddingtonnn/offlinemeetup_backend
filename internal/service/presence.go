package service

import (
	"context"
	"fmt"
	"slices"
	"time"
)

// presenceStore is the Redis-backed presence state this service drives. Declared
// at the consumer; implemented by cache.RedisPresenceStore.
type presenceStore interface {
	Connect(ctx context.Context, userID int64, connID string, ttl time.Duration) (becameOnline bool, err error)
	Disconnect(ctx context.Context, userID int64, connID string) (becameOffline bool, err error)
	Refresh(ctx context.Context, userID int64, ttl time.Duration) error
	SetLastSeen(ctx context.Context, userID int64, ts time.Time) error
	OnlineStatus(ctx context.Context, userIDs []int64) (map[int64]bool, error)
	LastSeen(ctx context.Context, userIDs []int64) (map[int64]time.Time, error)
}

// chatPeers resolves who should hear about a user's presence (co-chat members)
// and who belongs to a chat. Implemented by *ChatService.
type chatPeers interface {
	GetCoChatUserIDs(ctx context.Context, userID int64) ([]int64, error)
	GetChatParticipantIDs(ctx context.Context, chatID int64) ([]int64, error)
}

// profileNames resolves display names for a set of users so REST and WS
// snapshots can ship a name alongside each user_id. Declared at the consumer;
// implemented by *repo.ProfileRepo.
type profileNames interface {
	DisplayNamesByUserIDs(ctx context.Context, ids []int64) (map[int64]string, error)
}

// PresenceStatus is one user's presence, returned for REST and WS snapshots.
// LastSeen is set only when the user is offline and a timestamp is known.
type PresenceStatus struct {
	UserID      int64
	Online      bool
	DisplayName string
	LastSeen    *time.Time
}

// PresenceService owns presence policy: connection accounting (delegated to the
// store), who is notified on a transition, and presence snapshots. It does not
// know about WebSockets — the transport layer builds the events and broadcasts
// the recipients this service returns, which keeps the dependency direction
// transport->service.
type PresenceService struct {
	store presenceStore
	peers chatPeers
	names profileNames
	ttl   time.Duration
}

func NewPresenceService(store presenceStore, peers chatPeers, names profileNames, ttl time.Duration) *PresenceService {
	return &PresenceService{store: store, peers: peers, names: names, ttl: ttl}
}

// OnConnect records a new WS connection (connID). If the user just transitioned
// offline->online it returns becameOnline=true and the co-chat recipients who
// should receive a userOnline event; otherwise becameOnline=false and nil.
func (s *PresenceService) OnConnect(ctx context.Context, userID int64, connID string) (becameOnline bool, recipients []int64, err error) {
	online, err := s.store.Connect(ctx, userID, connID, s.ttl)
	if err != nil {
		return false, nil, fmt.Errorf("presence connect: %w", err)
	}
	if !online {
		return false, nil, nil
	}

	recipients, err = s.peers.GetCoChatUserIDs(ctx, userID)
	if err != nil {
		return false, nil, fmt.Errorf("presence connect recipients: %w", err)
	}
	return true, recipients, nil
}

// OnDisconnect records the loss of a WS connection (connID). If it was the
// user's last connection anywhere, it stamps last_seen and returns
// becameOffline=true with that timestamp and the recipients for a userOffline
// event. Idempotent: a connID already removed reports becameOffline=false, so a
// backpressure-drop plus a readPump exit won't double-fire.
func (s *PresenceService) OnDisconnect(ctx context.Context, userID int64, connID string) (becameOffline bool, lastSeen time.Time, recipients []int64, err error) {
	offline, err := s.store.Disconnect(ctx, userID, connID)
	if err != nil {
		return false, time.Time{}, nil, fmt.Errorf("presence disconnect: %w", err)
	}
	if !offline {
		return false, time.Time{}, nil, nil
	}

	ts := time.Now()
	if err := s.store.SetLastSeen(ctx, userID, ts); err != nil {
		return false, time.Time{}, nil, fmt.Errorf("presence set last seen: %w", err)
	}

	recipients, err = s.peers.GetCoChatUserIDs(ctx, userID)
	if err != nil {
		return false, time.Time{}, nil, fmt.Errorf("presence disconnect recipients: %w", err)
	}
	return true, ts, recipients, nil
}

// Heartbeat refreshes the TTL of a still-connected user's presence.
func (s *PresenceService) Heartbeat(ctx context.Context, userID int64) error {
	if err := s.store.Refresh(ctx, userID, s.ttl); err != nil {
		return fmt.Errorf("presence heartbeat: %w", err)
	}
	return nil
}

// StatusForChat returns presence for every member of a chat. The viewer must be
// a member, otherwise ErrForbidden.
func (s *PresenceService) StatusForChat(ctx context.Context, chatID, viewerID int64) ([]PresenceStatus, error) {
	ids, err := s.peers.GetChatParticipantIDs(ctx, chatID)
	if err != nil {
		return nil, fmt.Errorf("presence chat participants: %w", err)
	}

	if !slices.Contains(ids, viewerID) {
		return nil, ErrForbidden
	}

	return s.statuses(ctx, ids)
}

// SnapshotFor returns presence for everyone sharing a chat with userID — the
// initial state pushed to a client right after it connects.
func (s *PresenceService) SnapshotFor(ctx context.Context, userID int64) ([]PresenceStatus, error) {
	ids, err := s.peers.GetCoChatUserIDs(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("presence snapshot peers: %w", err)
	}
	return s.statuses(ctx, ids)
}

// statuses assembles online + last_seen for a set of users. last_seen is only
// attached to offline users.
func (s *PresenceService) statuses(ctx context.Context, ids []int64) ([]PresenceStatus, error) {
	online, err := s.store.OnlineStatus(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("presence online status: %w", err)
	}
	seen, err := s.store.LastSeen(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("presence last seen: %w", err)
	}
	names, err := s.names.DisplayNamesByUserIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("presence display names: %w", err)
	}

	out := make([]PresenceStatus, 0, len(ids))
	for _, id := range ids {
		st := PresenceStatus{UserID: id, Online: online[id], DisplayName: names[id]}
		if !st.Online {
			if ts, ok := seen[id]; ok {
				t := ts
				st.LastSeen = &t
			}
		}
		out = append(out, st)
	}
	return out, nil
}
