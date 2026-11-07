package weighing

import (
	"strings"

	"gorm.io/gorm"
)

// NetAdjustmentItem is one named deduction or uplift within a ticket. Mode is
// "amount" or "percent"; AmountMilli is the signed resolved quantity in
// milli-tonnes (positive deducts), RateBP keeps the entered ratio for percent
// items and is nil for amount items. Category and CategoryName are the
// dictionary snapshot at issuance; Remark is optional.
type NetAdjustmentItem struct {
	Mode         string `json:"mode"`
	AmountMilli  int64  `json:"amountMilli"`
	RateBP       *int   `json:"rateBp,omitempty"`
	Category     string `json:"category"`
	CategoryName string `json:"categoryName"`
	Remark       string `json:"remark,omitempty"`
}

// resolveNetAdjustmentItems validates each operator entry against the shared
// pre-adjustment net basis, resolves percent entries independently (each
// rounds half away from zero to 0.001 吨) and returns the detail list plus the
// signed total. Upward entries are accepted only when reopening a void ticket.
// Item category names come from ResolvedCategoryName, filled by
// resolveItemsCategoryNames before this runs.
func resolveNetAdjustmentItems(in []NetAdjustmentInput, base Tonnes, allowUpward bool, resolveName func(string) (string, error)) ([]NetAdjustmentItem, WeightDifference, error) {
	items := []NetAdjustmentItem{}
	total := int64(0)
	for _, entry := range in {
		amount, mode, rateBP, err := resolveNetAdjustment(&entry, base, allowUpward)
		if err != nil {
			return nil, 0, err
		}
		if amount == 0 {
			continue
		}
		item := NetAdjustmentItem{Mode: mode, AmountMilli: int64(amount), RateBP: rateBP, Category: strings.TrimSpace(entry.Category), CategoryName: strings.TrimSpace(entry.ResolvedCategoryName), Remark: strings.TrimSpace(entry.Remark)}
		if resolveName != nil && item.Category != "" {
			name, err := resolveName(item.Category)
			if err != nil {
				return nil, 0, err
			}
			item.CategoryName = name
		}
		items = append(items, item)
		total += int64(amount)
	}
	if len(items) == 0 {
		return nil, 0, nil
	}
	if base < 0 {
		return nil, 0, ErrWeight
	}
	// The lower bound holds here; the upper bound (net below final gross) is
	// enforced by makeTicket against the actual gross, since an upward
	// correction may legitimately exceed the pre-adjustment basis.
	net := int64(base) - total
	if net <= 0 {
		return nil, 0, ErrWeight
	}
	return items, WeightDifference(total), nil
}

// itemsTotal is the signed sum of resolved detail amounts.
func itemsTotal(items []NetAdjustmentItem) WeightDifference {
	total := int64(0)
	for _, item := range items {
		total += item.AmountMilli
	}
	return WeightDifference(total)
}

// resolveItemsCategoryNames validates and snapshots each item's category
// label from the dictionary. It fills ResolvedCategoryName so the shared
// item resolution path carries the frozen name onto the ticket.
func resolveItemsCategoryNames(tx *gorm.DB, entries NetAdjustmentsInput) error {
	for i := range entries {
		if !entries[i].Requested() || entries[i].Category == "" {
			continue
		}
		name, err := resolveNetAdjustmentCategory(tx, &entries[i])
		if err != nil {
			return err
		}
		entries[i].ResolvedCategoryName = name
	}
	return nil
}
