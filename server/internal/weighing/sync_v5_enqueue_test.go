package weighing

import (
	"encoding/json"
	"testing"
)

type digestVectorTicket struct {
	syncChangeV4
	Ticket *syncTicketV5 `json:"ticket"`
}

// TestV5VectorsWithoutDB re-parses the vector file through the wire shape that
// enqueueTicketChangeV5 produces, proving the payload carries the detail list.
func TestV5VectorWireShape(t *testing.T) {
	var wrapper struct {
		Vectors []struct {
			Name   string          `json:"name"`
			Change json.RawMessage `json:"change"`
		} `json:"vectors"`
	}
	data := []byte(`{"vectors":[{"name":"multi","change":{"schemaVersion":5,"ticket":{"netAdjustments":[{"mode":"percent","amount":"0.2","rateBp":100,"category":"WATER","categoryName":"水分大","remark":"水分大"},{"mode":"amount","amount":"0.5","rateBp":null,"category":"IMPURITY","categoryName":"杂质多","remark":""}]}}}]}`)
	if err := json.Unmarshal(data, &wrapper); err != nil {
		t.Fatal(err)
	}
	var payload digestVectorTicket
	if err := json.Unmarshal(wrapper.Vectors[0].Change, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Ticket == nil || len(payload.Ticket.NetAdjustments) != 2 {
		t.Fatalf("wire shape: %+v", payload.Ticket)
	}
	if payload.Ticket.NetAdjustments[0].RateBP == nil || *payload.Ticket.NetAdjustments[0].RateBP != 100 {
		t.Fatalf("item0: %+v", payload.Ticket.NetAdjustments[0])
	}
	if payload.Ticket.NetAdjustments[1].RateBP != nil {
		t.Fatalf("item1: %+v", payload.Ticket.NetAdjustments[1])
	}
}
