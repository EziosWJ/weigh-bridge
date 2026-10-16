package notification

import (
	"context"
	"sort"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/audit"
	"gorm.io/gorm"
)

type Repository struct {
	db  *gorm.DB
	hub *Hub
}

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }
func (r *Repository) Page(ctx context.Context, userID int64, q PageQuery) (Page, error) {
	var out Page
	d := r.db.WithContext(ctx).Table("sys_notification n").Select("n.*, r.is_read").Joins("JOIN sys_notification_recipient r ON r.notification_id=n.id").Where("r.user_id=?", userID)
	if e := d.Count(&out.Total).Error; e != nil {
		return out, e
	}
	e := d.Order("n.publish_time DESC,n.id DESC").Offset((q.Page - 1) * q.PageSize).Limit(q.PageSize).Scan(&out.Records).Error
	out.Page, out.PageSize = q.Page, q.PageSize
	return out, e
}
func (r *Repository) Find(ctx context.Context, userID, id int64) (*Notification, error) {
	var n Notification
	e := r.db.WithContext(ctx).Table("sys_notification n").Select("n.*, r.is_read").Joins("JOIN sys_notification_recipient r ON r.notification_id=n.id").Where("n.id=? AND r.user_id=?", id, userID).Scan(&n).Error
	if e != nil {
		return nil, e
	}
	if n.ID == 0 {
		return nil, ErrNotFound
	}
	return &n, nil
}
func (r *Repository) UnreadCount(ctx context.Context, userID int64) (int64, error) {
	var c int64
	e := r.db.WithContext(ctx).Table("sys_notification_recipient").Where("user_id=? AND is_read=0", userID).Count(&c).Error
	return c, e
}
func (r *Repository) MarkRead(ctx context.Context, userID, id int64) error {
	result := r.db.WithContext(ctx).Model(&Recipient{}).Where("notification_id=? AND user_id=? AND is_read=0", id, userID).Updates(map[string]any{"is_read": 1, "read_time": time.Now().UTC()})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		_, err := r.Find(ctx, userID, id)
		return err
	}
	if r.hub != nil {
		r.hub.Notify([]int64{userID}, Change{Type: "read"})
	}
	return nil
}
func (r *Repository) MarkAllRead(ctx context.Context, userID int64) error {
	result := r.db.WithContext(ctx).Model(&Recipient{}).Where("user_id=? AND is_read=0", userID).Updates(map[string]any{"is_read": 1, "read_time": time.Now().UTC()})
	if result.Error == nil && result.RowsAffected > 0 && r.hub != nil {
		r.hub.Notify([]int64{userID}, Change{Type: "read"})
	}
	return result.Error
}
func (r *Repository) Publish(ctx context.Context, actor int64, in PublishInput, source string) error {
	var committed func()
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if in.AllUsers {
			if err := tx.Table("sys_user").Where("status=1 AND deleted=0").Pluck("id", &in.UserIDs).Error; err != nil {
				return err
			}
		}
		var id int64
		var err error
		id, committed, err = r.recordMessage(ctx, tx, in, source, &actor)
		if err != nil {
			return err
		}
		return audit.RecordOn(ctx, tx, audit.Event{Action: "notification.publish", Resource: "notification", ResourceID: id, Summary: "发布站内通知", Metadata: audit.Metadata{ActorID: actor}})
	})
	if err == nil && committed != nil {
		committed()
	}
	return err
}
func (r *Repository) AdminPage(ctx context.Context, q PageQuery) (Page, error) {
	var out Page
	if e := r.db.WithContext(ctx).Model(&Notification{}).Count(&out.Total).Error; e != nil {
		return out, e
	}
	d := r.db.WithContext(ctx).Table("sys_notification n").Select("n.*, COUNT(r.id) AS recipient_count, COALESCE(SUM(CASE WHEN r.is_read=1 THEN 1 ELSE 0 END), 0) AS read_count").Joins("LEFT JOIN sys_notification_recipient r ON r.notification_id=n.id").Group("n.id")
	e := d.Order("publish_time DESC,id DESC").Offset((q.Page - 1) * q.PageSize).Limit(q.PageSize).Find(&out.Records).Error
	out.Page, out.PageSize = q.Page, q.PageSize
	return out, e
}
func (r *Repository) IsAdmin(ctx context.Context, userID int64) (bool, error) {
	var c int64
	e := r.db.WithContext(ctx).Table("sys_user_role ur").Joins("JOIN sys_role r ON r.id=ur.role_id").Where("ur.user_id=? AND r.role_code='ADMIN' AND r.status=1 AND r.deleted=0", userID).Count(&c).Error
	return c > 0, e
}
func (r *Repository) RecordRoleChange(ctx context.Context, tx *gorm.DB, userID int64, before, after []string) (func(), error) {
	if len(before) == 0 && len(after) == 0 {
		return nil, nil
	}
	b, a := joinRoles(before), joinRoles(after)
	in := PublishInput{Title: "用户角色已更新", Content: "你的角色已发生变更：" + b + " → " + a, UserIDs: []int64{userID}}
	_, committed, err := r.recordMessage(ctx, tx, in, SourceRoleChange, nil)
	return committed, err
}

// RecordBusiness joins the caller's transaction. Invoke the returned callback
// only after that transaction commits successfully; discard it on rollback.
func (r *Repository) RecordBusiness(ctx context.Context, tx *gorm.DB, in PublishInput) (func(), error) {
	if in.AllUsers {
		return nil, ErrInvalid
	}
	_, committed, err := r.recordMessage(ctx, tx, in, SourceBusiness, nil)
	return committed, err
}
func (r *Repository) recordMessage(ctx context.Context, tx *gorm.DB, in PublishInput, source string, actor *int64) (int64, func(), error) {
	tx = tx.WithContext(ctx)
	for _, id := range in.UserIDs {
		if id <= 0 {
			return 0, nil, ErrInvalid
		}
	}
	in.UserIDs = uniqueIDs(in.UserIDs)
	if !validMessage(in.Title, in.Content, in.JumpPath) || len(in.UserIDs) == 0 {
		return 0, nil, ErrInvalid
	}
	var active int64
	eligible := tx.Table("sys_user").Where("id IN ? AND deleted=0", in.UserIDs)
	// Role assignment already supports disabled accounts; retain their message
	// for re-enablement, as before. Explicit sends require enabled recipients.
	if source != SourceRoleChange {
		eligible = eligible.Where("status=1")
	}
	if err := eligible.Count(&active).Error; err != nil {
		return 0, nil, err
	}
	if active != int64(len(in.UserIDs)) {
		return 0, nil, ErrInvalid
	}
	now := time.Now().UTC()
	n := Notification{Title: trim(in.Title), Content: trim(in.Content), JumpPath: in.JumpPath, SourceType: source, PublisherID: actor, PublishTime: now, CreateTime: now}
	if err := tx.WithContext(ctx).Create(&n).Error; err != nil {
		return 0, nil, err
	}
	recipients := make([]Recipient, 0, len(in.UserIDs))
	for _, id := range in.UserIDs {
		recipients = append(recipients, Recipient{NotificationID: n.ID, UserID: id})
	}
	if err := tx.WithContext(ctx).CreateInBatches(recipients, 100).Error; err != nil {
		return 0, nil, err
	}
	return n.ID, func() {
		if r.hub != nil {
			r.hub.Notify(in.UserIDs, Change{Type: "notification", ID: n.ID})
		}
	}, nil
}
func joinRoles(v []string) string {
	if len(v) == 0 {
		return "无"
	}
	sort.Strings(v)
	out := ""
	for i, s := range v {
		if i > 0 {
			out += "、"
		}
		out += s
	}
	return out
}
