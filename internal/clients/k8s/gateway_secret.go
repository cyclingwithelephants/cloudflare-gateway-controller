package k8s

import (
	"crypto/rand"
	"encoding/json"
	"math/big"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type TunnelSecretData struct {
	Secret    string `json:"TunnelSecret"`
	TunnelID  string `json:"TunnelID"`
	AccountID string `json:"AccountTag"`
}

func NewTunnelSecretData(tunnelID string, accountID string, tunnelSecret string) (*TunnelSecretData, error) {
	return &TunnelSecretData{
		Secret:    tunnelSecret,
		TunnelID:  tunnelID,
		AccountID: accountID,
	}, nil
}

func (t *TunnelSecretData) ToJSON() (string, error) {
	JSON, err := json.Marshal(t)
	if err != nil {
		return "", err
	}
	return string(JSON), nil
}

func secretName(deploymentName string) string {
	return deploymentName + "-secret"
}

func BuildTunnelSecret(
	deploymentName string,
	namespace string,
	data *TunnelSecretData,
) (*corev1.Secret, error) {
	jsonData, err := data.ToJSON()
	if err != nil {
		return nil, err
	}

	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName(deploymentName),
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/deploymentName": deploymentName,
			},
		},
		StringData: map[string]string{"creds.json": jsonData},
	}, nil
}

// GenerateRandomString returns a securely generated random string.
// It will return an error if the system's secure random
// number generator fails to function correctly, in which
// case the caller should not continue.
func GenerateRandomString(n int) (string, error) {
	const letters = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz-"
	ret := make([]byte, n)
	for i := 0; i < n; i++ {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(letters))))
		if err != nil {
			return "", err
		}
		ret[i] = letters[num.Int64()]
	}

	return string(ret), nil
}
