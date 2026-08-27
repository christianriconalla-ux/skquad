// Package controller contains skquad Kubernetes reconcilers.
package controller

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	skquadv1 "github.com/rossbrigoli/skquad/operator/internal/api/v1"
)

const (
	managedBy                 = "skquad-operator"
	agentServiceAccountName   = "skquad-agent"
	defaultDenyPolicyName     = "default-deny"
	defaultSquadQuotaName     = "skquad-squad-quota"
	defaultSquadPodQuota      = "20"
	defaultSquadCPURequests   = "4"
	defaultSquadMemoryRequest = "8Gi"
	defaultSquadCPULimits     = "8"
	defaultSquadMemoryLimits  = "16Gi"
)

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
		ensureSquadLabels(&namespace.Labels, &squad)
		return nil
	}); err != nil {
		return ctrl.Result{}, err
	}

	if err := r.ensureAgentServiceAccount(ctx, &squad, namespaceName); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.ensureDefaultDenyNetworkPolicy(ctx, &squad, namespaceName); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.ensureResourceQuota(ctx, &squad, namespaceName); err != nil {
		return ctrl.Result{}, err
	}

	squad.Status.Namespace = namespaceName
	squad.Status.Ready = true
	squad.Status.Phase = "Ready"
	squad.Status.Reason = "BaseResourcesReady"
	squad.Status.UpdatedAt = metav1.Now()
	setCondition(&squad.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		Reason:             "BaseResourcesReady",
		Message:            fmt.Sprintf("Namespace %s base resources are ready", namespaceName),
		ObservedGeneration: squad.Generation,
	})
	if err := r.Status().Update(ctx, &squad); err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *SquadReconciler) ensureAgentServiceAccount(ctx context.Context, squad *skquadv1.Squad, namespace string) error {
	serviceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: agentServiceAccountName, Namespace: namespace},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, serviceAccount, func() error {
		ensureSquadLabels(&serviceAccount.Labels, squad)
		return nil
	})
	return err
}

func (r *SquadReconciler) ensureDefaultDenyNetworkPolicy(ctx context.Context, squad *skquadv1.Squad, namespace string) error {
	policy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: defaultDenyPolicyName, Namespace: namespace},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, policy, func() error {
		ensureSquadLabels(&policy.Labels, squad)
		policy.Spec = networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			},
		}
		return nil
	})
	return err
}

func (r *SquadReconciler) ensureResourceQuota(ctx context.Context, squad *skquadv1.Squad, namespace string) error {
	quota := &corev1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{Name: defaultSquadQuotaName, Namespace: namespace},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, quota, func() error {
		ensureSquadLabels(&quota.Labels, squad)
		quota.Spec.Hard = corev1.ResourceList{
			corev1.ResourcePods:           resource.MustParse(defaultSquadPodQuota),
			corev1.ResourceRequestsCPU:    resource.MustParse(defaultSquadCPURequests),
			corev1.ResourceRequestsMemory: resource.MustParse(defaultSquadMemoryRequest),
			corev1.ResourceLimitsCPU:      resource.MustParse(defaultSquadCPULimits),
			corev1.ResourceLimitsMemory:   resource.MustParse(defaultSquadMemoryLimits),
		}
		return nil
	})
	return err
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

func ensureSquadLabels(labels *map[string]string, squad *skquadv1.Squad) {
	if *labels == nil {
		*labels = map[string]string{}
	}
	(*labels)["app.kubernetes.io/managed-by"] = managedBy
	(*labels)["skquad.io/squad-id"] = squad.Spec.SquadID
	(*labels)["skquad.io/squad-resource"] = squad.Name
}
