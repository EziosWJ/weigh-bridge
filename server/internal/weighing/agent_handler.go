package weighing

import (
	"errors"

	"github.com/EziosWJ/weighbridge/server/internal/agentbridge"
	"github.com/gin-gonic/gin"
)

func (h *Handler) kickAgent(c *gin.Context) {
	id := c.Param("code")
	var in struct {
		ConnectionID string `json:"connectionId"`
	}
	if c.ShouldBindJSON(&in) != nil || in.ConnectionID == "" {
		writeResult(c, nil, ErrInvalid)
		return
	}
	if h.service.devices == nil {
		writeResult(c, nil, ErrConditions)
		return
	}
	e := requestEvent(c)
	e.Action = "agent.kick.requested"
	e.Resource = "weighing.agent"
	e.Summary = string(snapshot(map[string]any{"agentId": h.service.devices.State(id).AgentID, "connectionId": in.ConnectionID}))
	// Audit the operator's intent before issuing an irreversible process exit.
	if err := h.service.RecordAgentKick(c, e); err != nil {
		writeResult(c, nil, err)
		return
	}
	result, err := h.service.devices.Kick(c, id, in.ConnectionID)
	if errors.Is(err, agentbridge.ErrConnectionChanged) {
		err = ErrConflict
	}
	writeResult(c, result, err)
}
