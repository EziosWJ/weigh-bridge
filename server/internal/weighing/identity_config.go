package weighing

import (
	"fmt"

	"gorm.io/gorm"
)

// saveIdentityConfigs keeps protected configuration entries and their business
// references in the same transaction. Missing entries indicate an incomplete migration.
func saveIdentityConfigs(tx *gorm.DB, siteCode, scaleCode, siteName, scaleName string) error {
	for _, item := range []struct{ key, value string }{
		{"weighing.local-site-code", siteCode}, {"weighing.local-scale-code", scaleCode},
		{"weighing.local-site-name", siteName}, {"weighing.local-scale-name", scaleName},
	} {
		result := tx.Table("sys_config").Where("config_key=? AND is_builtin=1 AND deleted=0 AND status=1", item.key).Updates(map[string]any{"config_value": item.value, "update_time": gorm.Expr("CURRENT_TIMESTAMP")})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return fmt.Errorf("本厂身份配置项 %s 缺失或不可用，请检查数据库迁移", item.key)
		}
	}
	return nil
}
