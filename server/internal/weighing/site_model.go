package weighing

type Site struct {
	ServerURL  string `json:"serverUrl"`
	Version    int64  `json:"version"`
	ID         int64  `json:"id" gorm:"primaryKey;autoIncrement:false"`
	Code       string `json:"code"`
	Name       string `json:"name"`
	Enabled    int    `json:"enabled"`
	CanOperate bool   `json:"canOperate" gorm:"->"`
}

func (Site) TableName() string { return "wb_site" }

type SiteGrant struct {
	UserID     int64 `json:"-"`
	SiteID     int64 `json:"siteId"`
	CanOperate int   `json:"canOperate"`
}

func (SiteGrant) TableName() string { return "wb_user_site" }

const siteScopeKey = "weighing.siteScope"

type siteScope struct {
	FilterSiteID  int64
	WriteSiteID   int64
	Read, Operate []int64
	ScaleID       int64
}
