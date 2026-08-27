package controller

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	skquadv1 "github.com/rossbrigoli/skquad/operator/internal/api/v1"
)

func TestSquadReconcilerCreatesNamespace(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := skquadv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	squad := &skquadv1.Squad{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "squad-test",
			Namespace: "skquad-system",
		},
		Spec: skquadv1.SquadSpec{
			SquadID:   "11111111-1111-1111-1111-111111111111",
			OwnerRef:  "owner-id",
			Namespace: "squad-runtime-test",
			Status:    "active",
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(squad).
		Build()
	reconciler := &SquadReconciler{Client: k8sClient, Scheme: scheme}

	if _, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: squad.Name, Namespace: squad.Namespace},
	}); err != nil {
		t.Fatal(err)
	}

	var namespace corev1.Namespace
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Name: squad.Spec.Namespace}, &namespace); err != nil {
		t.Fatal(err)
	}
	if got := namespace.Labels["skquad.io/squad-id"]; got != squad.Spec.SquadID {
		t.Fatalf("namespace squad label = %q, want %q", got, squad.Spec.SquadID)
	}

	var serviceAccount corev1.ServiceAccount
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Name: agentServiceAccountName, Namespace: squad.Spec.Namespace}, &serviceAccount); err != nil {
		t.Fatal(err)
	}

	var policy networkingv1.NetworkPolicy
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Name: defaultDenyPolicyName, Namespace: squad.Spec.Namespace}, &policy); err != nil {
		t.Fatal(err)
	}
	if len(policy.Spec.Ingress) != 0 || len(policy.Spec.Egress) != 0 {
		t.Fatalf("default deny policy has ingress=%d egress=%d, want both empty", len(policy.Spec.Ingress), len(policy.Spec.Egress))
	}
	if got, want := policy.Spec.PolicyTypes, []networkingv1.PolicyType{networkingv1.PolicyTypeIngress, networkingv1.PolicyTypeEgress}; !policyTypesEqual(got, want) {
		t.Fatalf("policy types = %#v, want %#v", got, want)
	}

	var quota corev1.ResourceQuota
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Name: defaultSquadQuotaName, Namespace: squad.Spec.Namespace}, &quota); err != nil {
		t.Fatal(err)
	}
	if got := quota.Spec.Hard.Pods().String(); got != defaultSquadPodQuota {
		t.Fatalf("pod quota = %q, want %q", got, defaultSquadPodQuota)
	}
}

func TestSquadNamespaceFallsBackToSquadID(t *testing.T) {
	t.Parallel()

	squad := &skquadv1.Squad{
		ObjectMeta: metav1.ObjectMeta{Name: "fallback"},
		Spec: skquadv1.SquadSpec{
			SquadID: "22222222-2222-2222-2222-222222222222",
		},
	}
	if got, want := SquadNamespace(squad), "squad-22222222-2222-2222-2222-222222222222"; got != want {
		t.Fatalf("SquadNamespace() = %q, want %q", got, want)
	}
}

func policyTypesEqual(a, b []networkingv1.PolicyType) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
