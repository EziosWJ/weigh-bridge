package agentbridge_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	bridge "github.com/EziosWJ/weighbridge/server/internal/agentbridge"
	p "github.com/EziosWJ/weighbridge/server/internal/agentproto"
	"golang.org/x/net/websocket"
)

func helloKeyless(t *testing.T, conn *websocket.Conn) {
	t.Helper()
	if err := websocket.JSON.Send(conn, p.NewVersion(p.ProtocolVersion, p.TypeHello, bridge.ID(), "hello", p.Hello{ProtocolVersion: p.ProtocolVersion, Capabilities: append([]string{}, p.FormalCaptureCapabilities...)})); err != nil {
		t.Fatal(err)
	}
	if ack := receiveMessage(t, conn); ack.Type != p.TypeHelloAck {
		t.Fatalf("hello ack missing: %+v", ack)
	}
}

func TestCommandTimeoutLateReplyAndDisconnect(t *testing.T) {
	hub, server := keylessServer(t, func(context.Context) (bridge.Config, error) {
		return bridge.Config{ID: "agent-01", SiteID: 1, ScaleID: 1}, nil
	})
	conn := dialKeyless(t, server)
	helloKeyless(t, conn)

	// A timeout leaves no pending request that a late reply could satisfy.
	timedOut := make(chan error, 1)
	go func() {
		_, err := hub.Command(context.Background(), 1, p.TypeMeasurementBeginRequest, p.MeasurementBeginRequest{CaptureID: "capture-1"}, 30*time.Millisecond)
		timedOut <- err
	}()
	first := receiveMessage(t, conn)
	select {
	case err := <-timedOut:
		if !errors.Is(err, bridge.ErrUnavailable) {
			t.Fatalf("timeout error=%v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("command did not time out")
	}
	late := p.NewVersion(p.ProtocolVersion, p.TypeMeasurementBeginResult, bridge.ID(), first.RequestID, p.MeasurementBeginResult{Accepted: true, Generation: "late-generation"})
	if err := websocket.JSON.Send(conn, late); err != nil {
		t.Fatal(err)
	}

	// A late reply for the expired request must not complete a later command.
	secondResult := make(chan p.Message, 1)
	secondErr := make(chan error, 1)
	go func() {
		message, err := hub.Command(context.Background(), 1, p.TypeMeasurementEndRequest, p.MeasurementEndRequest{Generation: "gen-2"}, time.Second)
		if err != nil {
			secondErr <- err
			return
		}
		secondResult <- message
	}()
	second := receiveMessage(t, conn)
	if second.RequestID == first.RequestID {
		t.Fatal("request IDs were reused")
	}
	if err := websocket.JSON.Send(conn, late); err != nil {
		t.Fatal(err)
	}
	select {
	case <-secondResult:
		t.Fatal("late reply satisfied a later command")
	case err := <-secondErr:
		t.Fatalf("later command failed early: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	reply := p.MeasurementEndResult{Verified: true, Generation: "gen-2", EndedAt: time.Now().UTC()}
	if err := websocket.JSON.Send(conn, p.NewVersion(p.ProtocolVersion, p.TypeMeasurementEndResult, bridge.ID(), second.RequestID, reply)); err != nil {
		t.Fatal(err)
	}
	select {
	case message := <-secondResult:
		var got p.MeasurementEndResult
		if err := json.Unmarshal(message.Payload, &got); err != nil || !got.Verified || got.Generation != "gen-2" {
			t.Fatalf("correlated reply: %+v %v", got, err)
		}
	case err := <-secondErr:
		t.Fatalf("later command failed: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("correlated reply was not delivered")
	}

	// A disconnected Agent never leaves a command hanging.
	_ = conn.Close()
	deadline := time.Now().Add(2 * time.Second)
	for hub.State(1).ConnectionID != "" && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if hub.State(1).ConnectionID != "" {
		t.Fatal("connection did not close")
	}
	if _, err := hub.Command(context.Background(), 1, p.TypeMeasurementVerifyRequest, p.MeasurementVerifyRequest{Generation: "gen-2"}, 200*time.Millisecond); !errors.Is(err, bridge.ErrUnavailable) {
		t.Fatalf("disconnected command error=%v", err)
	}
}

func TestOldClientWithoutMeasurementCapabilityIsRejected(t *testing.T) {
	hub, server := keylessServer(t, func(context.Context) (bridge.Config, error) {
		return bridge.Config{ID: "agent-old", SiteID: 1, ScaleID: 1}, nil
	})
	conn := dialKeyless(t, server)
	if err := websocket.JSON.Send(conn, p.NewVersion(p.ProtocolVersion, p.TypeHello, bridge.ID(), "hello", p.Hello{ProtocolVersion: p.ProtocolVersion, AgentVersion: "old"})); err != nil {
		t.Fatal(err)
	}
	if ack := receiveMessage(t, conn); ack.Type != p.TypeHelloAck {
		t.Fatalf("hello ack missing: %+v", ack)
	}
	if err := hub.RequireCapability(1, p.CapabilityMeasurementWindow); !errors.Is(err, bridge.ErrCapabilityMissing) {
		t.Fatalf("old client was allowed to fall back: %v", err)
	}
}
