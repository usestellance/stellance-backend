package mail

import (
	"fmt"
	"time"

	"github.com/The-True-Hooha/stellance-backend.git/pkg/utils"
)

func CreateEmailToken(email string, userID string) (string, error) {
	payload := fmt.Sprintf("%s|%s|%d", email, userID, time.Now().Unix())
	return utils.EncryptEmail(payload)
}

func ParseVerificationToken(token string) (email string, userID string, err error) {
	payload, err := utils.DecryptEmail(token)
	if err != nil {
		return "", "", err
	}
	var timestamp int64
	_, err = fmt.Sscanf(payload, "%s|%s|%d", &email, &userID, &timestamp)
	if err != nil {
		return "", "", fmt.Errorf("invalid token payload")
	}

	return email, userID, nil
}