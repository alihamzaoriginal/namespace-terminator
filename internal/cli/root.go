package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/alihamzaoriginal/namespace-terminator/internal/kube"
	"github.com/spf13/cobra"
)

type options struct {
	kubeconfig     string
	contextName    string
	allTerminating bool
	dryRun         bool
	yes            bool
	output         string
	timeout        time.Duration
}

func Execute() error {
	rootCmd := newRootCommand()
	err := rootCmd.Execute()
	if err == nil {
		return nil
	}

	format, formatErr := rootCmd.Flags().GetString("output")
	if formatErr != nil {
		format = "text"
	}

	if format == "json" {
		_ = printJSON(os.Stderr, map[string]any{
			"error": err.Error(),
		})
		return err
	}

	fmt.Fprintln(os.Stderr, err)
	return err
}

func newRootCommand() *cobra.Command {
	opts := options{}

	cmd := &cobra.Command{
		Use:   "nst <namespace> [<namespace> ...]",
		Short: "Force terminate Kubernetes namespaces",
		Long: "nst force-terminates Kubernetes namespaces by explicit name or by selecting all namespaces stuck in the Terminating phase.",
		Args: func(cmd *cobra.Command, args []string) error {
			if opts.allTerminating && len(args) > 0 {
				return errors.New("namespace names cannot be provided with --all-terminating")
			}
			if !opts.allTerminating && len(args) == 0 {
				return errors.New("provide one or more namespace names or set --all-terminating")
			}
			switch opts.output {
			case "text", "json":
				return nil
			default:
				return fmt.Errorf("unsupported output format %q", opts.output)
			}
		},
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			clientset, err := kube.NewClientset(kube.Config{
				Kubeconfig: opts.kubeconfig,
				Context:    opts.contextName,
			})
			if err != nil {
				return err
			}

			targets, err := kube.ResolveTargets(ctx, clientset, args, opts.allTerminating)
			if err != nil {
				return err
			}

			if len(targets) == 0 {
				return errors.New("no matching namespaces found")
			}

			if kube.HasBulkOperation(targets, opts.allTerminating) && !opts.yes && !opts.dryRun {
				confirmed, err := confirmBulkOperation(targets)
				if err != nil {
					return err
				}
				if !confirmed {
					return errors.New("operation cancelled")
				}
			}

			response, err := kube.Run(ctx, clientset, kube.RunRequest{
				Names:          args,
				AllTerminating: opts.allTerminating,
				DryRun:         opts.dryRun,
			}, opts.timeout)
			if err != nil {
				return err
			}

			if err := printResponse(os.Stdout, opts.output, response); err != nil {
				return err
			}

			if kube.HasFailures(response.Results) {
				return errors.New("one or more namespaces failed to terminate")
			}

			return nil
		},
	}

	cmd.SetContext(context.Background())
	flags := cmd.Flags()
	flags.BoolVar(&opts.allTerminating, "all-terminating", false, "target all namespaces stuck in Terminating")
	flags.StringVar(&opts.contextName, "context", "", "kubeconfig context to use")
	flags.StringVar(&opts.kubeconfig, "kubeconfig", "", "path to the kubeconfig file")
	flags.BoolVar(&opts.dryRun, "dry-run", false, "show namespaces that would be terminated without making changes")
	flags.BoolVarP(&opts.yes, "yes", "y", false, "skip confirmation for bulk operations")
	flags.StringVarP(&opts.output, "output", "o", "text", "output format: text or json")
	flags.DurationVar(&opts.timeout, "timeout", kube.DefaultTimeout, "how long to wait for namespace deletion after clearing finalizers")

	return cmd
}

func confirmBulkOperation(targets []string) (bool, error) {
	fmt.Fprintf(os.Stderr, "This will force-terminate %d namespace(s): %s\n", len(targets), strings.Join(targets, ", "))
	fmt.Fprint(os.Stderr, "Continue? [y/N]: ")

	reader := bufio.NewReader(os.Stdin)
	answer, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, fmt.Errorf("read confirmation: %w", err)
	}

	answer = strings.TrimSpace(strings.ToLower(answer))
	return answer == "y" || answer == "yes", nil
}

func printResponse(out *os.File, format string, response kube.RunResponse) error {
	if format == "json" {
		return printJSON(out, response)
	}

	mode := "explicit namespace targets"
	if response.Mode == kube.ModeAllTerminating {
		mode = "all terminating namespaces"
	}

	if response.DryRun {
		fmt.Fprintf(out, "Dry run for %s:\n", mode)
	} else {
		fmt.Fprintf(out, "Executing for %s:\n", mode)
	}

	for _, target := range response.Targets {
		fmt.Fprintf(out, "- %s\n", target)
	}

	for _, result := range response.Results {
		if result.Message == "" {
			fmt.Fprintf(out, "%s: %s\n", result.Namespace, result.Status)
			continue
		}
		fmt.Fprintf(out, "%s: %s (%s)\n", result.Namespace, result.Status, result.Message)
	}

	return nil
}

func printJSON(out *os.File, payload any) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(payload)
}
