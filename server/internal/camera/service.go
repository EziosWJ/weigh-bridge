package camera

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	p "github.com/EziosWJ/weighbridge/server/internal/agentproto"
	"github.com/EziosWJ/weighbridge/server/internal/audit"
)

const defaultMaxConcurrentCaptures = 4

// Service owns camera configuration, bounded capture and MediaMTX
// reconciliation. It is independent of the live Agent: cameras, tests and
// preview remain maintainable while the Agent is offline.
type Service struct {
	store       Store
	frame       FrameGrabber
	recognizer  Recognizer
	control     Control
	imageDir    string
	captureSlot chan struct{}

	syncMu     sync.Mutex
	startOnce  sync.Once
	syncSignal chan struct{}
}

// NewService builds the camera module. The caller supplies the Store and the
// optional acquisition dependencies; no generic device framework is created.
func NewService(store Store, imageDir string, frame FrameGrabber, recognizer Recognizer, control Control) *Service {
	if frame == nil {
		frame = HTTPGrabber{}
	}
	if recognizer == nil {
		recognizer = HTTPRecognizer{}
	}
	if control == nil {
		control = HTTPControl{}
	}
	return &Service{
		store: store, frame: frame, recognizer: recognizer, control: control,
		imageDir: imageDir, captureSlot: make(chan struct{}, defaultMaxConcurrentCaptures),
		syncSignal: make(chan struct{}, 1),
	}
}

// NewServiceWithDefaults is the production constructor used by cmd/api.
func NewServiceWithDefaults(store Store, imageDir string) *Service {
	return NewService(store, imageDir, nil, nil, nil)
}

func (s *Service) List(ctx context.Context) (List, error) {
	settings, cameras, streams, err := s.store.ListState(ctx)
	if err != nil {
		return List{}, err
	}
	streamsByCamera := map[int64]streamRecord{}
	for _, stream := range streams {
		streamsByCamera[stream.CameraID] = stream
	}
	out := List{
		Cameras:          make([]Camera, 0, len(cameras)),
		ConfigVersion:    settings.ConfigVersion,
		CaptureTimeoutMS: captureTimeout(settings),
		RecognitionURL:   settings.RecognitionURL,
		Video:            toVideoSettings(settings),
	}
	if settings.RecognitionCameraID != nil {
		id := *settings.RecognitionCameraID
		out.RecognitionCameraID = &id
	}
	for _, camera := range cameras {
		stream, ok := streamsByCamera[camera.ID]
		var pointer *streamRecord
		if ok {
			pointer = &stream
		}
		out.Cameras = append(out.Cameras, toCamera(camera, pointer, settings))
	}
	return out, nil
}

func (s *Service) Get(ctx context.Context, id int64) (Camera, error) {
	settings, cameras, streams, err := s.store.ListState(ctx)
	if err != nil {
		return Camera{}, err
	}
	for _, stream := range streams {
		if stream.CameraID == id {
			for _, camera := range cameras {
				if camera.ID == id {
					return toCamera(camera, &stream, settings), nil
				}
			}
		}
	}
	camera, err := s.store.Camera(ctx, id)
	if err != nil {
		return Camera{}, err
	}
	return toCamera(camera, nil, settings), nil
}

// ConfigVersion is the current capture-relevant camera set version. DC-06
// compares it with the version frozen by Plan and with the sample row.
func (s *Service) ConfigVersion(ctx context.Context) (int64, error) {
	settings, _, _, err := s.store.ListState(ctx)
	if err != nil {
		return 0, err
	}
	return settings.ConfigVersion, nil
}

func (s *Service) Settings(ctx context.Context) (CaptureSettings, error) {
	settings, _, _, err := s.store.ListState(ctx)
	if err != nil {
		return CaptureSettings{}, err
	}
	return toCaptureSettings(settings), nil
}

func (s *Service) Video(ctx context.Context) (VideoSettings, error) {
	settings, _, _, err := s.store.ListState(ctx)
	if err != nil {
		return VideoSettings{}, err
	}
	return toVideoSettings(settings), nil
}

func (s *Service) Create(ctx context.Context, input CameraInput, event audit.Event) (Camera, error) {
	record, _, _, err := normalizeCameraInput(nil, input)
	if err != nil {
		return Camera{}, err
	}
	publicID, err := randomToken("cam_", 12)
	if err != nil {
		return Camera{}, err
	}
	record.PublicID = publicID
	event.Action = "camera.create"
	event.Resource = "weighing.camera"
	id, _, err := s.store.CreateCamera(ctx, record, streamRecord{
		MediaPath: managedPath(publicID), DesiredVersion: 1, State: StreamPending,
	}, event)
	if err != nil {
		return Camera{}, err
	}
	s.TriggerSync()
	return s.Get(ctx, id)
}

func (s *Service) Update(ctx context.Context, id int64, input CameraInput, event audit.Event) (Camera, error) {
	current, err := s.store.Camera(ctx, id)
	if err != nil {
		return Camera{}, err
	}
	if input.Version < 1 {
		return Camera{}, ErrInvalid
	}
	if input.Version != current.Version {
		return Camera{}, ErrConflict
	}
	record, contentChanged, streamChanged, err := normalizeCameraInput(&current, input)
	if err != nil {
		return Camera{}, err
	}
	event.Action = "camera.update"
	event.Resource = "weighing.camera"
	event.ResourceID = id
	if _, err := s.store.UpdateCamera(ctx, id, current.Version, record, contentChanged, streamChanged, event); err != nil {
		return Camera{}, err
	}
	if streamChanged {
		s.TriggerSync()
	}
	return s.Get(ctx, id)
}

func (s *Service) Delete(ctx context.Context, id, version int64, event audit.Event) error {
	current, err := s.store.Camera(ctx, id)
	if err != nil {
		return err
	}
	if version < 1 {
		return ErrInvalid
	}
	if current.Version != version {
		return ErrConflict
	}
	event.Action = "camera.delete"
	event.Resource = "weighing.camera"
	event.ResourceID = id
	if _, err := s.store.DeleteCamera(ctx, id, version, event); err != nil {
		return err
	}
	s.TriggerSync()
	return nil
}

func (s *Service) SaveCaptureSettings(ctx context.Context, input CaptureSettingsInput, event audit.Event) (CaptureSettings, error) {
	if input.ConfigVersion < 1 {
		return CaptureSettings{}, ErrInvalid
	}
	settings, _, _, err := s.store.ListState(ctx)
	if err != nil {
		return CaptureSettings{}, err
	}
	if settings.ConfigVersion != input.ConfigVersion {
		return CaptureSettings{}, ErrConflict
	}
	timeout := captureTimeout(settings)
	if input.CaptureTimeoutMS != nil {
		timeout = *input.CaptureTimeoutMS
	}
	if timeout < minCaptureTimeoutMS || timeout > maxCaptureTimeoutMS {
		return CaptureSettings{}, ErrInvalid
	}
	recognitionURL := settings.RecognitionURL
	if input.RecognitionURL != nil {
		recognitionURL, err = normalizeHTTPURL(*input.RecognitionURL, true)
		if err != nil {
			return CaptureSettings{}, err
		}
	}
	recognitionCameraID := settings.RecognitionCameraID
	if input.ClearRecognitionCamera {
		recognitionCameraID = nil
	} else if input.RecognitionCameraID != nil {
		if *input.RecognitionCameraID < 1 {
			return CaptureSettings{}, ErrInvalid
		}
		id := *input.RecognitionCameraID
		recognitionCameraID = &id
	}
	contentChanged := recognitionURL != settings.RecognitionURL || !sameID(recognitionCameraID, settings.RecognitionCameraID)
	event.Action = "camera.settings.update"
	event.Resource = "weighing.camera.settings"
	updated, err := s.store.UpdateCaptureSettings(ctx, input.ConfigVersion, timeout, recognitionCameraID, recognitionURL, contentChanged, event)
	if err != nil {
		return CaptureSettings{}, err
	}
	return toCaptureSettings(updated), nil
}

func (s *Service) SaveVideoSettings(ctx context.Context, input VideoSettingsInput, event audit.Event) (VideoSettings, error) {
	if input.Version < 1 {
		return VideoSettings{}, ErrInvalid
	}
	settings, _, _, err := s.store.ListState(ctx)
	if err != nil {
		return VideoSettings{}, err
	}
	if settings.VideoVersion != input.Version {
		return VideoSettings{}, ErrConflict
	}
	apiURL, err := normalizeBaseURL(input.APIURL, true)
	if err != nil {
		return VideoSettings{}, err
	}
	whepBaseURL, err := normalizeBaseURL(input.WHEPBaseURL, true)
	if err != nil {
		return VideoSettings{}, err
	}
	username := settings.MediaMTXAPIUsername
	if input.APIUsername != nil {
		username = *input.APIUsername
		if len(username) > 200 || strings.ContainsAny(username, "\r\n\x00") {
			return VideoSettings{}, ErrInvalid
		}
	}
	password, _, err := resolveSecret(settings.MediaMTXAPIPassword, input.APIPassword, input.PasswordAction)
	if err != nil {
		return VideoSettings{}, err
	}
	if password != "" && username == "" {
		return VideoSettings{}, ErrInvalid
	}
	event.Action = "camera.video.update"
	event.Resource = "weighing.camera.video"
	updated, err := s.store.UpdateVideoSettings(ctx, input.Version, apiURL, username, password, whepBaseURL, event)
	if err != nil {
		return VideoSettings{}, err
	}
	s.TriggerSync()
	return toVideoSettings(updated), nil
}

// Plan freezes the currently enabled capture cameras and credentials. DC-06
// records ConfigVersion on the sample and re-validates before confirmation.
func (s *Service) Plan(ctx context.Context, scaleCode string, captureID string, required bool) (CapturePlan, error) {
	settings, cameras, _, err := s.store.ListState(ctx)
	if err != nil {
		return CapturePlan{}, err
	}
	plan := CapturePlan{
		ScaleCode: scaleCode, CaptureID: captureID, ConfigVersion: settings.ConfigVersion,
		TimeoutMS: captureTimeout(settings), Required: required,
		RecognitionURL: settings.RecognitionURL,
		Cameras:        []CaptureCamera{},
	}
	if settings.RecognitionCameraID != nil {
		plan.RecognitionCamera = *settings.RecognitionCameraID
	}
	for _, camera := range cameras {
		if !camera.Enabled || !camera.CaptureEnabled {
			continue
		}
		plan.Cameras = append(plan.Cameras, CaptureCamera{
			ID: camera.ID, PublicID: camera.PublicID, Name: camera.Name,
			SourceURL: camera.SourceURL, Username: camera.Username, Password: camera.Password,
			CaptureEnabled: camera.CaptureEnabled, Version: camera.Version,
		})
	}
	return plan, nil
}

// Capture acquires one fresh image per frozen camera under a single deadline.
// It never creates a sample; DC-06 owns the weighing transaction.
func (s *Service) Capture(ctx context.Context, plan CapturePlan) (CaptureOutcome, error) {
	started := time.Now().UTC()
	if plan.ScaleCode == "" {
		return CaptureOutcome{}, ErrInvalid
	}
	outcome := CaptureOutcome{ConfigVersion: plan.ConfigVersion, Images: []p.Image{}, Failures: []CaptureFailure{}}
	if len(plan.Cameras) == 0 {
		outcome.At = started
		if plan.Required {
			outcome.Failures = append(outcome.Failures, CaptureFailure{Code: CodeCaptureRequired, Message: "未指定称重抓拍摄像头"})
		} else {
			outcome.Success = true
		}
		return outcome, nil
	}
	timeout := plan.TimeoutMS
	if timeout <= 0 {
		timeout = defaultCaptureTimeoutMS
	}
	captureCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Millisecond)
	defer cancel()
	images := make([]p.Image, len(plan.Cameras))
	failures := make([]*CaptureFailure, len(plan.Cameras))
	var wait sync.WaitGroup
	for index := range plan.Cameras {
		index := index
		camera := plan.Cameras[index]
		wait.Add(1)
		go func() {
			defer wait.Done()
			image, failure := s.captureOne(ctx, captureCtx, plan, camera)
			if failure != nil {
				failures[index] = failure
				if plan.ScaleCode != "" {
					s.recordFailedImage(ctx, plan, camera, *failure)
				}
				return
			}
			images[index] = image
		}()
	}
	wait.Wait()
	for _, image := range images {
		if image.ID != "" {
			outcome.Images = append(outcome.Images, image)
		}
	}
	for _, failure := range failures {
		if failure != nil {
			outcome.Failures = append(outcome.Failures, *failure)
		}
	}
	// Success expresses whether the capture policy was satisfied, not merely
	// whether every camera worked: optional capture may continue with missing
	// images, while required capture needs the full non-empty set.
	outcome.Success = !plan.Required || (len(outcome.Images) > 0 && len(outcome.Failures) == 0)
	outcome.At = time.Now().UTC()
	return outcome, nil
}

func (s *Service) captureOne(parent, captureCtx context.Context, plan CapturePlan, camera CaptureCamera) (p.Image, *CaptureFailure) {
	if strings.TrimSpace(camera.SourceURL) == "" {
		failure := CaptureFailure{CameraID: camera.ID, CameraPublicID: camera.PublicID, CameraName: camera.Name, Code: CodeNotConfigured, Message: "摄像头未配置源地址"}
		return p.Image{}, &failure
	}
	select {
	case s.captureSlot <- struct{}{}:
		defer func() { <-s.captureSlot }()
	case <-captureCtx.Done():
		failure := failureFor(camera, contextFrameError(captureCtx))
		return p.Image{}, &failure
	}
	frame, err := s.frame.Snapshot(captureCtx, camera.SourceURL, camera.Username, camera.Password)
	if err != nil {
		if errors.Is(captureCtx.Err(), context.DeadlineExceeded) {
			err = errFrameTimeout
		}
		failure := failureFor(camera, err)
		return p.Image{}, &failure
	}
	id, err := randomToken("srv_", 16)
	if err != nil {
		failure := failureFor(camera, err)
		return p.Image{}, &failure
	}
	id += "." + imageExtension(frame.Format)
	fileName := id
	if err := s.writeImage(fileName, frame.Data); err != nil {
		failure := CaptureFailure{CameraID: camera.ID, CameraPublicID: camera.PublicID, CameraName: camera.Name, Code: CodeSaveFailed, Message: "抓拍图片保存失败"}
		return p.Image{}, &failure
	}
	digest := imageDigest(frame.Data)
	capturedAt := time.Now().UTC()
	record := imageRecord{
		ID: id, ScaleCode: plan.ScaleCode, CameraID: int64Pointer(camera.ID), CameraPublicID: camera.PublicID,
		CameraName: camera.Name, CameraSource: camera.SourceURL, Source: "server", Status: "captured",
		FileName: fileName, SHA256: digest, Width: frame.Width, Height: frame.Height,
		Bytes: int64(len(frame.Data)), CapturedAt: capturedAt,
	}
	if err := s.store.SaveImage(parent, record); err != nil {
		_ = os.Remove(filepath.Join(s.imageDir, fileName))
		failure := CaptureFailure{CameraID: camera.ID, CameraPublicID: camera.PublicID, CameraName: camera.Name, Code: CodeSaveFailed, Message: "抓拍图片登记失败"}
		return p.Image{}, &failure
	}
	return p.Image{
		ID: id, URL: "/api/weighing/images/" + id, SHA256: digest, Source: "server",
		CameraID: camera.ID, CameraPublicID: camera.PublicID, CameraName: camera.Name,
		Status: "captured", CapturedAt: &capturedAt, Width: frame.Width, Height: frame.Height,
		Bytes: int64(len(frame.Data)),
	}, nil
}

func (s *Service) recordFailedImage(ctx context.Context, plan CapturePlan, camera CaptureCamera, failure CaptureFailure) {
	id, err := randomToken("srv_", 16)
	if err != nil {
		return
	}
	id += ".jpg"
	_ = s.store.SaveImage(ctx, imageRecord{
		ID: id, ScaleCode: plan.ScaleCode, CameraID: int64Pointer(camera.ID), CameraPublicID: camera.PublicID,
		CameraName: camera.Name, CameraSource: camera.SourceURL, Source: "server", Status: "failed",
		Error: failure.Message, CapturedAt: time.Now().UTC(),
	})
}

// Test performs a diagnostic acquisition. It never registers a sample, a
// weighing image row or a business record.
func (s *Service) Test(ctx context.Context, id int64, event audit.Event) (TestResult, error) {
	settings, _, _, err := s.store.ListState(ctx)
	if err != nil {
		return TestResult{}, err
	}
	camera, err := s.store.Camera(ctx, id)
	if err != nil {
		return TestResult{}, err
	}
	result := TestResult{At: time.Now().UTC()}
	if strings.TrimSpace(camera.SourceURL) == "" {
		result.Code, result.Message = CodeNotConfigured, "摄像头未配置源地址"
	} else {
		timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(captureTimeout(settings))*time.Millisecond)
		frame, snapshotErr := s.frame.Snapshot(timeoutCtx, camera.SourceURL, camera.Username, camera.Password)
		if errors.Is(timeoutCtx.Err(), context.DeadlineExceeded) {
			snapshotErr = errFrameTimeout
		}
		cancel()
		if snapshotErr != nil {
			result.Code, result.Message = failureCode(snapshotErr)
		} else {
			thumb, width, height, thumbErr := thumbnailJPEG(frame.Data)
			if thumbErr != nil {
				result.Code, result.Message = CodeInvalidImage, "摄像头图片处理失败"
			} else {
				result.Success, result.JPEG, result.Width, result.Height = true, thumb, width, height
			}
		}
	}
	event.Action = "camera.test"
	event.Resource = "weighing.camera"
	event.ResourceID = id
	if result.Success {
		event.Summary = "测试抓拍成功"
	} else {
		event.Summary = result.Message
	}
	if err := s.store.RecordAudit(ctx, event); err != nil {
		return TestResult{}, err
	}
	result.At = time.Now().UTC()
	return result, nil
}

// Preview returns the managed playback mapping. A URL is exposed only after
// the current source version has been synced, so a stale picture cannot be
// presented as the new configuration.
func (s *Service) Preview(ctx context.Context, id int64) (Stream, error) {
	settings, cameras, streams, err := s.store.ListState(ctx)
	if err != nil {
		return Stream{}, err
	}
	var camera *cameraRecord
	for index := range cameras {
		if cameras[index].ID == id {
			camera = &cameras[index]
			break
		}
	}
	if camera == nil {
		return Stream{}, ErrNotFound
	}
	for _, stream := range streams {
		if stream.CameraID == id {
			return toStream(*camera, &stream, settings), nil
		}
	}
	return toStream(*camera, nil, settings), nil
}

// ImagePath validates a Server-acquired image reference before serving it.
func (s *Service) ImagePath(ctx context.Context, id string) (string, error) {
	record, err := s.store.Image(ctx, id)
	if err != nil {
		return "", err
	}
	if record.Status != "captured" || record.FileName == "" || s.imageDir == "" {
		return "", ErrNotFound
	}
	if filepath.Base(record.FileName) != record.FileName || record.FileName == "." || record.FileName == ".." {
		return "", ErrNotFound
	}
	path := filepath.Join(s.imageDir, record.FileName)
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return "", ErrNotFound
	}
	return path, nil
}

// RecognizeImage reuses an already captured image. Recognition is optional and
// its failure never blocks a business confirmation.
func (s *Service) RecognizeImage(ctx context.Context, endpoint, imageID string) (string, error) {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return "", nil
	}
	if _, err := normalizeHTTPURL(endpoint, false); err != nil {
		return "", err
	}
	path, err := s.ImagePath(ctx, imageID)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	recognizeCtx, cancel := recognitionTimeout(ctx)
	defer cancel()
	return s.recognizer.Recognize(recognizeCtx, endpoint, data)
}

// RecognizeOutcome recognizes the image produced by the configured capture
// camera, without acquiring an extra frame.
func (s *Service) RecognizeOutcome(ctx context.Context, plan CapturePlan, outcome CaptureOutcome) (string, error) {
	if plan.RecognitionCamera < 1 || strings.TrimSpace(plan.RecognitionURL) == "" {
		return "", nil
	}
	for _, image := range outcome.Images {
		if image.CameraID == plan.RecognitionCamera && image.Status == "captured" && image.ID != "" {
			return s.RecognizeImage(ctx, plan.RecognitionURL, image.ID)
		}
	}
	return "", nil
}

func (s *Service) writeImage(name string, data []byte) error {
	if s.imageDir == "" {
		return errors.New("image directory is not configured")
	}
	if err := os.MkdirAll(s.imageDir, 0o700); err != nil {
		return err
	}
	temp, err := os.CreateTemp(s.imageDir, ".camera-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	cleanup := func() {
		_ = temp.Close()
		_ = os.Remove(tempName)
	}
	if _, err := temp.Write(data); err != nil {
		cleanup()
		return err
	}
	if err := temp.Chmod(0o600); err != nil {
		cleanup()
		return err
	}
	if err := temp.Close(); err != nil {
		_ = os.Remove(tempName)
		return err
	}
	if err := os.Rename(tempName, filepath.Join(s.imageDir, name)); err != nil {
		_ = os.Remove(tempName)
		return err
	}
	return nil
}

// Start runs an initial reconciliation and then a light periodic check. The
// periodic check restores paths lost by a MediaMTX restart or file reload; it
// does not create a generic job platform.
func (s *Service) Start(ctx context.Context) {
	s.startOnce.Do(func() {
		go func() {
			s.backgroundSync(ctx)
			ticker := time.NewTicker(5 * time.Minute)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-s.syncSignal:
					s.backgroundSync(ctx)
				case <-ticker.C:
					s.backgroundSync(ctx)
				}
			}
		}()
	})
}

// TriggerSync coalesces save-triggered reconciliations.
func (s *Service) TriggerSync() {
	select {
	case s.syncSignal <- struct{}{}:
	default:
	}
}

func (s *Service) backgroundSync(ctx context.Context) {
	syncCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	_, _ = s.Sync(syncCtx, SyncRequest{})
}

// Sync reconciles only this system's managed MediaMTX paths. Results are bound
// to the current versions; a late request cannot mark a newer configuration as
// synced because every write is conditional on the observed version.
func (s *Service) Sync(ctx context.Context, request SyncRequest) (SyncResult, error) {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	settings, streams, cameras, err := s.store.SyncSnapshot(ctx)
	if err != nil {
		return SyncResult{}, err
	}
	if request.ConfigVersion != 0 && request.ConfigVersion != settings.ConfigVersion {
		return SyncResult{}, ErrConflict
	}
	if request.VideoVersion != 0 && request.VideoVersion != settings.VideoVersion {
		return SyncResult{}, ErrConflict
	}
	result := SyncResult{
		ConfigVersion: settings.ConfigVersion, VideoVersion: settings.VideoVersion,
		Streams: make([]Stream, 0, len(streams)),
	}
	endpoint := Endpoint{URL: settings.MediaMTXAPIURL, Username: settings.MediaMTXAPIUsername, Password: settings.MediaMTXAPIPassword}
	if strings.TrimSpace(settings.MediaMTXAPIURL) == "" {
		for _, row := range streams {
			if camera, ok := cameras[row.CameraID]; ok {
				result.Streams = append(result.Streams, toStream(camera, &row, settings))
			}
		}
		result.State = VideoSyncPending
		result.Error = "未配置视频预览服务管理地址"
		result.At = time.Now().UTC()
		return result, nil
	}
	var overallError string
	for _, row := range streams {
		stream := row
		camera, ok := cameras[row.CameraID]
		if !ok {
			continue
		}
		active := !camera.Deleted && camera.Enabled
		if !active {
			_, exists, getErr := s.control.Get(ctx, endpoint, row.MediaPath)
			if getErr != nil {
				if err := s.store.MarkStreamError(ctx, row.CameraID, row.DesiredVersion, "视频服务不可用"); err != nil {
					return SyncResult{}, err
				}
				stream.State, stream.LastError = StreamError, "视频服务不可用"
				if overallError == "" {
					overallError = "视频服务不可用"
				}
			} else {
				removeErr := error(nil)
				if exists {
					removeErr = s.control.Delete(ctx, endpoint, row.MediaPath)
				}
				if removeErr != nil {
					if err := s.store.MarkStreamError(ctx, row.CameraID, row.DesiredVersion, "托管路径删除失败"); err != nil {
						return SyncResult{}, err
					}
					stream.State, stream.LastError = StreamError, "托管路径删除失败"
					if overallError == "" {
						overallError = "托管路径删除失败"
					}
				} else {
					if err := s.store.MarkStreamRemoved(ctx, row.CameraID, row.DesiredVersion); err != nil {
						return SyncResult{}, err
					}
					stream.State, stream.LastError = StreamRemoved, ""
				}
			}
			result.Streams = append(result.Streams, toStream(camera, &stream, settings))
			continue
		}
		message := ""
		if strings.TrimSpace(camera.SourceURL) == "" {
			message = "摄像头未配置源地址"
		} else if !strings.HasPrefix(strings.ToLower(camera.SourceURL), "rtsp://") {
			message = "视频预览仅支持 RTSP 源"
		} else {
			source, sourceErr := sourceWithCredentials(camera.SourceURL, camera.Username, camera.Password)
			if sourceErr != nil {
				message = "摄像头源地址无效"
			} else {
				remoteSource, exists, getErr := s.control.Get(ctx, endpoint, row.MediaPath)
				switch {
				case getErr != nil:
					message = "视频服务不可用"
				case !exists:
					createErr := s.control.Add(ctx, endpoint, row.MediaPath, source)
					if errors.Is(createErr, ErrConflict) {
						// A concurrent sync created the path first; the source
						// still has to match this version.
						createErr = s.control.Patch(ctx, endpoint, row.MediaPath, source)
					}
					if createErr != nil {
						message = "托管路径创建失败"
					}
				case remoteSource != source:
					patchErr := s.control.Patch(ctx, endpoint, row.MediaPath, source)
					if errors.Is(patchErr, errRemotePathNotFound) {
						patchErr = s.control.Add(ctx, endpoint, row.MediaPath, source)
					}
					if patchErr != nil {
						message = "托管路径更新失败"
					}
				}
			}
		}
		if message != "" {
			if err := s.store.MarkStreamError(ctx, row.CameraID, row.DesiredVersion, message); err != nil {
				return SyncResult{}, err
			}
			stream.State, stream.LastError = StreamError, message
			if overallError == "" {
				overallError = message
			}
		} else {
			if err := s.store.MarkStreamSynced(ctx, row.CameraID, row.DesiredVersion); err != nil {
				return SyncResult{}, err
			}
			stream.State, stream.LastError = StreamSynced, ""
			stream.SyncedVersion = row.DesiredVersion
		}
		result.Streams = append(result.Streams, toStream(camera, &stream, settings))
	}
	result.State = VideoSyncSynced
	if overallError != "" {
		result.State = VideoSyncError
		result.Error = overallError
	}
	if err := s.store.UpdateVideoSync(ctx, settings.VideoVersion, result.State, result.Error, overallError == ""); err != nil {
		return SyncResult{}, err
	}
	result.At = time.Now().UTC()
	return result, nil
}

func normalizeCameraInput(current *cameraRecord, input CameraInput) (cameraRecord, bool, bool, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" || len([]rune(name)) > 100 || strings.ContainsAny(name, "\x00\r\n") {
		return cameraRecord{}, false, false, ErrInvalid
	}
	source, err := normalizeSourceURL(input.SourceURL)
	if err != nil {
		return cameraRecord{}, false, false, err
	}
	username := ""
	password := ""
	captureEnabled := false
	enabled := true
	if current != nil {
		username, password = current.Username, current.Password
		captureEnabled, enabled = current.CaptureEnabled, current.Enabled
	}
	if input.Username != nil {
		username = *input.Username
		if len(username) > 200 || strings.ContainsAny(username, "\r\n\x00") {
			return cameraRecord{}, false, false, ErrInvalid
		}
	}
	password, passwordChanged, err := resolveSecret(password, input.Password, input.PasswordAction)
	if err != nil {
		return cameraRecord{}, false, false, err
	}
	if password != "" && username == "" {
		return cameraRecord{}, false, false, ErrInvalid
	}
	if input.CaptureEnabled != nil {
		captureEnabled = *input.CaptureEnabled
	}
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	record := cameraRecord{
		Name: name, SourceURL: source, Username: username, Password: password,
		CaptureEnabled: captureEnabled, Enabled: enabled,
	}
	if current == nil {
		return record, true, true, nil
	}
	contentChanged := source != current.SourceURL || username != current.Username || passwordChanged ||
		captureEnabled != current.CaptureEnabled || enabled != current.Enabled
	streamChanged := source != current.SourceURL || username != current.Username || passwordChanged || enabled != current.Enabled
	return record, contentChanged, streamChanged, nil
}

func resolveSecret(current, provided, action string) (string, bool, error) {
	switch strings.TrimSpace(action) {
	case "", SecretKeep:
		if provided == "" {
			return current, false, nil
		}
		if err := validateSecret(provided); err != nil {
			return "", false, err
		}
		return provided, provided != current, nil
	case SecretSet:
		if provided == "" {
			return "", false, ErrInvalid
		}
		if err := validateSecret(provided); err != nil {
			return "", false, err
		}
		return provided, provided != current, nil
	case SecretClear:
		if provided != "" {
			return "", false, ErrInvalid
		}
		return "", current != "", nil
	default:
		return "", false, ErrInvalid
	}
}

func validateSecret(value string) error {
	if len(value) > 500 || strings.ContainsAny(value, "\x00\r\n") {
		return ErrInvalid
	}
	return nil
}

func normalizeSourceURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.ContainsAny(raw, "\r\n\x00") {
		return "", ErrInvalid
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || parsed.User != nil ||
		(parsed.Scheme != "rtsp" && parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", ErrInvalid
	}
	return raw, nil
}

// normalizeBaseURL is for MediaMTX management and playback endpoints. Query
// strings are rejected because the managed API path is appended to the base.
func normalizeBaseURL(raw string, allowEmpty bool) (string, error) {
	value, err := normalizeHTTPURL(raw, allowEmpty)
	if err != nil || value == "" {
		return value, err
	}
	parsed, _ := url.Parse(value)
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", ErrInvalid
	}
	return value, nil
}

func normalizeHTTPURL(raw string, allowEmpty bool) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		if allowEmpty {
			return "", nil
		}
		return "", ErrInvalid
	}
	if strings.ContainsAny(raw, "\r\n\x00") {
		return "", ErrInvalid
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || parsed.User != nil ||
		(parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Fragment != "" {
		return "", ErrInvalid
	}
	return raw, nil
}

func sameID(left, right *int64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func toCamera(camera cameraRecord, stream *streamRecord, settings settingsRecord) Camera {
	output := Camera{
		ID: camera.ID, PublicID: camera.PublicID, Name: camera.Name, SourceURL: camera.SourceURL,
		Username: camera.Username, HasPassword: camera.Password != "", CaptureEnabled: camera.CaptureEnabled,
		Enabled: camera.Enabled, Version: camera.Version, CreateTime: camera.CreateTime, UpdateTime: camera.UpdateTime,
	}
	output.Stream = toStream(camera, stream, settings)
	return output
}

func toStream(camera cameraRecord, stream *streamRecord, settings settingsRecord) Stream {
	if stream == nil || stream.MediaPath == "" {
		return Stream{
			Path: managedPath(camera.PublicID), DesiredVersion: 1, SyncedVersion: 0,
			State: StreamPending, UpdatedAt: camera.UpdateTime,
		}
	}
	output := Stream{
		Path: stream.MediaPath, DesiredVersion: stream.DesiredVersion, SyncedVersion: stream.SyncedVersion,
		State: stream.State, LastError: stream.LastError, UpdatedAt: stream.UpdatedAt,
	}
	if output.State == "" {
		output.State = StreamPending
	}
	if output.State == StreamSynced && camera.Enabled {
		output.PlaybackURL = playbackURL(settings.MediaMTXWHEPBaseURL, stream.MediaPath)
	}
	return output
}

func toCaptureSettings(settings settingsRecord) CaptureSettings {
	output := CaptureSettings{
		ConfigVersion: settings.ConfigVersion, CaptureTimeoutMS: captureTimeout(settings),
		RecognitionURL: settings.RecognitionURL,
	}
	if settings.RecognitionCameraID != nil {
		id := *settings.RecognitionCameraID
		output.RecognitionCameraID = &id
	}
	return output
}

func toVideoSettings(settings settingsRecord) VideoSettings {
	return VideoSettings{
		APIURL: settings.MediaMTXAPIURL, APIUsername: settings.MediaMTXAPIUsername,
		HasAPIPassword: settings.MediaMTXAPIPassword != "", WHEPBaseURL: settings.MediaMTXWHEPBaseURL,
		Version: settings.VideoVersion, SyncState: settings.VideoSyncState,
		SyncError: settings.VideoSyncError, SyncedVersion: settings.VideoSyncedVersion,
		UpdateTime: settings.UpdateTime,
	}
}

func captureTimeout(settings settingsRecord) int {
	if settings.CaptureTimeoutMS >= minCaptureTimeoutMS && settings.CaptureTimeoutMS <= maxCaptureTimeoutMS {
		return settings.CaptureTimeoutMS
	}
	return defaultCaptureTimeoutMS
}

func failureFor(camera CaptureCamera, err error) CaptureFailure {
	code, message := failureCode(err)
	return CaptureFailure{CameraID: camera.ID, CameraPublicID: camera.PublicID, CameraName: camera.Name, Code: code, Message: message}
}

func failureCode(err error) (string, string) {
	switch {
	case errors.Is(err, errFrameTimeout):
		return CodeTimeout, "摄像头抓拍超时"
	case errors.Is(err, errInvalidFrame):
		return CodeInvalidImage, "摄像头未返回有效图片"
	case errors.Is(err, errFrameUnavailable):
		return CodeUnavailable, "摄像头不可用"
	default:
		return CodeUnavailable, "摄像头抓拍失败"
	}
}

func randomToken(prefix string, size int) (string, error) {
	buffer := make([]byte, size)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(buffer), nil
}

func int64Pointer(value int64) *int64 { return &value }

// TestResult carries a bounded thumbnail and the concrete diagnostic error;
// it is not a formal image reference.
type TestResult struct {
	Success bool      `json:"success"`
	Code    string    `json:"code,omitempty"`
	Message string    `json:"message,omitempty"`
	JPEG    []byte    `json:"jpeg,omitempty"`
	Width   int       `json:"width,omitempty"`
	Height  int       `json:"height,omitempty"`
	At      time.Time `json:"at"`
}
