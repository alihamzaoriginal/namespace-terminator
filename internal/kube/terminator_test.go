package kube

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func TestResolveTargetsExplicitNamesAreSortedAndUnique(t *testing.T) {
	t.Parallel()

	client := fake.NewSimpleClientset()

	targets, err := ResolveTargets(context.Background(), client, []string{"beta", "alpha", "beta"}, false)
	if err != nil {
		t.Fatalf("ResolveTargets() error = %v", err)
	}

	want := []string{"alpha", "beta"}
	if len(targets) != len(want) {
		t.Fatalf("ResolveTargets() len = %d, want %d", len(targets), len(want))
	}

	for i := range want {
		if targets[i] != want[i] {
			t.Fatalf("ResolveTargets()[%d] = %q, want %q", i, targets[i], want[i])
		}
	}
}

func TestResolveTargetsAllTerminatingFiltersNamespaces(t *testing.T) {
	t.Parallel()

	client := fake.NewSimpleClientset(
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "active"},
			Status:     corev1.NamespaceStatus{Phase: corev1.NamespaceActive},
		},
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "terminating"},
			Status:     corev1.NamespaceStatus{Phase: corev1.NamespaceTerminating},
		},
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "deleting",
				DeletionTimestamp: &metav1.Time{Time: metav1.Now().Time},
			},
			Status: corev1.NamespaceStatus{Phase: corev1.NamespaceActive},
		},
	)

	targets, err := ResolveTargets(context.Background(), client, nil, true)
	if err != nil {
		t.Fatalf("ResolveTargets() error = %v", err)
	}

	want := []string{"deleting", "terminating"}
	if len(targets) != len(want) {
		t.Fatalf("ResolveTargets() len = %d, want %d", len(targets), len(want))
	}

	for i := range want {
		if targets[i] != want[i] {
			t.Fatalf("ResolveTargets()[%d] = %q, want %q", i, targets[i], want[i])
		}
	}
}

func TestResolveTargetsAllTerminatingWithoutMatchesErrors(t *testing.T) {
	t.Parallel()

	client := fake.NewSimpleClientset(
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "active"},
			Status:     corev1.NamespaceStatus{Phase: corev1.NamespaceActive},
		},
	)

	_, err := ResolveTargets(context.Background(), client, nil, true)
	if !errors.Is(err, ErrNoTerminatingNamespaces) {
		t.Fatalf("ResolveTargets() error = %v, want %v", err, ErrNoTerminatingNamespaces)
	}
}

func TestRunDryRunReturnsTargetsWithoutMutating(t *testing.T) {
	t.Parallel()

	client := fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "team-a"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "team-b"}},
	)

	response, err := Run(context.Background(), client, RunRequest{
		Names:  []string{"team-b", "team-a"},
		DryRun: true,
	}, DefaultTimeout)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if !response.DryRun {
		t.Fatalf("Run() DryRun = false, want true")
	}

	wantTargets := []string{"team-a", "team-b"}
	for i := range wantTargets {
		if response.Targets[i] != wantTargets[i] {
			t.Fatalf("Run() Targets[%d] = %q, want %q", i, response.Targets[i], wantTargets[i])
		}
		if response.Results[i].Status != "dry_run" {
			t.Fatalf("Run() Results[%d].Status = %q, want dry_run", i, response.Results[i].Status)
		}
	}
}

func TestBuildRESTConfigErrorsWhenCurrentContextIsMissing(t *testing.T) {
	t.Parallel()

	kubeconfigPath := filepath.Join(t.TempDir(), "config")
	config := clientcmdapi.Config{
		Contexts: map[string]*clientcmdapi.Context{
			"dev": {Cluster: "cluster", AuthInfo: "user"},
		},
		Clusters: map[string]*clientcmdapi.Cluster{
			"cluster": {Server: "https://example.invalid"},
		},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			"user": {},
		},
	}

	if err := clientcmd.WriteToFile(config, kubeconfigPath); err != nil {
		t.Fatalf("WriteToFile() error = %v", err)
	}

	_, err := BuildRESTConfig(Config{Kubeconfig: kubeconfigPath})
	if err == nil || !strings.Contains(err.Error(), "kubernetes context is not set") {
		t.Fatalf("BuildRESTConfig() error = %v, want context is not set", err)
	}
}

func TestBuildRESTConfigErrorsWhenNamedContextIsMissing(t *testing.T) {
	t.Parallel()

	kubeconfigPath := filepath.Join(t.TempDir(), "config")
	config := clientcmdapi.Config{
		CurrentContext: "dev",
		Contexts: map[string]*clientcmdapi.Context{
			"dev": {Cluster: "cluster", AuthInfo: "user"},
		},
		Clusters: map[string]*clientcmdapi.Cluster{
			"cluster": {Server: "https://example.invalid"},
		},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			"user": {},
		},
	}

	if err := clientcmd.WriteToFile(config, kubeconfigPath); err != nil {
		t.Fatalf("WriteToFile() error = %v", err)
	}

	_, err := BuildRESTConfig(Config{Kubeconfig: kubeconfigPath, Context: "prod"})
	if err == nil || !strings.Contains(err.Error(), `kubernetes context "prod" was not found`) {
		t.Fatalf("BuildRESTConfig() error = %v, want missing context", err)
	}
}
