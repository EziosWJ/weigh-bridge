package notification

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/EziosWJ/weighbridge/server/internal/audit"
	"gorm.io/gorm"
)

const (
	ScopePublic   = "PUBLIC"
	ScopeInternal = "INTERNAL"
	Draft         = "DRAFT"
	Published     = "PUBLISHED"
	Withdrawn     = "WITHDRAWN"
)

var ErrAnnouncementNotFound = err("公告已失效或不存在")

type Announcement struct {
	ID          int64      `json:"id"`
	Title       string     `json:"title"`
	Content     string     `json:"content"`
	Scope       string     `json:"scope"`
	Status      string     `json:"status"`
	PublisherID int64      `json:"-"`
	PublishTime *time.Time `json:"publishTime"`
	ExpiresAt   *time.Time `json:"expiresAt"`
	CreateTime  time.Time  `json:"createTime"`
}

func (Announcement) TableName() string { return "sys_announcement" }

type AnnouncementInput struct {
	Title     string     `json:"title"`
	Content   string     `json:"content"`
	Scope     string     `json:"scope"`
	ExpiresAt *time.Time `json:"expiresAt"`
}
type AnnouncementPage struct {
	Records  []Announcement `json:"records"`
	Total    int64          `json:"total"`
	Page     int            `json:"page"`
	PageSize int            `json:"pageSize"`
}
type announcementStore interface {
	AnnouncementPage(context.Context, PageQuery, string, bool) (AnnouncementPage, error)
	AnnouncementFind(context.Context, int64, string) (*Announcement, error)
	SaveAnnouncement(context.Context, int64, int64, AnnouncementInput) (*Announcement, error)
	TransitionAnnouncement(context.Context, int64, int64, string) (*Announcement, error)
	NextExpiry(context.Context, string) (*time.Time, error)
}

func (s *Service) AnnouncementPage(ctx context.Context, actor int64, q PageQuery, scope string, admin bool) (AnnouncementPage, error) {
	if admin {
		if err := s.requireAdmin(ctx, actor); err != nil {
			return AnnouncementPage{}, err
		}
	}
	q.Page, q.PageSize = normalizePage(q.Page, q.PageSize)
	page, err := s.announcements.AnnouncementPage(ctx, q, scope, admin)
	if page.Records == nil {
		page.Records = []Announcement{}
	}
	return page, err
}
func (s *Service) AnnouncementFind(ctx context.Context, id int64, scope string) (*Announcement, error) {
	return s.announcements.AnnouncementFind(ctx, id, scope)
}
func (s *Service) SaveAnnouncement(ctx context.Context, actor, id int64, in AnnouncementInput) (*Announcement, error) {
	if err := s.requireAdmin(ctx, actor); err != nil {
		return nil, err
	}
	in.Title, in.Content = strings.TrimSpace(in.Title), strings.TrimSpace(in.Content)
	if in.Title == "" || in.Content == "" || utf8.RuneCountInString(in.Title) > 200 || len(in.Content) > 100000 || (in.Scope != ScopePublic && in.Scope != ScopeInternal) || (in.ExpiresAt != nil && !in.ExpiresAt.After(time.Now())) {
		return nil, ErrInvalid
	}
	return s.announcements.SaveAnnouncement(ctx, actor, id, in)
}
func (s *Service) TransitionAnnouncement(ctx context.Context, actor, id int64, status string) error {
	if err := s.requireAdmin(ctx, actor); err != nil {
		return err
	}
	a, err := s.announcements.TransitionAnnouncement(ctx, actor, id, status)
	if err == nil {
		s.hub.Announce(a.Scope)
	}
	return err
}
func (s *Service) requireAdmin(ctx context.Context, actor int64) error {
	ok, err := s.store.IsAdmin(ctx, actor)
	if err != nil {
		return err
	}
	if !ok {
		return ErrForbidden
	}
	return nil
}

func visibleAnnouncements(db *gorm.DB, scope string) *gorm.DB {
	return db.Where("status = ? AND scope = ? AND (expires_at IS NULL OR expires_at > ?)", Published, scope, time.Now().UTC())
}
func (r *Repository) AnnouncementPage(ctx context.Context, q PageQuery, scope string, admin bool) (AnnouncementPage, error) {
	out := AnnouncementPage{Page: q.Page, PageSize: q.PageSize}
	db := r.db.WithContext(ctx).Model(&Announcement{})
	if !admin {
		db = visibleAnnouncements(db, scope)
	}
	if err := db.Count(&out.Total).Error; err != nil {
		return out, err
	}
	err := db.Order("id DESC").Offset((q.Page - 1) * q.PageSize).Limit(q.PageSize).Find(&out.Records).Error
	return out, err
}
func (r *Repository) AnnouncementFind(ctx context.Context, id int64, scope string) (*Announcement, error) {
	var out Announcement
	err := visibleAnnouncements(r.db.WithContext(ctx), scope).Where("id = ?", id).Take(&out).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrAnnouncementNotFound
	}
	return &out, err
}
func (r *Repository) SaveAnnouncement(ctx context.Context, actor, id int64, in AnnouncementInput) (*Announcement, error) {
	a := Announcement{ID: id, Title: in.Title, Content: in.Content, Scope: in.Scope, Status: Draft, PublisherID: actor, ExpiresAt: in.ExpiresAt, CreateTime: time.Now().UTC()}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if id == 0 {
			if err := tx.Create(&a).Error; err != nil {
				return err
			}
		} else {
			result := tx.Model(&Announcement{}).Where("id = ? AND status = ?", id, Draft).Updates(map[string]any{"title": in.Title, "content": in.Content, "scope": in.Scope, "expires_at": in.ExpiresAt})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return ErrInvalid
			}
			if err := tx.First(&a, id).Error; err != nil {
				return err
			}
		}
		return audit.RecordOn(ctx, tx, audit.Event{Action: "announcement.save", Resource: "announcement", ResourceID: a.ID, Summary: "保存公告草稿", Metadata: audit.Metadata{ActorID: actor}})
	})
	return &a, err
}
func (r *Repository) TransitionAnnouncement(ctx context.Context, actor, id int64, status string) (*Announcement, error) {
	var a Announcement
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		expected := Draft
		changes := map[string]any{"status": status}
		where := tx.Model(&Announcement{}).Where("id = ?", id)
		if status == Published {
			now := time.Now().UTC()
			changes["publish_time"], changes["publisher_id"] = now, actor
			where = where.Where("expires_at IS NULL OR expires_at > ?", now)
		} else if status == Withdrawn {
			expected = Published
		} else {
			return ErrInvalid
		}
		result := where.Where("status = ?", expected).Updates(changes)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrInvalid
		}
		if err := tx.First(&a, id).Error; err != nil {
			return err
		}
		return audit.RecordOn(ctx, tx, audit.Event{Action: "announcement." + strings.ToLower(status), Resource: "announcement", ResourceID: id, Summary: "变更公告发布状态", Metadata: audit.Metadata{ActorID: actor}})
	})
	return &a, err
}
func (r *Repository) NextExpiry(ctx context.Context, scope string) (*time.Time, error) {
	var a Announcement
	err := visibleAnnouncements(r.db.WithContext(ctx), scope).Where("expires_at IS NOT NULL").Order("expires_at ASC").First(&a).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return a.ExpiresAt, err
}
