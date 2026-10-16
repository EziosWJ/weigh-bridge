package monitoring

import (
	"context"
	"gorm.io/gorm"
)

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }
func (r *Repository) IsAdmin(ctx context.Context, userID int64) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Table("sys_user_role ur").Joins("JOIN sys_role r ON r.id=ur.role_id").Where("ur.user_id=? AND r.role_code='ADMIN' AND r.status=1 AND r.deleted=0", userID).Count(&count).Error
	return count > 0, err
}
