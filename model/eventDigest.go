package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

func NotificationDigest(event *TaskEventType) (string, error) {
	body, err := json.Marshal(event)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:]), nil
}
