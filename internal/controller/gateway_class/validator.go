package gateway_class

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	corev1 "k8s.io/api/core/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type validator struct {
	Ctx          context.Context
	gatewayClass *gatewayv1.GatewayClass
	configSecret *corev1.Secret
}

func newGatewayClassValidator(
	ctx context.Context,
	gatewayClass *gatewayv1.GatewayClass,
	configSecret *corev1.Secret,
) *validator {
	return &validator{
		Ctx:          ctx,
		gatewayClass: gatewayClass,
		configSecret: configSecret,
	}
}

type cloudflareTokenValidationResponse struct {
	Success bool `json:"success"`
	Errors  []struct {
		Code       int    `json:"code"`
		Message    string `json:"message"`
		ErrorChain []struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error_chain"`
	} `json:"errors"`
	Messages []interface{} `json:"messages"`
	Result   interface{}   `json:"result"`
}

func (v *validator) validateConfig() error {
	cloudflareApiToken := string(v.configSecret.Data["api_token"])
	if cloudflareApiToken == "" {
		msg := "secret does not contain a CLOUDFLARE_API_TOKEN key"
		return errors.New(msg)
	}

	domain := string(v.configSecret.Data["domain"])
	if domain == "" {
		msg := "secret does not contain a domain key"
		return errors.New(msg)
	}

	email := string(v.configSecret.Data["email"])
	if email == "" {
		msg := "secret does not contain an email key"
		return errors.New(msg)
	}

	accountId := string(v.configSecret.Data["account_id"])
	if accountId == "" {
		msg := "secret does not contain an account_id key"
		return errors.New(msg)
	}
	return nil
}

func (v *validator) validateToken() error {
	req, err := http.NewRequest(http.MethodGet, "https://api.cloudflare.com/client/v4/user/tokens/verify", nil)
	if err != nil {
		return err
	}
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", string(v.configSecret.Data["api_token"])))
	req.Header.Add("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close() //nolint:errcheck
	// cloudflare doesn't expose a specific invalid response when the token isn't valid
	if resp.StatusCode == http.StatusBadRequest {
		return errors.New("invalid token")
	}
	if resp.StatusCode != http.StatusOK {
		var p cloudflareTokenValidationResponse
		err := json.NewDecoder(resp.Body).Decode(&p)
		if err != nil {
			return errors.New("could not parse response from cloudflare")
		}
		return fmt.Errorf(
			"failed cloudflare token validation: expected 200 OK , got %d: %s",
			resp.StatusCode,
			p.Errors[0].Message,
		)
	}
	return nil
}
