package api

import (
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/dreamer-zq/DKNet/internal/tss"
	tssv1 "github.com/dreamer-zq/DKNet/proto/tss/v1"
)

// Helper functions to convert between internal types and proto types
func convertOperationStatus(status tss.OperationStatus) tssv1.OperationStatus {
	switch status {
	case tss.StatusPending:
		return tssv1.OperationStatus_OPERATION_STATUS_PENDING
	case tss.StatusInProgress:
		return tssv1.OperationStatus_OPERATION_STATUS_IN_PROGRESS
	case tss.StatusCompleted:
		return tssv1.OperationStatus_OPERATION_STATUS_COMPLETED
	case tss.StatusFailed:
		return tssv1.OperationStatus_OPERATION_STATUS_FAILED
	case tss.StatusCancelled:
		return tssv1.OperationStatus_OPERATION_STATUS_CANCELED
	default:
		return tssv1.OperationStatus_OPERATION_STATUS_UNSPECIFIED
	}
}

func convertOperationType(opType tss.OperationType) tssv1.OperationType {
	switch opType {
	case tss.OperationKeygen:
		return tssv1.OperationType_OPERATION_TYPE_KEYGEN
	case tss.OperationSigning:
		return tssv1.OperationType_OPERATION_TYPE_SIGNING
	case tss.OperationResharing:
		return tssv1.OperationType_OPERATION_TYPE_RESHARING
	default:
		return tssv1.OperationType_OPERATION_TYPE_UNSPECIFIED
	}
}

// buildOperationResponse builds a complete operation response from in-memory operation
func buildOperationResponse(operation *tss.Operation) *tssv1.GetOperationResponse {
	response := &tssv1.GetOperationResponse{
		OperationId: operation.ID,
		Type:        convertOperationType(operation.Type),
		SessionId:   operation.SessionID,
		Status:      convertOperationStatus(operation.Status),
		CreatedAt:   timestamppb.New(operation.CreatedAt),
	}

	// Add participants
	for _, p := range operation.Participants {
		response.Participants = append(response.Participants, p.Id)
	}

	// Add completion time if available
	if operation.CompletedAt != nil {
		response.CompletedAt = timestamppb.New(*operation.CompletedAt)
	}

	// Add error if available
	if operation.Error != nil {
		errMsg := operation.Error.Error()
		response.Error = &errMsg
	}

	// Add result based on operation type
	if operation.Result != nil {
		switch operation.Type {
		case tss.OperationKeygen:
			if keygenResult, ok := operation.Result.(*tss.KeygenResult); ok {
				response.Result = &tssv1.GetOperationResponse_KeygenResult{
					KeygenResult: &tssv1.KeygenResult{
						PublicKey: keygenResult.PublicKey,
						KeyId:     keygenResult.KeyID,
					},
				}
			}
		case tss.OperationSigning:
			if signingResult, ok := operation.Result.(*tss.SigningResult); ok {
				response.Result = &tssv1.GetOperationResponse_SigningResult{
					SigningResult: &tssv1.SigningResult{
						Signature: signingResult.Signature,
						R:         signingResult.R,
						S:         signingResult.S,
						V:         int32(signingResult.V),
					},
				}
			}
		case tss.OperationResharing:
			if resharingResult, ok := operation.Result.(*tss.KeygenResult); ok {
				response.Result = &tssv1.GetOperationResponse_ResharingResult{
					ResharingResult: &tssv1.KeygenResult{
						PublicKey: resharingResult.PublicKey,
						KeyId:     resharingResult.KeyID,
					},
				}
			}
		}
	}

	// Add original request
	if operation.Request != nil {
		switch req := operation.Request.(type) {
		case *tss.KeygenRequest:
			response.Request = &tssv1.GetOperationResponse_KeygenRequest{
				KeygenRequest: &tssv1.StartKeygenRequest{
					Threshold:    int32(req.Threshold),
					Participants: req.Participants,
				},
			}
		case *tss.SigningRequest:
			response.Request = &tssv1.GetOperationResponse_SigningRequest{
				SigningRequest: &tssv1.StartSigningRequest{
					Message:      req.Message,
					KeyId:        req.KeyID,
					Participants: req.Participants,
				},
			}
		case *tss.ResharingRequest:
			response.Request = &tssv1.GetOperationResponse_ResharingRequest{
				ResharingRequest: &tssv1.StartResharingRequest{
					KeyId:           req.KeyID,
					NewThreshold:    int32(req.NewThreshold),
					OldParticipants: req.OldParticipants,
					NewParticipants: req.NewParticipants,
				},
			}
		}
	}

	return response
}

// buildOperationResponseFromStorage builds a complete operation response from storage data
// This function takes the same parameters as the TSS service's GetOperationData method returns
func buildOperationResponseFromStorage(data *tss.OperationData) *tssv1.GetOperationResponse {
	response := &tssv1.GetOperationResponse{
		OperationId:  data.ID,
		Type:         convertOperationType(data.Type),
		SessionId:    data.SessionID,
		Status:       convertOperationStatus(data.Status),
		Participants: data.Participants,
		CreatedAt:    timestamppb.New(data.CreatedAt),
	}

	// Add completion time if available
	if data.CompletedAt != nil {
		response.CompletedAt = timestamppb.New(*data.CompletedAt)
	}

	// Add error if available
	if data.Error != "" {
		response.Error = &data.Error
	}

	// Add result based on operation type if available
	if data.Result != nil {
		switch data.Type {
		case tss.OperationKeygen:
			if keygenResult, ok := data.Result.(*tss.KeygenResult); ok {
				response.Result = &tssv1.GetOperationResponse_KeygenResult{
					KeygenResult: &tssv1.KeygenResult{
						PublicKey: keygenResult.PublicKey,
						KeyId:     keygenResult.KeyID,
					},
				}
			}
		case tss.OperationSigning:
			if signingResult, ok := data.Result.(*tss.SigningResult); ok {
				response.Result = &tssv1.GetOperationResponse_SigningResult{
					SigningResult: &tssv1.SigningResult{
						Signature: signingResult.Signature,
						R:         signingResult.R,
						S:         signingResult.S,
						V:         int32(signingResult.V),
					},
				}
			}
		case tss.OperationResharing:
			if resharingResult, ok := data.Result.(*tss.KeygenResult); ok {
				response.Result = &tssv1.GetOperationResponse_ResharingResult{
					ResharingResult: &tssv1.KeygenResult{
						PublicKey: resharingResult.PublicKey,
						KeyId:     resharingResult.KeyID,
					},
				}
			}
		}
	}

	// Add original request if available
	if data.Request != nil {
		switch req := data.Request.(type) {
		case *tss.KeygenRequest:
			response.Request = &tssv1.GetOperationResponse_KeygenRequest{
				KeygenRequest: &tssv1.StartKeygenRequest{
					Threshold:    int32(req.Threshold),
					Participants: req.Participants,
				},
			}
		case *tss.SigningRequest:
			response.Request = &tssv1.GetOperationResponse_SigningRequest{
				SigningRequest: &tssv1.StartSigningRequest{
					Message:      req.Message,
					KeyId:        req.KeyID,
					Participants: req.Participants,
				},
			}
		case *tss.ResharingRequest:
			response.Request = &tssv1.GetOperationResponse_ResharingRequest{
				ResharingRequest: &tssv1.StartResharingRequest{
					KeyId:           req.KeyID,
					NewThreshold:    int32(req.NewThreshold),
					OldParticipants: req.OldParticipants,
					NewParticipants: req.NewParticipants,
				},
			}
		}
	}

	return response
}
