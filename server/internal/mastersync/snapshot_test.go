package mastersync

import (
	"strings"
	"testing"
)

const validResponse = `{"code":200,"message":"success","data":{"schemaVersion":1,"siteCode":"F001","scaleCodes":["WB01"],"categories":[{"code":"C-C-0001","name":"谷物","enabled":true}],"materials":[{"code":"C-M-0001","name":"高粱","categoryCode":"C-C-0001","spec":"","remark":"","enabled":true}],"parties":[{"code":"C-P-0001","name":"供货方","shortName":"","nature":"enterprise","isSupplier":true,"isCustomer":false,"isCarrier":false,"phone":"","address":"","creditCode":"","remark":"","enabled":true,"contacts":[{"id":1,"name":"联系人","phone":"","purpose":"","remark":"","isDefault":true}]}]}}`

func TestDecode(t *testing.T) {
	if _, err := Decode([]byte(validResponse), "F001", "WB01"); err != nil {
		t.Fatal(err)
	}
	tests := []struct{ name, body, site, scale string }{
		{"business failure", strings.Replace(validResponse, `"code":200`, `"code":500`, 1), "F001", "WB01"},
		{"version", strings.Replace(validResponse, `"schemaVersion":1`, `"schemaVersion":2`, 1), "F001", "WB01"},
		{"site", validResponse, "F002", "WB01"},
		{"scale", validResponse, "F001", "WB02"},
		{"missing boolean", strings.Replace(validResponse, `,"enabled":true`, ``, 1), "F001", "WB01"},
		{"null list", strings.Replace(validResponse, `"categories":[{"code":"C-C-0001","name":"谷物","enabled":true}]`, `"categories":null`, 1), "F001", "WB01"},
		{"missing category", strings.Replace(validResponse, `"categoryCode":"C-C-0001"`, `"categoryCode":"missing"`, 1), "F001", "WB01"},
		{"invalid role", strings.Replace(validResponse, `"isSupplier":true`, `"isSupplier":false`, 1), "F001", "WB01"},
		{"missing default", strings.Replace(validResponse, `"isDefault":true`, `"isDefault":false`, 1), "F001", "WB01"},
		{"name too long", strings.Replace(validResponse, `"name":"谷物"`, `"name":"`+strings.Repeat("谷", 101)+`"`, 1), "F001", "WB01"},
		{"phone too long", strings.Replace(validResponse, `"phone":""`, `"phone":"`+strings.Repeat("1", 51)+`"`, 1), "F001", "WB01"},
		{"duplicate scale", strings.Replace(validResponse, `["WB01"]`, `["WB01","WB01"]`, 1), "F001", "WB01"},
		{"duplicate category", strings.Replace(validResponse, `{"code":"C-C-0001","name":"谷物","enabled":true}`, `{"code":"C-C-0001","name":"谷物","enabled":true},{"code":"C-C-0001","name":"谷物","enabled":false}`, 1), "F001", "WB01"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Decode([]byte(tt.body), tt.site, tt.scale); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}
