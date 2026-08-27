package controller

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

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

	result, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: squad.Name, Namespace: squad.Namespace},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Requeue {
		t.Fatalf("first reconcile result = %#v, want requeue after finalizer add", result)
	}
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

	var dnsPolicy networkingv1.NetworkPolicy
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Name: dnsEgressPolicyName, Namespace: squad.Spec.Namespace}, &dnsPolicy); err != nil {
		t.Fatal(err)
	}
	if got := len(dnsPolicy.Spec.Egress); got != 1 {
		t.Fatalf("dns policy egress rules = %d, want 1", got)
	}
	if got := len(dnsPolicy.Spec.Egress[0].Ports); got != 2 {
		t.Fatalf("dns policy ports = %d, want 2", got)
	}
	if got := dnsPolicy.Spec.Egress[0].To[0].NamespaceSelector.MatchLabels["kubernetes.io/metadata.name"]; got != "kube-system" {
		t.Fatalf("dns egress namespace = %q, want kube-system", got)
	}

	var platformPolicy networkingv1.NetworkPolicy
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Name: platformEgressPolicyName, Namespace: squad.Spec.Namespace}, &platformPolicy); err != nil {
		t.Fatal(err)
	}
	if got := platformPolicy.Spec.Egress[0].To[0].NamespaceSelector.MatchLabels["kubernetes.io/metadata.name"]; got != squad.Namespace {
		t.Fatalf("platform egress namespace = %q, want %q", got, squad.Namespace)
	}
	if got := len(platformPolicy.Spec.Egress[0].Ports); got != 5 {
		t.Fatalf("platform egress ports = %d, want 5", got)
	}

	var quota corev1.ResourceQuota
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Name: defaultSquadQuotaName, Namespace: squad.Spec.Namespace}, &quota); err != nil {
		t.Fatal(err)
	}
	if got := quota.Spec.Hard.Pods().String(); got != defaultSquadPodQuota {
		t.Fatalf("pod quota = %q, want %q", got, defaultSquadPodQuota)
	}

	var updatedSquad skquadv1.Squad
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Name: squad.Name, Namespace: squad.Namespace}, &updatedSquad); err != nil {
		t.Fatal(err)
	}
	if !controllerutil.ContainsFinalizer(&updatedSquad, squadFinalizer) {
		t.Fatalf("squad finalizers = %#v, want %q", updatedSquad.Finalizers, squadFinalizer)
	}
}

func TestSquadReconcilerFinalizerDeletesManagedResources(t *testing.T) {
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
			Name:       "squad-delete",
			Namespace:  "skquad-system",
			Finalizers: []string{squadFinalizer},
		},
		Spec: skquadv1.SquadSpec{
			SquadID:   "33333333-3333-3333-3333-333333333333",
			Namespace: "squad-delete-test",
		},
	}
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(
			squad,
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: squad.Spec.Namespace}},
			&corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: agentServiceAccountName, Namespace: squad.Spec.Namespace}},
			&networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: defaultDenyPolicyName, Namespace: squad.Spec.Namespace}},
			&networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: dnsEgressPolicyName, Namespace: squad.Spec.Namespace}},
			&networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: platformEgressPolicyName, Namespace: squad.Spec.Namespace}},
			&corev1.ResourceQuota{ObjectMeta: metav1.ObjectMeta{Name: defaultSquadQuotaName, Namespace: squad.Spec.Namespace}},
		).
		Build()
	reconciler := &SquadReconciler{Client: k8sClient, Scheme: scheme}

	if err := k8sClient.Delete(context.Background(), squad); err != nil {
		t.Fatal(err)
	}
	if _, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: squad.Name, Namespace: squad.Namespace},
	}); err != nil {
		t.Fatal(err)
	}

	for _, obj := range []client.Object{
		&corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: agentServiceAccountName, Namespace: squad.Spec.Namespace}},
		&networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: defaultDenyPolicyName, Namespace: squad.Spec.Namespace}},
		&networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: dnsEgressPolicyName, Namespace: squad.Spec.Namespace}},
		&networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: platformEgressPolicyName, Namespace: squad.Spec.Namespace}},
		&corev1.ResourceQuota{ObjectMeta: metav1.ObjectMeta{Name: defaultSquadQuotaName, Namespace: squad.Spec.Namespace}},
	} {
		if err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(obj), obj); !apierrors.IsNotFound(err) {
			t.Fatalf("object %s/%s still exists or lookup failed: %v", obj.GetNamespace(), obj.GetName(), err)
		}
	}
	var namespace corev1.Namespace
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Name: squad.Spec.Namespace}, &namespace); !apierrors.IsNotFound(err) {
		t.Fatalf("namespace still exists or lookup failed: %v", err)
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
