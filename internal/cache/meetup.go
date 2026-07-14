package cache

import (
	"context"
	"slices"
	"time"

	"github.com/puddingtonnn/offlinemeetup_backend/internal/dto"
)

// meetupSnapshot — то, что лежит в кеше: инвариантный (одинаковый для всех) DTO
// митапа с IsMember=false и множество user_id участников. MemberIDs берётся из
// доменных участников и потому не зависит от наличия у них профиля.
type meetupSnapshot struct {
	Response  dto.MeetupResponse `json:"response"`
	MemberIDs []int64            `json:"member_ids"`
}

// MeetupCache кеширует инвариантный снапшот митапа под ключом meetup:{id} и
// накладывает per-user IsMember поверх копии при чтении.
type MeetupCache struct {
	cache   Cache
	metrics Metrics
	ttl     time.Duration
}

// NewMeetupCache создаёт MeetupCache поверх любого Cache.
func NewMeetupCache(c Cache, m Metrics, ttl time.Duration) *MeetupCache {
	return &MeetupCache{cache: c, metrics: m, ttl: ttl}
}

// Meetup возвращает митап для смотрящего userID. Инвариантный снапшот берётся из
// кеша (или load при промахе); IsMember вычисляется по memberIDs и кладётся в
// КОПИЮ ответа, чтобы не мутировать общий кешированный объект (singleflight).
// load возвращает инвариантный DTO (IsMember=false) и список user_id участников.
func (c *MeetupCache) Meetup(ctx context.Context, meetupID, userID int64, load func() (dto.MeetupResponse, []int64, error)) (*dto.MeetupResponse, error) {
	snap, err := Load(ctx, c.cache, c.metrics, "meetup", MeetupKey(meetupID), c.ttl, func() (meetupSnapshot, error) {
		resp, ids, err := load()
		if err != nil {
			return meetupSnapshot{}, err
		}
		return meetupSnapshot{Response: resp, MemberIDs: ids}, nil
	})
	if err != nil {
		return nil, err
	}

	out := snap.Response // копия значения: per-user поля не должны попасть в кеш
	out.IsMember = slices.Contains(snap.MemberIDs, userID)
	// invite_token — секрет вступления, виден только создателю. Снапшот в кеше
	// хранит полный токен (caller-invariant); прячем его на копии при чтении.
	if out.CreatorID != userID {
		out.InviteToken = ""
	}
	return &out, nil
}

// InvalidateMeetup сбрасывает кешированный снапшот митапа.
func (c *MeetupCache) InvalidateMeetup(ctx context.Context, meetupID int64) error {
	return c.cache.Del(ctx, MeetupKey(meetupID))
}
