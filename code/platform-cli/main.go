package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
)

// Platform CLI for interacting with the Internal Developer Platform

var (
	version   = "dev"
	commit    = "unknown"
	buildTime = "unknown"
)

type Config struct {
	Kubeconfig string
	Namespace  string
	Context    string
	Output     string // table, json, yaml
}

type PlatformClient struct {
	dynamicClient dynamic.Interface
	config        *Config
}

// Resource GVR definitions
var (
	serviceGVR       = schema.GroupVersionResource{Group: "platform.mycompany.io", Version: "v1", Resource: "services"}
	environmentGVR   = schema.GroupVersionResource{Group: "platform.mycompany.io", Version: "v1", Resource: "environments"}
	tenantGVR        = schema.GroupVersionResource{Group: "platform.mycompany.io", Version: "v1", Resource: "tenants"}
	deploymentGVR    = schema.GroupVersionResource{Group: "platform.mycompany.io", Version: "v1", Resource: "deployments"}
)

func main() {
	rootCmd := &cobra.Command{
		Use:     "platform-cli",
		Short:   "Platform Engineering CLI",
		Long:    `CLI for interacting with the Internal Developer Platform`,
		Version: fmt.Sprintf("%s (%s) built at %s", version, commit, buildTime),
	}

	// Global flags
	config := &Config{}
	rootCmd.PersistentFlags().StringVar(&config.Kubeconfig, "kubeconfig", "", "Path to kubeconfig")
	rootCmd.PersistentFlags().StringVar(&config.Namespace, "namespace", "", "Namespace (default: all)")
	rootCmd.PersistentFlags().StringVar(&config.Context, "context", "", "Kubernetes context")
	rootCmd.PersistentFlags().StringVarP(&config.Output, "output", "o", "table", "Output format (table, json, yaml)")

	// Add subcommands
	rootCmd.AddCommand(newServiceCmd(config))
	rootCmd.AddCommand(newEnvironmentCmd(config))
	rootCmd.AddCommand(newTenantCmd(config))
	rootCmd.AddCommand(newDeploymentCmd(config))
	rootCmd.AddCommand(newVersionCmd())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("platform-cli version %s\n", version)
			fmt.Printf("  commit: %s\n", commit)
			fmt.Printf("  built: %s\n", buildTime)
		},
	}
}

func newServiceCmd(config *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "service",
		Short: "Manage services",
		Long:  `Manage services in the platform catalog`,
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List services",
		RunE:  runListServices(config),
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "get [name]",
		Short: "Get service details",
		Args:  cobra.ExactArgs(1),
		RunE:  runGetService(config),
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "create [name]",
		Short: "Create a new service",
		Args:  cobra.ExactArgs(1),
		RunE:  runCreateService(config),
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "delete [name]",
		Short: "Delete a service",
		Args:  cobra.ExactArgs(1),
		RunE:  runDeleteService(config),
	})

	return cmd
}

func newEnvironmentCmd(config *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "environment",
		Short: "Manage environments",
		Aliases: []string{"env"},
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List environments",
		RunE:  runListEnvironments(config),
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "get [name]",
		Short: "Get environment details",
		Args:  cobra.ExactArgs(1),
		RunE:  runGetEnvironment(config),
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "create [name]",
		Short: "Create a new environment",
		Args:  cobra.ExactArgs(1),
		RunE:  runCreateEnvironment(config),
	})

	return cmd
}

func newTenantCmd(config *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tenant",
		Short: "Manage tenants",
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List tenants",
		RunE:  runListTenants(config),
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "get [name]",
		Short: "Get tenant details",
		Args:  cobra.ExactArgs(1),
		RunE:  runGetTenant(config),
	})

	return cmd
}

func newDeploymentCmd(config *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deployment",
		Short: "Manage deployments",
		Aliases: []string{"deploy"},
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List deployments",
		RunE:  runListDeployments(config),
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "get [name]",
		Short: "Get deployment details",
		Args:  cobra.ExactArgs(1),
		RunE:  runGetDeployment(config),
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "create [name]",
		Short: "Create a new deployment",
		Args:  cobra.ExactArgs(1),
		RunE:  runCreateDeployment(config),
	})

	return cmd
}

// Client initialization
func (c *Config) newClient() (*PlatformClient, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if c.Kubeconfig != "" {
		loadingRules.ExplicitPath = c.Kubeconfig
	}

	overrides := &clientcmd.ConfigOverrides{}
	if c.Context != "" {
		overrides.CurrentContext = c.Context
	}

	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides)
	restConfig, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create client config: %w", err)
	}

	dynamicClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}

	return &PlatformClient{
		dynamicClient: dynamicClient,
		config:        c,
	}, nil
}

func (c *Config) getNamespace() string {
	if c.Namespace != "" {
		return c.Namespace
	}
	return metav1.NamespaceAll
}

// Service commands
func runListServices(config *Config) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		client, err := config.newClient()
		if err != nil {
			return err
		}

		list, err := client.dynamicClient.Resource(serviceGVR).Namespace(config.getNamespace()).List(
			context.Background(), metav1.ListOptions{})
		if err != nil {
			return fmt.Errorf("failed to list services: %w", err)
		}

		return outputList(list, config.Output, []string{"NAME", "NAMESPACE", "OWNER", "REPOSITORY", "AGE"})
	}
}

func runGetService(config *Config) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		client, err := config.newClient()
		if err != nil {
			return err
		}

		obj, err := client.dynamicClient.Resource(serviceGVR).Namespace(config.getNamespace()).Get(
			context.Background(), args[0], metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get service: %w", err)
		}

		return outputObject(obj, config.Output)
	}
}

func runCreateService(config *Config) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		fmt.Printf("Creating service %s (not implemented - requires API server)\n", args[0])
		return nil
	}
}

func runDeleteService(config *Config) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		client, err := config.newClient()
		if err != nil {
			return err
		}

		err = client.dynamicClient.Resource(serviceGVR).Namespace(config.getNamespace()).Delete(
			context.Background(), args[0], metav1.DeleteOptions{})
		if err != nil {
			return fmt.Errorf("failed to delete service: %w", err)
		}

		fmt.Printf("Service %s deleted\n", args[0])
		return nil
	}
}

// Environment commands
func runListEnvironments(config *Config) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		client, err := config.newClient()
		if err != nil {
			return err
		}

		list, err := client.dynamicClient.Resource(environmentGVR).Namespace(config.getNamespace()).List(
			context.Background(), metav1.ListOptions{})
		if err != nil {
			return fmt.Errorf("failed to list environments: %w", err)
		}

		return outputList(list, config.Output, []string{"NAME", "NAMESPACE", "CLUSTER", "STATUS", "AGE"})
	}
}

func runGetEnvironment(config *Config) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		client, err := config.newClient()
		if err != nil {
			return err
		}

		obj, err := client.dynamicClient.Resource(environmentGVR).Namespace(config.getNamespace()).Get(
			context.Background(), args[0], metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get environment: %w", err)
		}

		return outputObject(obj, config.Output)
	}
}

func runCreateEnvironment(config *Config) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		fmt.Printf("Creating environment %s (not implemented - requires API server)\n", args[0])
		return nil
	}
}

// Tenant commands
func runListTenants(config *Config) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		client, err := config.newClient()
		if err != nil {
			return err
		}

		list, err := client.dynamicClient.Resource(tenantGVR).Namespace(config.getNamespace()).List(
			context.Background(), metav1.ListOptions{})
		if err != nil {
			return fmt.Errorf("failed to list tenants: %w", err)
		}

		return outputList(list, config.Output, []string{"NAME", "DISPLAY NAME", "OWNER", "EMAIL", "AGE"})
	}
}

func runGetTenant(config *Config) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		client, err := config.newClient()
		if err != nil {
			return err
		}

		obj, err := client.dynamicClient.Resource(tenantGVR).Namespace(config.getNamespace()).Get(
			context.Background(), args[0], metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get tenant: %w", err)
		}

		return outputObject(obj, config.Output)
	}
}

// Deployment commands
func runListDeployments(config *Config) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		client, err := config.newClient()
		if err != nil {
			return err
		}

		list, err := client.dynamicClient.Resource(deploymentGVR).Namespace(config.getNamespace()).List(
			context.Background(), metav1.ListOptions{})
		if err != nil {
			return fmt.Errorf("failed to list deployments: %w", err)
		}

		return outputList(list, config.Output, []string{"NAME", "NAMESPACE", "SERVICE", "ENVIRONMENT", "VERSION", "STATUS", "AGE"})
	}
}

func runGetDeployment(config *Config) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		client, err := config.newClient()
		if err != nil {
			return err
		}

		obj, err := client.dynamicClient.Resource(deploymentGVR).Namespace(config.getNamespace()).Get(
			context.Background(), args[0], metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get deployment: %w", err)
		}

		return outputObject(obj, config.Output)
	}
}

func runCreateDeployment(config *Config) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		fmt.Printf("Creating deployment %s (not implemented - requires API server)\n", args[0])
		return nil
	}
}

// Output helpers
func outputList(list interface{}, format string, headers []string) error {
	switch format {
	case "json":
		return outputJSON(list)
	case "yaml":
		return outputYAML(list)
	case "table":
		return outputTable(list, headers)
	default:
		return fmt.Errorf("unknown output format: %s", format)
	}
}

func outputObject(obj interface{}, format string) error {
	switch format {
	case "json":
		return outputJSON(obj)
	case "yaml":
		return outputYAML(obj)
	case "table":
		// For single object, print as JSON
		return outputJSON(obj)
	default:
		return fmt.Errorf("unknown output format: %s", format)
	}
}

func outputJSON(obj interface{}) error {
	data, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

func outputYAML(obj interface{}) error {
	// Would need gopkg.in/yaml.v3
	data, _ := json.MarshalIndent(obj, "", "  ")
	fmt.Println(string(data))
	return nil
}

func outputTable(list interface{}, headers []string) error {
	// This is a simplified table output
	// In reality, you'd extract items from the list
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	defer w.Flush()

	// Print headers
	for i, h := range headers {
		if i > 0 {
			fmt.Fprint(w, "\t")
		}
		fmt.Fprint(w, h)
	}
	fmt.Fprintln(w)

	// Note: Actual implementation would iterate over list items
	// This is a placeholder
	fmt.Fprintln(w, "(no resources found)")

	return nil
}

// Helper to get string from map
func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// Helper to get nested map
func getMap(m map[string]interface{}, key string) map[string]interface{} {
	if v, ok := m[key]; ok {
		if mm, ok := v.(map[string]interface{}); ok {
			return mm
		}
	}
	return nil
}

// Helper to format age
func formatAge(timestamp metav1.Time) string {
	if timestamp.IsZero() {
		return "unknown"
	}
	return time.Since(timestamp.Time).Round(time.Second).String()
}
