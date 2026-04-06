package kube

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const DefaultTimeout = 15 * time.Second

type Config struct {
	Kubeconfig string
	Context    string
}

type TerminationMode string

const (
	ModeExplicitNames  TerminationMode = "explicit_names"
	ModeAllTerminating TerminationMode = "all_terminating"
)

type RunRequest struct {
	Names          []string
	AllTerminating bool
	DryRun         bool
}

type RunResponse struct {
	Mode    TerminationMode `json:"mode"`
	DryRun  bool            `json:"dry_run"`
	Targets []string        `json:"targets"`
	Results []Result        `json:"results"`
}

type Result struct {
	Namespace string `json:"namespace"`
	Action    string `json:"action"`
	Status    string `json:"status"`
	Message   string `json:"message,omitempty"`
}

func NewClientset(cfg Config) (*kubernetes.Clientset, error) {
	restConfig, err := BuildRESTConfig(cfg)
	if err != nil {
		return nil, err
	}

	return kubernetes.NewForConfig(restConfig)
}

func BuildRESTConfig(cfg Config) (*rest.Config, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	loadingRules.ExplicitPath = cfg.Kubeconfig

	overrides := &clientcmd.ConfigOverrides{}
	if cfg.Context != "" {
		overrides.CurrentContext = cfg.Context
	}

	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides)

	restConfig, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("load kubernetes config: %w", err)
	}

	return restConfig, nil
}

func Run(ctx context.Context, client kubernetes.Interface, req RunRequest, timeout time.Duration) (RunResponse, error) {
	mode := resolveMode(req.AllTerminating)

	targets, err := ResolveTargets(ctx, client, req.Names, req.AllTerminating)
	if err != nil {
		return RunResponse{}, err
	}

	response := RunResponse{
		Mode:    mode,
		DryRun:  req.DryRun,
		Targets: targets,
		Results: make([]Result, 0, len(targets)),
	}

	if req.DryRun {
		for _, namespace := range targets {
			response.Results = append(response.Results, Result{
				Namespace: namespace,
				Action:    "terminate",
				Status:    "dry_run",
				Message:   "namespace would be force-terminated",
			})
		}
		return response, nil
	}

	for _, namespace := range targets {
		result := terminateNamespace(ctx, client, namespace, timeout)
		response.Results = append(response.Results, result)
	}

	return response, nil
}

func ResolveTargets(ctx context.Context, client kubernetes.Interface, names []string, allTerminating bool) ([]string, error) {
	switch {
	case allTerminating:
		items, err := client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
		if err != nil {
			return nil, fmt.Errorf("list namespaces: %w", err)
		}

		targets := make([]string, 0, len(items.Items))
		for _, item := range items.Items {
			if IsNamespaceTerminating(&item) {
				targets = append(targets, item.Name)
			}
		}

		sort.Strings(targets)
		return targets, nil
	case len(names) > 0:
		return uniqueSorted(names), nil
	default:
		return nil, errors.New("provide one or more namespace names or set --all-terminating")
	}
}

func IsNamespaceTerminating(namespace *corev1.Namespace) bool {
	if namespace == nil {
		return false
	}

	return namespace.Status.Phase == corev1.NamespaceTerminating || namespace.DeletionTimestamp != nil
}

func HasBulkOperation(targets []string, allTerminating bool) bool {
	return allTerminating || len(targets) > 1
}

func HasFailures(results []Result) bool {
	for _, result := range results {
		switch result.Status {
		case "failed", "pending":
			return true
		}
	}

	return false
}

func resolveMode(allTerminating bool) TerminationMode {
	if allTerminating {
		return ModeAllTerminating
	}

	return ModeExplicitNames
}

func terminateNamespace(ctx context.Context, client kubernetes.Interface, namespace string, timeout time.Duration) Result {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}

	current, err := client.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return Result{
				Namespace: namespace,
				Action:    "terminate",
				Status:    "failed",
				Message:   "namespace not found",
			}
		}

		return Result{
			Namespace: namespace,
			Action:    "terminate",
			Status:    "failed",
			Message:   fmt.Sprintf("read namespace: %v", err),
		}
	}

	if current.DeletionTimestamp == nil {
		if err := client.CoreV1().Namespaces().Delete(ctx, namespace, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			return Result{
				Namespace: namespace,
				Action:    "terminate",
				Status:    "failed",
				Message:   fmt.Sprintf("request deletion: %v", err),
			}
		}
	}

	latest, err := waitForNamespaceTermination(ctx, client, namespace, current, timeout)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return Result{
				Namespace: namespace,
				Action:    "terminate",
				Status:    "deleted",
				Message:   "namespace already removed",
			}
		}

		return Result{
			Namespace: namespace,
			Action:    "terminate",
			Status:    "failed",
			Message:   fmt.Sprintf("refresh namespace: %v", err),
		}
	}

	finalized := latest.DeepCopy()
	finalized.Spec.Finalizers = nil
	finalized.Finalizers = nil

	if _, err := client.CoreV1().Namespaces().Finalize(ctx, finalized, metav1.UpdateOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return Result{
			Namespace: namespace,
			Action:    "terminate",
			Status:    "failed",
			Message:   fmt.Sprintf("clear finalizers: %v", err),
		}
	}

	waitErr := wait.PollUntilContextTimeout(ctx, time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		_, err := client.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		if err != nil {
			return false, err
		}
		return false, nil
	})

	switch {
	case waitErr == nil:
		return Result{
			Namespace: namespace,
			Action:    "terminate",
			Status:    "deleted",
			Message:   "namespace deleted after finalizers were cleared",
		}
	case errors.Is(waitErr, context.DeadlineExceeded):
		return Result{
			Namespace: namespace,
			Action:    "terminate",
			Status:    "pending",
			Message:   "finalizers cleared but namespace still exists after timeout",
		}
	default:
		return Result{
			Namespace: namespace,
			Action:    "terminate",
			Status:    "failed",
			Message:   fmt.Sprintf("wait for deletion: %v", waitErr),
		}
	}
}

func uniqueSorted(names []string) []string {
	seen := make(map[string]struct{}, len(names))
	targets := make([]string, 0, len(names))
	for _, name := range names {
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		targets = append(targets, name)
	}

	sort.Strings(targets)
	return targets
}

func waitForNamespaceTermination(ctx context.Context, client kubernetes.Interface, namespace string, fallback *corev1.Namespace, timeout time.Duration) (*corev1.Namespace, error) {
	if fallback != nil && fallback.DeletionTimestamp != nil {
		return fallback, nil
	}

	var latest *corev1.Namespace
	waitErr := wait.PollUntilContextTimeout(ctx, 250*time.Millisecond, timeout, true, func(ctx context.Context) (bool, error) {
		current, err := client.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return false, err
		}
		if err != nil {
			return false, err
		}

		latest = current
		return current.DeletionTimestamp != nil, nil
	})
	if waitErr != nil {
		if latest != nil {
			return latest, nil
		}
		return nil, waitErr
	}

	return latest, nil
}
