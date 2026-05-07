package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/sangchenglong/kapi/internal/dao"
	"github.com/sangchenglong/kapi/internal/model"
)

type OpLogService struct {
	opLogDAO *dao.OperationLogDAO
}

func NewOpLogService(opLogDAO *dao.OperationLogDAO) *OpLogService {
	return &OpLogService{opLogDAO: opLogDAO}
}

func (s *OpLogService) LogOperation(ctx context.Context, userID, sessionID uint64, opType, targetTable string, targetID uint64, before, after interface{}, triggerText string) error {
	beforeJSON := "null"
	afterJSON := "null"
	if before != nil {
		b, err := json.Marshal(before)
		if err != nil {
			log.Printf("warning: failed to marshal before snapshot: %v", err)
			beforeJSON = fmt.Sprintf(`{"raw": "%+v"}`, before)
		} else {
			beforeJSON = string(b)
		}
	}
	if after != nil {
		a, err := json.Marshal(after)
		if err != nil {
			log.Printf("warning: failed to marshal after snapshot: %v", err)
			afterJSON = fmt.Sprintf(`{"raw": "%+v"}`, after)
		} else {
			afterJSON = string(a)
		}
	}

	opLog := &model.OperationLog{
		UserID:         userID,
		SessionID:      sessionID,
		OperationType:  opType,
		TargetTable:    targetTable,
		TargetID:       targetID,
		BeforeSnapshot: beforeJSON,
		AfterSnapshot:  afterJSON,
		TriggerText:    triggerText,
	}
	return s.opLogDAO.Create(ctx, opLog)
}

func (s *OpLogService) GetHistory(ctx context.Context, userID uint64, limit int) ([]model.OperationLog, error) {
	return s.opLogDAO.ListByUser(ctx, userID, limit)
}

func (s *OpLogService) GetTargetHistory(ctx context.Context, userID uint64, table string, targetID uint64) ([]model.OperationLog, error) {
	return s.opLogDAO.ListByTarget(ctx, userID, table, targetID)
}
