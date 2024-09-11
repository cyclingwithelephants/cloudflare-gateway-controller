package gateway_class

import (
	"context"
	"fmt"
	"time"

	"github.com/cyclingwithelephants/cloudflare-gateway-controller/internal/controller"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// Reconciler reconciles a GatewayClass object
type Reconciler struct {
	client.Client
	Scheme *runtime.Scheme
	logger logr.Logger
}

func (r *Reconciler) validate(ctx context.Context, gatewayClass *gatewayv1.GatewayClass) error {
	if gatewayClass == nil {
		return errors.New("nil GatewayClass")
	}
	secretObjectKey := client.ObjectKey{
		Namespace: "default",
		Name:      gatewayClass.Spec.ParametersRef.Name,
	}
	secret := &corev1.Secret{}
	if err := r.Get(ctx, secretObjectKey, secret); err != nil {
		return errors.Wrapf(err, "failed to retrieve secret %s/%s", secretObjectKey.Namespace, secretObjectKey.Name)
	}
	validator := newGatewayClassValidator(ctx, gatewayClass, secret)
	if err := validator.validateConfig(); err != nil {
		return errors.Wrapf(err, "failed to create gatewayclass validator")
	}
	if err := validator.validateToken(); err != nil {
		return errors.Wrapf(err, "failed to validate token")
	}

	return nil
}

func (r *Reconciler) reject(ctx context.Context, gatewayClass *gatewayv1.GatewayClass, message string) error {
	r.logger.Info("updating status")
	for i, condition := range gatewayClass.Status.Conditions {
		if condition.Type != "Accepted" {
			continue
		}
		r.logger.Info("Updating status")
		gatewayClass.Status.Conditions[i] = metav1.Condition{
			Type:               "Accepted",
			Status:             "False",
			LastTransitionTime: metav1.Time{Time: time.Now()},
			Reason:             "Invalid",
			Message:            message,
		}
	}

	if err := r.Status().Update(ctx, gatewayClass); err != nil {
		r.logger.Info(fmt.Sprintf("Failed to update condition status: %v\n", err))
		return err
	}

	return nil
}

func (r *Reconciler) accept(ctx context.Context, gatewayClass *gatewayv1.GatewayClass) error {
	r.logger.Info("updating status")
	for i, condition := range gatewayClass.Status.Conditions {
		if condition.Type != "Accepted" {
			continue
		}
		r.logger.Info("Updating status")
		gatewayClass.Status.Conditions[i] = metav1.Condition{
			Type:               "Accepted",
			Status:             "True",
			LastTransitionTime: metav1.Time{Time: time.Now()},
			Reason:             "Accepted",
			Message:            "this is a test acceptance",
		}
	}

	if err := r.Status().Update(ctx, gatewayClass); err != nil {
		r.logger.Info(fmt.Sprintf("Failed to update condition status: %v\n", err))
		return err
	}

	return nil
}

// +kubebuilder:rbac:groups=gateway.networking.k8s.io.adamland.xyz,resources=gatewayclasses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gateway.networking.k8s.io.adamland.xyz,resources=gatewayclasses/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=gateway.networking.k8s.io.adamland.xyz,resources=gatewayclasses/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the GatewayClass object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.0/pkg/reconcile
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	defaultResult := ctrl.Result{Requeue: true, RequeueAfter: 60 * time.Second}
	r.logger = log.FromContext(ctx)
	r.logger.Info(fmt.Sprintf("Reconciling GatewayClass: %s", req.NamespacedName))

	gatewayClass := &gatewayv1.GatewayClass{}
	if err := r.Get(ctx, req.NamespacedName, gatewayClass); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// don't reconcile gatewayClasses we don't own
	if gatewayClass.Spec.ControllerName != controller.Name {
		r.logger.Info(fmt.Sprintf("gatewayClass %s is not mine", req.NamespacedName))
		return ctrl.Result{}, nil
	}

	if err := r.validate(ctx, gatewayClass); err != nil {
		if err := r.reject(ctx, gatewayClass, err.Error()); err != nil {
			r.logger.Error(err, "failed to reject condition")
			return defaultResult, nil
		}
		r.logger.Error(err, "failed to validate GatewayClass")
		return defaultResult, nil
	}

	if err := r.accept(ctx, gatewayClass); err != nil {
		r.logger.Error(err, "failed to accept GatewayClass")
		return defaultResult, nil
	}

	return defaultResult, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&gatewayv1.GatewayClass{}).
		Owns(&corev1.Secret{}).
		Complete(r)
}
