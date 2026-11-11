package weighing

import (
	"context"
	"net/http"
	"strings"

	"github.com/EziosWJ/weighbridge/server/internal/tripsync"
)

type TripCandidateResult struct {
	Enabled    bool            `json:"enabled"`
	Available  bool            `json:"available"`
	Message    string          `json:"message,omitempty"`
	Candidates []tripsync.Trip `json:"candidates"`
}

func tripCloudMessage(err error) string {
	if status, ok := tripsync.StatusOf(err); ok {
		switch status {
		case http.StatusUnauthorized:
			return "云端拒绝厂区编码：编码不存在或已停用"
		case http.StatusNotFound:
			return "流转单不存在或不属于本厂区"
		case http.StatusConflict:
			return "流转单状态已变化，请重新查询"
		}
	}
	return err.Error()
}

func (s *Service) TripCandidates(ctx context.Context, plate string) (TripCandidateResult, error) {
	result := TripCandidateResult{Candidates: []tripsync.Trip{}}
	if !s.masterSyncEnabled || strings.TrimSpace(s.masterSyncAddress) == "" {
		result.Message = "未开启厂云同步，不能查询流转单"
		return result, nil
	}
	result.Enabled = true
	if !IsBusinessPlate(plate) {
		return result, ErrInvalid
	}
	siteCode, err := s.LocalSiteCode(ctx)
	if err != nil {
		return result, err
	}
	values, err := tripsync.FetchCandidates(ctx, s.masterSyncAddress, siteCode, normalizePlate(plate))
	if err != nil {
		result.Message = tripCloudMessage(err)
		return result, nil
	}
	if values == nil {
		values = []tripsync.Trip{}
	}
	result.Available = true
	result.Candidates = values
	return result, nil
}

func (s *Service) fetchTripForBinding(ctx context.Context, in ConfirmationInput) (*tripsync.Trip, error) {
	if in.Trip == nil || strings.TrimSpace(in.Trip.TripNo) == "" {
		return nil, nil
	}
	if !s.masterSyncEnabled || strings.TrimSpace(s.masterSyncAddress) == "" {
		return nil, RejectionError("未开启厂云同步，不能绑定流转单")
	}
	siteCode, err := s.LocalSiteCode(ctx)
	if err != nil {
		return nil, err
	}
	trip, err := tripsync.FetchTrip(ctx, s.masterSyncAddress, siteCode, in.Trip.TripNo)
	if err != nil {
		return nil, RejectionError(tripCloudMessage(err))
	}
	if trip.Version != in.Trip.Version {
		return nil, RejectionError("流转单已发生变化，请重新查询并选择")
	}
	if !trip.Linkable {
		message := "流转单不可关联"
		if len(trip.Blockers) > 0 {
			message += "：" + strings.Join(trip.Blockers, "；")
		}
		return nil, RejectionError(message)
	}
	if normalizePlate(trip.Plate) != normalizePlate(in.Business.Plate) {
		return nil, RejectionError("车牌与流转单不一致，请重新选择")
	}
	if in.Business.Direction != "inbound" {
		return nil, RejectionError("原粮流转单只能用于入厂卸货")
	}
	if trip.InspectionMethod == "lab" {
		if trip.Inspection == nil || trip.Inspection.Status != "confirmed" || trip.Inspection.Conclusion != "accept" {
			return nil, RejectionError("流转单尚未完成化验接收，不能首磅")
		}
	}
	return &trip, nil
}

func (s *Service) TaskTripBasis(ctx context.Context, taskID int64) (TaskTripBasis, error) {
	repo, ok := s.store.(*Repository)
	if !ok {
		return TaskTripBasis{Deductions: []tripsync.Deduction{}}, ErrForbidden
	}
	return repo.TaskTripBasis(ctx, taskID)
}

// TaskTripCloudStatus reads the current cloud trip state for the second-weigh
// reminder. A lookup failure is returned in Message and never blocks weighing.
func (s *Service) TaskTripCloudStatus(ctx context.Context, taskID int64) (TaskTripCloudStatus, error) {
	repo, ok := s.store.(*Repository)
	if !ok {
		return TaskTripCloudStatus{Blockers: []string{}}, ErrForbidden
	}
	binding, err := repo.TripBinding(ctx, taskID)
	if err != nil {
		return TaskTripCloudStatus{Blockers: []string{}}, err
	}
	if binding == nil {
		return TaskTripCloudStatus{Blockers: []string{}}, nil
	}
	result := TaskTripCloudStatus{Bound: true, TripNo: binding.TripNo, Blockers: []string{}}
	if !s.masterSyncEnabled || strings.TrimSpace(s.masterSyncAddress) == "" {
		result.Message = "未开启厂云同步，无法查询卸货确认"
		return result, nil
	}
	siteCode, err := s.LocalSiteCode(ctx)
	if err != nil {
		result.Message = "读取本厂厂区编码失败"
		return result, nil
	}
	trip, err := tripsync.FetchTrip(ctx, s.masterSyncAddress, siteCode, binding.TripNo)
	if err != nil {
		result.Message = tripCloudMessage(err)
		return result, nil
	}
	result.Available = true
	result.Stage = trip.Stage
	result.Linkable = trip.Linkable
	result.Unloaded = trip.UnloadedAt != nil
	result.UnloadedAt = trip.UnloadedAt
	result.UnloadingLocation = trip.UnloadingLocation
	result.TicketNo = trip.TicketNo
	if trip.Blockers != nil {
		result.Blockers = trip.Blockers
	}
	return result, nil
}
