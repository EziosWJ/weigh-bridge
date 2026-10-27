package weighing

type Site struct {
	ServerURL  string `json:"serverUrl"`
	Version    int64  `json:"version"`
	Code       string `json:"code" gorm:"column:site_code;primaryKey"`
	Name       string `json:"name"`
	Enabled    int    `json:"enabled"`
	CanOperate bool   `json:"canOperate" gorm:"->"`
}

func (Site) TableName() string { return "wb_site" }

type SiteGrant struct {
	UserID     int64  `json:"-"`
	SiteCode   string `json:"siteCode"`
	CanOperate int    `json:"canOperate"`
}

const siteScopeKey = "weighing.siteScope"

type siteScope struct {
	Read, Operate []string
	ScaleCode     string
}
