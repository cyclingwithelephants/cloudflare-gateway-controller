package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/cloudflare/cloudflare-go"
	"github.com/cyclingwithelephants/cloudflare-gateway-controller/internal/clients/cf"
	k8s2 "github.com/cyclingwithelephants/cloudflare-gateway-controller/internal/clients/k8s"
	"github.com/cyclingwithelephants/cloudflare-gateway-controller/internal/controller"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// ReconciliationLoop represents the data used for a single reconciliation loop
type ReconciliationLoop struct {
	logger           logr.Logger
	GatewayName      string
	GatewayNamespace string
	tunnelID         string
	tunnelSecret     string
	accountID        string
	api              *cf.Api
}

// Reconciler reconciles a Gateway object
type Reconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Loop   *ReconciliationLoop
}

func (r *Reconciler) isMine(ctx context.Context, gateway *gatewayv1.Gateway) (bool, error) {
	if gateway == nil {
		return false, errors.New("returned nil gateway")
	}

	var gatewayClass *gatewayv1.GatewayClass
	if err := r.Get(
		ctx,
		client.ObjectKey{Name: string(gateway.Spec.GatewayClassName)},
		gatewayClass,
	); err != nil {
		return false, errors.Wrap(err, "failed to get gateway")
	}
	if gatewayClass == nil {
		return false, errors.New("returned nil gatewayClass")
	}
	if gatewayClass.Spec.ControllerName == "" {
		return false, errors.New("returned gatewayClass has no controllerName")
	}
	return gatewayClass.Spec.ControllerName == controller.Name, nil
}

func (r *Reconciler) getConfigFromGatewayClass(ctx context.Context, gateway *gatewayv1.Gateway) (k8s2.GatewayClassConfig, error) {
	if gateway == nil {
		return k8s2.GatewayClassConfig{}, errors.New("nil gateway")
	}
	gatewayClassName := gateway.Spec.GatewayClassName
	if gatewayClassName == "" {
		return k8s2.GatewayClassConfig{}, errors.New("gateway class name is empty")
	}

	gatewayClass := &gatewayv1.GatewayClass{}
	err := r.Client.Get(ctx, client.ObjectKey{Name: string(gatewayClassName)}, gatewayClass)
	if err != nil {
		return k8s2.GatewayClassConfig{}, errors.Wrap(err, "failed to get gateway class")
	}

	var namespace string
	if gatewayClass.Spec.ParametersRef.Namespace == nil {
		namespace = "default"
	} else {
		namespace = string(*gatewayClass.Spec.ParametersRef.Namespace)
	}
	secret := &corev1.Secret{}
	err = r.Client.Get(
		ctx,
		client.ObjectKey{
			Name:      gatewayClass.Spec.ParametersRef.Name,
			Namespace: namespace,
		},
		secret,
	)
	if err != nil {
		return k8s2.GatewayClassConfig{}, errors.Wrap(err, "failed to get secret")
	}

	cloudflareApiToken := string(secret.Data["api_token"])
	if cloudflareApiToken == "" {
		return k8s2.GatewayClassConfig{}, errors.New("cloudflare api token is empty")
	}
	domain := string(secret.Data["domain"])
	if domain == "" {
		return k8s2.GatewayClassConfig{}, errors.New("domain is empty")
	}
	emailAddress := string(secret.Data["email"])
	if emailAddress == "" {
		return k8s2.GatewayClassConfig{}, errors.New("email is empty")
	}
	cloudflareAccountId := string(secret.Data["account_id"])
	if cloudflareAccountId == "" {
		return k8s2.GatewayClassConfig{}, errors.New("cloudflare account id is empty")
	}

	return k8s2.GatewayClassConfig{
		CloudflareApiToken:  cloudflareApiToken,
		Domain:              domain,
		EmailAddress:        emailAddress,
		CloudflareAccountId: cloudflareAccountId,
	}, nil
}

func (r *Reconciler) ensureTunnelDeployment() error {
	existingDeployment := &appsv1.Deployment{}
	expectedDeployment := k8s2.BuildTunnelDeployment(r.Loop.GatewayName, r.Loop.GatewayNamespace, r.Loop.tunnelID)

	if err := r.Client.Get(context.Background(), client.ObjectKey{Name: expectedDeployment.Name, Namespace: r.Loop.GatewayNamespace}, existingDeployment); err != nil {
		if !apierrors.IsNotFound(err) {
			return errors.Wrap(err, "failed to get existing deployment")
		}
		r.Loop.logger.Info("existing deployment not found, creating")
		if err := r.Client.Create(context.Background(), expectedDeployment); err != nil {
			return errors.Wrap(err, "failed to create expectedDeployment")
		}
	} else {
		if err := r.Client.Update(context.Background(), expectedDeployment); err != nil {
			return errors.Wrap(err, "failed to update expectedDeployment")
		}
	}
	return nil
}

func (r *Reconciler) ensureCloudflareTunnel() (tunnel cloudflare.Tunnel, err error) {
	exists, existingTunnel, err := r.Loop.api.TunnelExists(r.Loop.GatewayName)
	if err != nil {
		return cloudflare.Tunnel{}, errors.Wrap(err, "failed to check if existingTunnel exists")
	}
	if exists {
		return existingTunnel, nil
	}

	newTunnel, err := r.Loop.api.CreateTunnel(r.Loop.GatewayName, r.Loop.tunnelSecret)
	if err != nil {
		return cloudflare.Tunnel{}, errors.Wrap(err, "failed to create cloudflare tunnel")
	}
	return newTunnel, nil
}

func (r *Reconciler) ensureTunnelSecret() error {
	secretData, err := k8s2.NewTunnelSecretData(r.Loop.tunnelID, r.Loop.accountID, r.Loop.tunnelSecret)
	if err != nil {
		return errors.Wrap(err, "failed to create tunnel secret data")
	}
	secret, err := k8s2.BuildTunnelSecret(
		r.Loop.GatewayName,
		r.Loop.GatewayNamespace,
		secretData,
	)
	if err != nil {
		return errors.Wrap(err, "failed to serialize tunnel secret")
	}

	if err := r.Client.Get(context.Background(), client.ObjectKey{Name: secret.Name, Namespace: r.Loop.GatewayNamespace}, secret); err != nil {
		if !apierrors.IsNotFound(err) {
			return errors.Wrap(err, "failed to get existing secret")
		}
		r.Loop.logger.Info("existing secret not found, creating")
		if err := r.Client.Create(context.Background(), secret); err != nil {
			return errors.Wrap(err, "failed to create secret")
		}
	}
	return nil
}

func (r *Reconciler) createInitialConfigMapIfNotExists(ctx context.Context) error {
	existingConfigMap := &corev1.ConfigMap{}
	err := r.Client.Get(
		ctx,
		client.ObjectKey{
			Name:      k8s2.ConfigMapName(r.Loop.GatewayName),
			Namespace: r.Loop.GatewayNamespace,
		},
		existingConfigMap,
	)
	if err == nil {
		r.Loop.logger.Info("existing configmap already exists")
		return nil
	}
	if err != nil && !apierrors.IsNotFound(err) {
		return errors.Wrap(err, "failed to get existing configmap")
	}
	configFile, err := cf.NewTunnelConfigFile(
		r.Loop.tunnelID,
		[]cf.IngressConfig{},
	)
	if err != nil {
		return errors.Wrap(err, "failed to create tunnel config file")
	}
	configFileJsonBytes, err := json.Marshal(configFile)
	if err != nil {
		return errors.Wrap(err, "failed to serialize config")
	}
	newConfigMap, err := k8s2.BuildTunnelConfigMap(
		r.Loop.GatewayName,
		r.Loop.GatewayNamespace,
		string(configFileJsonBytes),
	)
	if err != nil {
		return errors.Wrap(err, "failed to generate configmap definition")
	}
	if err := r.Client.Create(ctx, newConfigMap); err != nil {
		return errors.Wrap(err, "failed to create configmap")
	}
	return nil
}

// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployment,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=configmap,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secret,verbs=create;list;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Gateway object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.0/pkg/reconcile
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	defaultResult := ctrl.Result{Requeue: true, RequeueAfter: 60 * time.Second}
	r.Loop = &ReconciliationLoop{
		logger: log.FromContext(ctx),
	}
	r.Loop.logger.Info(fmt.Sprintf("Reconciling Gateway: %s", req.NamespacedName))

	gateway := &gatewayv1.Gateway{}
	if err := r.Get(ctx, req.NamespacedName, gateway); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	isMine, err := r.isMine(ctx, gateway)
	if err != nil {
		r.Loop.logger.Error(err, "failed to check if the httpRoute is mine")
		return defaultResult, err
	}
	if !isMine {
		// Don't requeue this for processing if we don't own this resource
		r.Loop.logger.Info(fmt.Sprintf("httpRoute %s is not mine", req.NamespacedName))
		return ctrl.Result{}, nil
	}

	// gateway cfTunnelExists
	r.Loop.GatewayName = gateway.ObjectMeta.Name
	r.Loop.GatewayNamespace = gateway.ObjectMeta.Namespace
	gatewayClassConfig, err := r.getConfigFromGatewayClass(ctx, gateway)
	if err != nil {
		r.Loop.logger.Error(err, "failed to get gateway class config")
		return defaultResult, nil
	}

	tunnelSecret, err := k8s2.GenerateRandomString(32)
	if err != nil {
		r.Loop.logger.Error(err, "failed to generate tunnel secret")
		return defaultResult, nil
	}
	r.Loop.tunnelSecret = tunnelSecret

	api, err := cf.NewAPI(
		ctx,
		gatewayClassConfig.CloudflareApiToken,
		gatewayClassConfig.CloudflareAccountId,
	)
	if err != nil {
		r.Loop.logger.Error(err, "failed to create cloudflare api")
		return defaultResult, nil
	}
	r.Loop.api = api

	tunnel, err := r.ensureCloudflareTunnel()
	if err != nil {
		r.Loop.logger.Error(err, "failed to deploy tunnel")
		return defaultResult, nil
	}
	r.Loop.tunnelID = tunnel.ID

	if err := r.createInitialConfigMapIfNotExists(ctx); err != nil {
		r.Loop.logger.Error(err, "failed to create initial configmap")
		return defaultResult, nil
	}

	if err := r.ensureTunnelSecret(); err != nil {
		r.Loop.logger.Error(err, "failed to create tunnel secret")
		return defaultResult, nil
	}
	if err := r.ensureTunnelDeployment(); err != nil {
		r.Loop.logger.Error(err, "failed to ensure tunnel deployment")
		return defaultResult, nil
	}
	return defaultResult, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&gatewayv1.Gateway{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Secret{}).
		Owns(&appsv1.Deployment{}).
		Complete(r)
}
