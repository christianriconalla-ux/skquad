package controller

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	skquadv1 "github.com/rossbrigoli/skquad/operator/internal/api/v1"
)

const (
	agentContainerName = "agent"
	defaultAgentImage  = "skquad/agent-runtime:0.1.0"
)

// AgentReconciler reconciles Agent resources into per-agent Deployments.
type AgentReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// Reconcile ensures the agent Deployment exists in its squad namespace.
func (r *AgentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var agent skquadv1.Agent
	if err := r.Get(ctx, req.NamespacedName, &agent); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	namespace, err := r.squadNamespaceForAgent(ctx, &agent)
	if err != nil {
		return ctrl.Result{}, err
	}
	replicas := int32(0)
	if agent.Spec.DesiredActive {
		replicas = 1
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: agent.Name, Namespace: namespace},
	}
	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, deployment, func() error {
		labels := agentLabels(&agent)
		ensureAgentLabels(&deployment.Labels, &agent)
		deployment.Spec.Replicas = &replicas
		deployment.Spec.Selector = &metav1.LabelSelector{MatchLabels: labels}
		deployment.Spec.Template.ObjectMeta.Labels = labels
		deployment.Spec.Template.Spec.ServiceAccountName = agentServiceAccountName
		deployment.Spec.Template.Spec.Containers = []corev1.Container{{
			Name:  agentContainerName,
			Image: agentImage(&agent),
			Env: []corev1.EnvVar{
				{Name: "SKQUAD_AGENT_ID", Value: agent.Spec.AgentID},
				{Name: "SKQUAD_SQUAD_ID", Value: agent.Spec.SquadID},
				{Name: "SKQUAD_AGENT_ROLE", Value: agent.Spec.Role},
				{Name: "SKQUAD_DEFAULT_PROVIDER_ID", Value: agent.Spec.DefaultProviderID},
				{Name: "SKQUAD_IDLE_TIMEOUT", Value: agent.Spec.IdleTimeout},
			},
		}}
		return nil
	})
	if err != nil {
		return ctrl.Result{}, err
	}

	agent.Status.ReadyDeployment = deployment.Name
	agent.Status.Replicas = replicas
	agent.Status.Ready = true
	agent.Status.Phase = "Ready"
	agent.Status.Reason = "DeploymentReady"
	agent.Status.UpdatedAt = metav1.Now()
	setCondition(&agent.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		Reason:             "DeploymentReady",
		Message:            fmt.Sprintf("Deployment %s/%s is ready", namespace, deployment.Name),
		ObservedGeneration: agent.Generation,
	})
	if err := r.Status().Update(ctx, &agent); err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager registers the Agent controller with a controller-runtime
// manager.
func (r *AgentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&skquadv1.Agent{}).
		Complete(r)
}

func (r *AgentReconciler) squadNamespaceForAgent(ctx context.Context, agent *skquadv1.Agent) (string, error) {
	var squads skquadv1.SquadList
	if err := r.List(ctx, &squads, client.InNamespace(agent.Namespace)); err != nil {
		return "", err
	}
	for i := range squads.Items {
		if squads.Items[i].Spec.SquadID == agent.Spec.SquadID {
			return SquadNamespace(&squads.Items[i]), nil
		}
	}
	if agent.Spec.SquadID == "" {
		return "", fmt.Errorf("agent %s/%s has empty squadId", agent.Namespace, agent.Name)
	}
	return "squad-" + agent.Spec.SquadID, nil
}

func agentLabels(agent *skquadv1.Agent) map[string]string {
	return map[string]string{
		"app.kubernetes.io/managed-by": managedBy,
		"app.kubernetes.io/name":       "skquad-agent",
		"skquad.io/agent-id":           agent.Spec.AgentID,
		"skquad.io/squad-id":           agent.Spec.SquadID,
	}
}

func ensureAgentLabels(labels *map[string]string, agent *skquadv1.Agent) {
	if *labels == nil {
		*labels = map[string]string{}
	}
	for key, value := range agentLabels(agent) {
		(*labels)[key] = value
	}
}

func agentImage(agent *skquadv1.Agent) string {
	if agent.Spec.Image == "" {
		return defaultAgentImage
	}
	return agent.Spec.Image
}
