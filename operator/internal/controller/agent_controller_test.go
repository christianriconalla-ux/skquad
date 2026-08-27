package controller

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	skquadv1 "github.com/rossbrigoli/skquad/operator/internal/api/v1"
)

func TestAgentReconcilerCreatesDeployment(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := skquadv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	squad := &skquadv1.Squad{
		ObjectMeta: metav1.ObjectMeta{Name: "squad-test", Namespace: "skquad-system"},
		Spec: skquadv1.SquadSpec{
			SquadID:   "11111111-1111-1111-1111-111111111111",
			Namespace: "squad-runtime-test",
		},
	}
	agent := &skquadv1.Agent{
		ObjectMeta: metav1.ObjectMeta{Name: "agent-test", Namespace: "skquad-system"},
		Spec: skquadv1.AgentSpec{
			AgentID:           "22222222-2222-2222-2222-222222222222",
			SquadID:           squad.Spec.SquadID,
			Role:              "worker",
			DefaultProviderID: "provider-id",
			Image:             "example.com/skquad/agent:test",
			IdleTimeout:       "300s",
			DesiredActive:     true,
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(squad, agent).
		Build()
	reconciler := &AgentReconciler{Client: k8sClient, Scheme: scheme}

	if _, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace},
	}); err != nil {
		t.Fatal(err)
	}

	var deployment appsv1.Deployment
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Name: agent.Name, Namespace: squad.Spec.Namespace}, &deployment); err != nil {
		t.Fatal(err)
	}
	if deployment.Spec.Replicas == nil || *deployment.Spec.Replicas != 1 {
		t.Fatalf("deployment replicas = %v, want 1", deployment.Spec.Replicas)
	}
	if got := deployment.Spec.Template.Spec.ServiceAccountName; got != agentServiceAccountName {
		t.Fatalf("service account = %q, want %q", got, agentServiceAccountName)
	}
	if got := deployment.Spec.Template.Spec.Containers[0].Image; got != agent.Spec.Image {
		t.Fatalf("container image = %q, want %q", got, agent.Spec.Image)
	}
	if got := deployment.Spec.Template.Labels["skquad.io/agent-id"]; got != agent.Spec.AgentID {
		t.Fatalf("agent label = %q, want %q", got, agent.Spec.AgentID)
	}
}

func TestAgentReconcilerScalesInactiveAgentToZero(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := skquadv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	agent := &skquadv1.Agent{
		ObjectMeta: metav1.ObjectMeta{Name: "agent-zero", Namespace: "skquad-system"},
		Spec: skquadv1.AgentSpec{
			AgentID: "33333333-3333-3333-3333-333333333333",
			SquadID: "44444444-4444-4444-4444-444444444444",
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(agent).
		Build()
	reconciler := &AgentReconciler{Client: k8sClient, Scheme: scheme}

	if _, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace},
	}); err != nil {
		t.Fatal(err)
	}

	var deployment appsv1.Deployment
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Name: agent.Name, Namespace: "squad-" + agent.Spec.SquadID}, &deployment); err != nil {
		t.Fatal(err)
	}
	if deployment.Spec.Replicas == nil || *deployment.Spec.Replicas != 0 {
		t.Fatalf("deployment replicas = %v, want 0", deployment.Spec.Replicas)
	}
	if got := deployment.Spec.Template.Spec.Containers[0].Image; got != defaultAgentImage {
		t.Fatalf("default image = %q, want %q", got, defaultAgentImage)
	}
}
