// Package controller contains skquad Kubernetes reconcilers.
package controller

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	skquadv1 "github.com/rossbrigoli/skquad/operator/internal/api/v1"
)

const managedBy = "skquad-operator"

// SquadReconciler reconciles Squad resources into isolated Kubernetes
// namespaces.
type SquadReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// Reconcile ensures the squad namespace exists and records basic status.
func (r *SquadReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var squad skquadv1.Squad
	if err := r.Get(ctx, req.NamespacedName, &squad); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	namespaceName := SquadNamespace(&squad)
	namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespaceName}}
	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, namespace, func() error {
		if namespace.Labels == nil {
			namespace.Labels = map[string]string{}
		}
		namespace.Labels["app.kubernetes.io/managed-by"] = managedBy
		namespace.Labels["skquad.io/squad-id"] = squad.Spec.SquadID
		namespace.Labels["skquad.io/squad-resource"] = squad.Name
		return nil
	}); err != nil {
		return ctrl.Result{}, err
	}

	squad.Status.Namespace = namespaceName
	squad.Status.Ready = true
	squad.Status.Phase = "Ready"
	squad.Status.Reason = "NamespaceReady"
	squad.Status.UpdatedAt = metav1.Now()
	setCondition(&squad.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		Reason:             "NamespaceReady",
		Message:            fmt.Sprintf("Namespace %s is ready", namespaceName),
		ObservedGeneration: squad.Generation,
	})
	if err := r.Status().Update(ctx, &squad); err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager registers the Squad controller with a controller-runtime
// manager.
func (r *SquadReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&skquadv1.Squad{}).
		Complete(r)
}

// SquadNamespace returns the namespace reconciled for a Squad.
func SquadNamespace(squad *skquadv1.Squad) string {
	if squad.Spec.Namespace != "" {
		return squad.Spec.Namespace
	}
	if squad.Spec.SquadID != "" {
		return "squad-" + squad.Spec.SquadID
	}
	return "squad-" + squad.Name
}

func setCondition(conditions *[]metav1.Condition, condition metav1.Condition) {
	condition.LastTransitionTime = metav1.Now()
	for i := range *conditions {
		if (*conditions)[i].Type == condition.Type {
			(*conditions)[i] = condition
			return
		}
	}
	*conditions = append(*conditions, condition)
}
