package weighing

import "time"

// V6 adds the actual weighing times and explicit manual-supplement origin to
// the v5 ticket digest. Existing queued payloads retain their original version.
const (
	SyncSchemaVersionV6 = 6
	SyncDigestVersion4  = 4
)

type syncManualSupplementV6 struct {
	EntryType       string    `json:"entryType"`
	FirstWeighedAt  time.Time `json:"firstWeighedAt"`
	SecondWeighedAt time.Time `json:"secondWeighedAt"`
}

func computeChangeDigestV6(c syncChangeV4, items []syncNetAdjustmentItemV5, manual syncManualSupplementV6) (string, error) {
	return computeChangeDigestV5OrV6(c, items, &manual)
}
