package controller

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
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
