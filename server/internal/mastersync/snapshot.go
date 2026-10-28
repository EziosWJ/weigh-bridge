// Package mastersync validates the versioned headquarters master-data contract.
package mastersync

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

type Category struct {
	Code    string `json:"code"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}
type Material struct {
	Code         string  `json:"code"`
	Name         string  `json:"name"`
	CategoryCode *string `json:"categoryCode"`
	Spec         string  `json:"spec"`
	Remark       string  `json:"remark"`
	Enabled      bool    `json:"enabled"`
}
type Contact struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Phone     string `json:"phone"`
	Purpose   string `json:"purpose"`
	Remark    string `json:"remark"`
	IsDefault bool   `json:"isDefault"`
}
type Party struct {
	Code       string    `json:"code"`
	Name       string    `json:"name"`
	ShortName  string    `json:"shortName"`
	Nature     string    `json:"nature"`
	IsSupplier bool      `json:"isSupplier"`
	IsCustomer bool      `json:"isCustomer"`
	IsCarrier  bool      `json:"isCarrier"`
	Phone      string    `json:"phone"`
	Address    string    `json:"address"`
	CreditCode string    `json:"creditCode"`
	Remark     string    `json:"remark"`
	Enabled    bool      `json:"enabled"`
	Contacts   []Contact `json:"contacts"`
}
type Snapshot struct {
	SchemaVersion int        `json:"schemaVersion"`
	SiteCode      string     `json:"siteCode"`
	ScaleCodes    []string   `json:"scaleCodes"`
	Categories    []Category `json:"categories"`
	Materials     []Material `json:"materials"`
	Parties       []Party    `json:"parties"`
}

// Decode rejects missing and null fields as well as invalid relationships. This
// distinction is necessary because JSON zero values otherwise hide partial replies.
func Decode(body []byte, site, scale string) (Snapshot, error) {
	var empty Snapshot
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(body, &envelope); err != nil {
		return empty, errors.New("响应格式错误")
	}
	if err := fields(envelope, "code", "message", "data"); err != nil {
		return empty, err
	}
	var code int
	if json.Unmarshal(envelope["code"], &code) != nil || code != 200 {
		return empty, errors.New("云端业务响应失败")
	}
	var message string
	if json.Unmarshal(envelope["message"], &message) != nil {
		return empty, errors.New("响应格式错误")
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(envelope["data"], &obj) != nil {
		return empty, errors.New("资料格式错误")
	}
	if err := fields(obj, "schemaVersion", "siteCode", "scaleCodes", "categories", "materials", "parties"); err != nil {
		return empty, err
	}
	for key, required := range map[string][]string{
		"categories": {"code", "name", "enabled"},
		"materials":  {"code", "name", "categoryCode", "spec", "remark", "enabled"},
		"parties":    {"code", "name", "shortName", "nature", "isSupplier", "isCustomer", "isCarrier", "phone", "address", "creditCode", "remark", "enabled", "contacts"},
	} {
		var rows []map[string]json.RawMessage
		if err := json.Unmarshal(obj[key], &rows); err != nil || rows == nil {
			return empty, fmt.Errorf("%s 必须为数组", key)
		}
		for _, row := range rows {
			if err := fieldsNullableCategory(row, required...); err != nil {
				return empty, err
			}
			if key == "parties" {
				var contacts []map[string]json.RawMessage
				if err := json.Unmarshal(row["contacts"], &contacts); err != nil || contacts == nil {
					return empty, errors.New("contacts 必须为数组")
				}
				for _, c := range contacts {
					if err := fields(c, "id", "name", "phone", "purpose", "remark", "isDefault"); err != nil {
						return empty, err
					}
				}
			}
		}
	}
	var out Snapshot
	if err := json.Unmarshal(envelope["data"], &out); err != nil {
		return empty, errors.New("资料字段类型错误")
	}
	if out.SchemaVersion != 1 {
		return empty, errors.New("资料版本不兼容")
	}
	if site == "" || scale == "" || out.SiteCode != site || out.ScaleCodes == nil {
		return empty, errors.New("厂区或地磅身份不匹配")
	}
	scales := map[string]bool{}
	found := false
	for _, v := range out.ScaleCodes {
		if strings.TrimSpace(v) == "" || scales[v] {
			return empty, errors.New("地磅编码无效或重复")
		}
		scales[v] = true
		found = found || v == scale
	}
	if !found {
		return empty, errors.New("厂区或地磅身份不匹配")
	}
	categories := map[string]bool{}
	materials := map[string]bool{}
	parties := map[string]bool{}
	contacts := map[int64]bool{}
	valid := func(code, name string, seen map[string]bool) bool {
		if strings.TrimSpace(code) == "" || strings.TrimSpace(code) != code || utf8.RuneCountInString(code) > 40 || strings.TrimSpace(name) == "" || utf8.RuneCountInString(name) > 100 || seen[code] {
			return false
		}
		seen[code] = true
		return true
	}
	for _, c := range out.Categories {
		if !valid(c.Code, c.Name, categories) {
			return empty, errors.New("分类编码或名称无效")
		}
	}
	for _, m := range out.Materials {
		if !valid(m.Code, m.Name, materials) || (m.CategoryCode != nil && !categories[*m.CategoryCode]) || utf8.RuneCountInString(m.Spec) > 200 || utf8.RuneCountInString(m.Remark) > 2000 {
			return empty, errors.New("物料或分类引用无效")
		}
	}
	for _, p := range out.Parties {
		if !valid(p.Code, p.Name, parties) || (p.Nature != "enterprise" && p.Nature != "individual") || !(p.IsSupplier || p.IsCustomer || p.IsCarrier) || utf8.RuneCountInString(p.ShortName) > 100 || utf8.RuneCountInString(p.Phone) > 50 || utf8.RuneCountInString(p.Address) > 500 || utf8.RuneCountInString(p.CreditCode) > 50 || utf8.RuneCountInString(p.Remark) > 2000 || len(p.Contacts) > 100 {
			return empty, errors.New("主体资料无效")
		}
		defaults := 0
		for _, c := range p.Contacts {
			if c.ID <= 0 || contacts[c.ID] || strings.TrimSpace(c.Name) == "" || utf8.RuneCountInString(c.Name) > 100 || utf8.RuneCountInString(c.Phone) > 50 || utf8.RuneCountInString(c.Purpose) > 200 || utf8.RuneCountInString(c.Remark) > 2000 {
				return empty, errors.New("联系人无效")
			}
			contacts[c.ID] = true
			if c.IsDefault {
				defaults++
			}
		}
		if len(p.Contacts) > 0 && defaults != 1 {
			return empty, errors.New("默认联系人无效")
		}
	}
	return out, nil
}
func fields(obj map[string]json.RawMessage, names ...string) error {
	for _, n := range names {
		v, ok := obj[n]
		if !ok || string(v) == "null" {
			return fmt.Errorf("缺少字段 %s", n)
		}
	}
	return nil
}
func fieldsNullableCategory(obj map[string]json.RawMessage, names ...string) error {
	for _, n := range names {
		v, ok := obj[n]
		if !ok || (n != "categoryCode" && string(v) == "null") {
			return fmt.Errorf("缺少字段 %s", n)
		}
	}
	return nil
}
