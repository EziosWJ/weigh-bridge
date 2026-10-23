package main

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/agentcamera"
	"github.com/EziosWJ/weighbridge/server/internal/agentmaintenance"
	"github.com/EziosWJ/weighbridge/server/internal/agentprinter"
	p "github.com/EziosWJ/weighbridge/server/internal/agentproto"
	"github.com/EziosWJ/weighbridge/server/internal/agentscale"
)

// deviceRuntime keeps control communication alive while device workers stop or
// reload. All instrument access and print execution remain in this process.
type deviceRuntime struct {
	mu               sync.Mutex
	parent           context.Context
	cfg              agentprinter.Config
	scale            *agentscale.Monitor
	running          bool
	worker           context.Context
	cancel           context.CancelFunc
	monitorDone      chan struct{}
	statusDone       chan struct{}
	jobs             sync.WaitGroup
	generation       int64
	printing         int
	dir, environment string
	maintenance      *agentmaintenance.Service
}

func (r *deviceRuntime) start(cfg agentprinter.Config) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.running {
		return nil
	}
	scale, err := agentscale.New(agentscale.Config{Enabled: cfg.SerialEnabled, Port: cfg.SerialPort, Baud: cfg.SerialBaud, Protocol: cfg.SerialProtocol, Unit: agentscale.Unit(cfg.SerialWeightUnit), Stale: time.Duration(cfg.SerialStaleMS) * time.Millisecond}, nil)
	if err != nil {
		return err
	}
	worker, cancel := context.WithCancel(r.parent)
	r.cfg, r.scale, r.worker, r.cancel, r.running = cfg, scale, worker, cancel, true
	r.generation++
	r.monitorDone = make(chan struct{})
	done := r.monitorDone
	go func() { defer close(done); scale.Run(worker) }()
	r.statusDone = make(chan struct{})
	statusDone := r.statusDone
	go func() {
		defer close(statusDone)
		writeScaleStatus(worker, filepath.Join(r.dir, "scale-status.json"), scale)
	}()
	writeAgentStatus(filepath.Join(r.dir, "maintenance-status.json"), true, agentprinter.ConfigDigest(cfg))
	return nil
}
func (r *deviceRuntime) stop() error {
	r.mu.Lock()
	if !r.running {
		r.mu.Unlock()
		return nil
	}
	if r.printing > 0 {
		r.mu.Unlock()
		return errors.New("打印正在执行，请核实完成后再维护设备")
	}
	r.running = false
	r.generation++
	r.cancel()
	done := r.monitorDone
	statusDone := r.statusDone
	r.mu.Unlock()
	r.jobs.Wait()
	<-done
	<-statusDone
	return nil
}
func (r *deviceRuntime) snapshot() (agentprinter.Config, *agentscale.Monitor, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cfg, r.scale, r.running
}
func (r *deviceRuntime) acquire(parent context.Context, printing bool) (context.Context, agentprinter.Config, *agentscale.Monitor, func(), error) {
	return r.acquireGeneration(parent, printing, 0)
}
func (r *deviceRuntime) acquireGeneration(parent context.Context, printing bool, generation int64) (context.Context, agentprinter.Config, *agentscale.Monitor, func(), error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.running || (generation != 0 && generation != r.generation) {
		return nil, agentprinter.Config{}, nil, nil, errors.New("设备执行已停止，请先启动设备")
	}
	ctx, cancel := context.WithCancel(parent)
	stop := context.AfterFunc(r.worker, cancel)
	r.jobs.Add(1)
	if printing {
		r.printing++
	}
	return ctx, r.cfg, r.scale, func() {
		stop()
		cancel()
		r.mu.Lock()
		if printing {
			r.printing--
		}
		r.mu.Unlock()
		r.jobs.Done()
	}, nil
}
func (r *deviceRuntime) deviceState() p.DeviceState {
	_, scale, running := r.snapshot()
	if !running || scale == nil {
		return p.DeviceState{Unit: "t", Error: "设备执行已停止", DeviceOK: false}
	}
	return currentDeviceState(scale, time.Now())
}
func (r *deviceRuntime) capture(ctx context.Context) p.Capture { return r.captureWithPolicy(ctx, true) }
func (r *deviceRuntime) captureWithPolicy(ctx context.Context, required bool) p.Capture {
	child, cfg, scale, done, err := r.acquire(ctx, false)
	if err != nil {
		return p.Capture{Error: err.Error(), CapturedAt: time.Now().UTC()}
	}
	defer done()
	return captureEvidenceWithPolicy(child, cfg, scale, required)
}
func (r *deviceRuntime) testCameras(ctx context.Context, _ int64) (p.CameraTest, error) {
	child, cfg, _, done, err := r.acquire(ctx, false)
	if err != nil {
		return p.CameraTest{}, err
	}
	defer done()
	return (agentcamera.Client{Config: cfg}).Test(child), nil
}
func (r *deviceRuntime) printWorker(ctx context.Context, state, pause string, requests <-chan devicePrint, unavailable func(agentprinter.Request)) {
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-requests:
			request := job.request
			child, cfg, _, done, err := r.acquireGeneration(ctx, true, job.generation)
			if err != nil {
				unavailable(request)
				continue
			}
			handlePrint(child, cfg, state, cfg.Queue, pause, request)
			done()
		}
	}
}
func (r *deviceRuntime) report() p.MaintenanceState {
	cfg, _, running := r.snapshot()
	cfg.AgentKey = ""
	cfg.CameraFrontURL, cfg.CameraRearURL, cfg.PlateRecognitionURL = "", "", ""
	return p.MaintenanceState{Config: cfg, Running: running}
}

type deviceRunner struct {
	runtime  *deviceRuntime
	commands agentmaintenance.ExecRunner
}

func (r deviceRunner) Run(args []string) agentmaintenance.Result {
	if len(args) >= 4 && args[0] == "systemctl" && args[1] == "--user" && args[len(args)-1] == "weighbridge-agent-printer.service" {
		switch args[2] {
		case "is-active":
			_, _, running := r.runtime.snapshot()
			if running {
				return agentmaintenance.Result{Output: "active"}
			}
			return agentmaintenance.Result{Output: "inactive"}
		case "stop":
			return agentmaintenance.Result{Output: "设备执行已停止；Agent 维护连接保留", Err: r.runtime.stop()}
		case "start":
			cfg, err := agentprinter.ReadEnvironment(r.runtime.environment, agentprinter.Defaults())
			if err == nil {
				err = r.runtime.start(cfg)
			}
			return agentmaintenance.Result{Output: "设备执行已启动", Err: err}
		}
	}
	return r.commands.Run(args)
}
func environmentArgument(args []string) string {
	for i, arg := range args {
		if arg == "--env-file" && i+1 < len(args) {
			return args[i+1]
		}
		if strings.HasPrefix(arg, "--env-file=") {
			return strings.TrimPrefix(arg, "--env-file=")
		}
	}
	return ""
}

type devicePrint struct {
	request    agentprinter.Request
	generation int64
}

func (r *deviceRuntime) printGeneration() int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.generation
}
