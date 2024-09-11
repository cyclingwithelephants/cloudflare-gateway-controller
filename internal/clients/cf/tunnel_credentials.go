package cf

import (
	"encoding/json"

	"github.com/pkg/errors"
)

type TunnelCredentials struct {
	AccountID    string `json:"AccountTag"`
	TunnelSecret string `json:"TunnelSecret"`
	TunnelID     string `json:"TunnelID"`
}

func NewTunnelCredentials(
	accountID string,
	tunnelID string,
	tunnelSecret string,
) (*TunnelCredentials, error) {
	if accountID == "" {
		return &TunnelCredentials{}, errors.New("AccountID is empty")
	}
	if tunnelID == "" {
		return &TunnelCredentials{}, errors.New("TunnelID is empty")
	}
	if tunnelSecret == "" {
		return &TunnelCredentials{}, errors.New("TunnelSecret is empty")
	}
	return &TunnelCredentials{
		AccountID:    accountID,
		TunnelSecret: tunnelSecret,
		TunnelID:     tunnelID,
	}, nil
}

func NewTunnelCredentialsFromJSON(jsonString string) (*TunnelCredentials, error) {
	credentials := &TunnelCredentials{}
	err := json.Unmarshal([]byte(jsonString), credentials)
	if err != nil {
		return &TunnelCredentials{}, err
	}
	return credentials, nil
}

func (t *TunnelCredentials) ToJSON() (string, error) {
	jsonData, err := json.Marshal(t)
	if err != nil {
		return "", errors.Wrap(err, "failed to marshal configuration")
	}
	return string(jsonData), nil
}
