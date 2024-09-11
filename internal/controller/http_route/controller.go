package http_route

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/cyclingwithelephants/cloudflare-gateway-operator/internal/clients/cf"
	"github.com/cyclingwithelephants/cloudflare-gateway-operator/internal/clients/k8s"
	"github.com/cyclingwithelephants/cloudflare-gateway-operator/internal/controller"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// ReconciliationLoop represents the data used for a single reconciliation loop
type ReconciliationLoop struct {
	logger           logr.Logger
	GatewayName      string
	GatewayNamespace string
	// tunnelSecret     string
	// accountID        string
}

// Reconciler reconciles a HTTPRoute object
type Reconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Loop   *ReconciliationLoop
}

func (r *Reconciler) isMine(ctx context.Context, httpRoute *gatewayv1.HTTPRoute) (bool, error) {
	if httpRoute == nil {
		return false, errors.New("nil HTTPRoute")
	}
	var gateway *gatewayv1.Gateway
	if err := r.Get(
		ctx,
		client.ObjectKey{
			Namespace: string(*httpRoute.Spec.ParentRefs[0].Namespace),
			Name:      string(httpRoute.Spec.ParentRefs[0].Name),
		},
		gateway,
	); err != nil {
		return false, errors.Wrap(err, "failed to get gateway")
	}
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

func (r *Reconciler) getConfigMap(ctx context.Context, configMapName client.ObjectKey) (*corev1.ConfigMap, error) {
	configMap := &corev1.ConfigMap{}
	err := r.Client.Get(
		ctx,
		configMapName,
		configMap,
	)
	if err != nil {
		return &corev1.ConfigMap{}, errors.Wrap(err, "failed to get existing configmap")
	}
	return configMap, nil
}

func (r *Reconciler) upsertConfigMap(
	ctx context.Context,
	configMap *corev1.ConfigMap,
	newConfig []cf.IngressConfig,
) error {

	existingConfigString := configMap.Data[k8s.ConfigYamlFileName]
	var existingTunnelConfig cf.TunnelConfigFile
	err := json.Unmarshal([]byte(existingConfigString), &existingTunnelConfig)
	if err != nil {
		return errors.Wrap(err, "failed to unmarshal existing configmap")
	}

	// check which routes need to be updated or added
	var routesToAdd []cf.IngressConfig
	for _, newRoute := range newConfig {
		for _, existingRoute := range existingTunnelConfig.Ingress {
			if newRoute.Hostname != existingRoute.Hostname {
				continue
			}
			if newRoute.Service != existingRoute.Service {
				// an existing hostname is pointing to a new backing service
				existingRoute.Service = newRoute.Service
				continue
			}
			// this is a totally new route that needs to be added to the config file
			routesToAdd = append(routesToAdd, newRoute)
		}
	}

	newTunnelConfigFile := existingTunnelConfig
	newTunnelConfigFile.Ingress = append(existingTunnelConfig.Ingress, routesToAdd...)
	cf.Sort(newTunnelConfigFile.Ingress)
	newConfigBytes, err := json.Marshal(newTunnelConfigFile)
	if err != nil {
		return errors.Wrap(err, "failed to marshal new configmap")
	}
	newConfigString := string(newConfigBytes)

	// if there's no change, we don't perform an update
	if existingConfigString == newConfigString {
		return nil
	}

	configMap.Data[k8s.ConfigYamlFileName] = newConfigString
	return r.Update(ctx, configMap)
}

func (r *Reconciler) buildRoutingFragment(route *gatewayv1.HTTPRoute) []cf.IngressConfig {
	ingressConfigs := make([]cf.IngressConfig, len(route.Spec.Hostnames)+1)
	for i, hostname := range route.Spec.Hostnames {
		ingressConfigs[i] = cf.IngressConfig{
			Hostname: string(hostname),
			Service: fmt.Sprintf(
				"http://%s.%s.svc.cluster.local:%s",
				string(route.Spec.Rules[0].BackendRefs[0].Name),
				string(*route.Spec.Rules[0].BackendRefs[0].Namespace),
				string(*route.Spec.Rules[0].BackendRefs[0].Port),
			),
		}
	}
	return ingressConfigs
}

// +kubebuilder:rbac:groups=gateway.networking.k8s.io.adamland.xyz,resources=httproutes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gateway.networking.k8s.io.adamland.xyz,resources=httproutes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=gateway.networking.k8s.io.adamland.xyz,resources=httproutes/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the HTTPRoute object against the actual cluster state, and then
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

	var httpRoute *gatewayv1.HTTPRoute
	if err := r.Get(ctx, req.NamespacedName, httpRoute); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	isMine, err := r.isMine(ctx, httpRoute)
	if err != nil {
		r.Loop.logger.Error(err, "failed to check if the httpRoute is mine")
		return defaultResult, err
	}
	if !isMine {
		// Don't requeue this for processing if we don't own this resource
		r.Loop.logger.Info(fmt.Sprintf("httpRoute %s is not mine", req.NamespacedName))
		return ctrl.Result{}, nil
	}

	configMap, err := r.getConfigMap(ctx, req.NamespacedName)
	if err != nil {
		r.Loop.logger.Error(err, "failed to get configmap")
		return defaultResult, err
	}

	if err := r.upsertConfigMap(ctx, configMap, r.buildRoutingFragment(httpRoute)); err != nil {
		r.Loop.logger.Error(err, "failed to upsert configmap")
		return defaultResult, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		// Uncomment the following line adding a pointer to an instance of the controlled resource as an argument
		For(&gatewayv1.HTTPRoute{}).
		Owns(&corev1.ConfigMap{}).
		Complete(r)
}
