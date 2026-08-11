package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"k8s.io/client-go/kubernetes"

	"github.com/continuumx1/knw/internal/explain"
	knwkube "github.com/continuumx1/knw/internal/kubernetes"
	"github.com/continuumx1/knw/internal/render"
)

// apiTimeout bounds every cluster call so a hung API server cannot hang the CLI.
const apiTimeout = 30 * time.Second

// resourceFunc explains a single named resource.
type resourceFunc func(ctx context.Context, clientset kubernetes.Interface, namespace, name string) (string, error)

func main() {
	args := os.Args[1:]

	// Help must work without a cluster connection.
	if len(args) > 0 {
		switch args[0] {
		case "-h", "--help", "help":
			printUsage()
			return
		}
	}

	client, err := knwkube.NewClient()
	if err != nil {
		fail(err)
	}

	if len(args) == 0 {
		printClusterInfo(client)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), apiTimeout)
	defer cancel()

	switch args[0] {
	case "inspect":
		handleResource(ctx, client, "inspect", args, map[string]resourceFunc{
			"pod":     explain.InspectPod,
			"service": explain.InspectService,
			"ingress": explain.InspectIngress,
		}, "pod/payment-api")
	case "history":
		handleResource(ctx, client, "history", args, map[string]resourceFunc{
			"deployment": explain.DeploymentHistory,
		}, "deployment/payment-api")
	case "map":
		handleMap(ctx, client, args[1:])
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", args[0])
		printUsage()
		os.Exit(1)
	}
}

// handleResource parses a kind/name argument, dispatches to the matching
// explainer, and prints the result.
func handleResource(ctx context.Context, client *knwkube.Client, verb string, args []string, table map[string]resourceFunc, example string) {
	if len(args) != 2 {
		usageError(fmt.Sprintf("Usage: knw %s <kind>/<name>", verb))
	}

	kind, name, ok := parseResource(args[1])
	if !ok {
		usageError(fmt.Sprintf("Invalid resource format.\nExample: knw %s %s", verb, example))
	}

	fn, ok := table[strings.ToLower(kind)]
	if !ok {
		usageError(fmt.Sprintf("KNW %s supports: %s", verb, strings.Join(sortedKeys(table), ", ")))
	}

	result, err := fn(ctx, client.Clientset, client.Namespace, name)
	if err != nil {
		fail(err)
	}
	emit(result)
}

// handleMap parses the optional namespace and --all flag, then prints the map.
func handleMap(ctx context.Context, client *knwkube.Client, args []string) {
	showAll := false
	namespace := client.Namespace

	for _, arg := range args {
		switch {
		case arg == "--all":
			showAll = true
		case strings.HasPrefix(arg, "-"):
			usageError("Usage: knw map [--all] [namespace]")
		case strings.Contains(arg, "/"):
			usageError(fmt.Sprintf("map takes a namespace, not a resource.\nDid you mean: knw inspect %s", arg))
		default:
			namespace = arg
		}
	}

	result, err := explain.MapNamespace(ctx, client.Clientset, namespace, showAll)
	if err != nil {
		fail(err)
	}
	emit(result)
}

// emit prints the KNW banner followed by a command's result.
func emit(result string) {
	fmt.Println("KNW — Know what's happening.")
	fmt.Println()
	fmt.Print(result)
}

// fail reports an error to stderr and exits non-zero.
func fail(err error) {
	fmt.Fprintf(os.Stderr, "KNW error: %v\n", err)
	os.Exit(1)
}

// usageError reports a usage problem to stderr and exits non-zero.
func usageError(msg string) {
	fmt.Fprintln(os.Stderr, msg)
	os.Exit(1)
}

func sortedKeys(table map[string]resourceFunc) []string {
	keys := make([]string, 0, len(table))
	for k := range table {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// printUsage prints the KNW command overview.
func printUsage() {
	fmt.Println("KNW — Know what's happening.")
	fmt.Println("Every K8s Resource Has a Story.")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  knw                          Show cluster connection info")
	fmt.Println("  knw inspect <kind>/<name>    Explain a resource and its relationships")
	fmt.Println("  knw history <kind>/<name>    Show a resource's revision history")
	fmt.Println("  knw map [--all] [namespace]  Map a namespace's resource graph")
	fmt.Println("  knw help                     Show this help")
	fmt.Println()
	fmt.Println("Supported kinds for 'inspect': pod, service, ingress")
	fmt.Println("Supported kinds for 'history': deployment")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  knw inspect pod/payment-api")
	fmt.Println("  knw history deployment/payment-api")
	fmt.Println("  knw map kube-system")
	fmt.Println()
	fmt.Println("KNW is read-only and never modifies your cluster.")
}

func parseResource(value string) (string, string, bool) {
	parts := strings.SplitN(value, "/", 2)

	if len(parts) != 2 {
		return "", "", false
	}

	if parts[0] == "" || parts[1] == "" {
		return "", "", false
	}

	return parts[0], parts[1], true
}

func printClusterInfo(client *knwkube.Client) {
	version, err := client.Clientset.Discovery().ServerVersion()
	if err != nil {
		fmt.Fprintf(
			os.Stderr,
			"KNW error: cannot connect to Kubernetes: %v\n",
			err,
		)
		os.Exit(1)
	}

	fmt.Println(render.Mascot(colorMode()))
	fmt.Println("KNW — Know what's happening.")
	fmt.Println()
	fmt.Println("Cluster")
	fmt.Printf("  Context:     %s\n", client.Context)
	fmt.Printf("  Server:      %s\n", client.Server)
	fmt.Printf("  Kubernetes:  %s\n", version.GitVersion)
	fmt.Printf("  Namespace:   %s\n", client.Namespace)
	fmt.Println()
	fmt.Println("✓ Connected")
}

// stdoutIsTerminal reports whether standard output is a terminal, so KNW emits
// ANSI colour only when it will be displayed and never into a pipe or file.
func stdoutIsTerminal() bool {
	info, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// colorMode decides how much colour to use: none when output is not a terminal
// or NO_COLOR is set, truecolor when the terminal advertises it, otherwise the
// widely supported 256-colour palette.
func colorMode() render.ColorMode {
	if os.Getenv("NO_COLOR") != "" || !stdoutIsTerminal() {
		return render.ColorNone
	}
	switch os.Getenv("COLORTERM") {
	case "truecolor", "24bit":
		return render.ColorTrue
	default:
		return render.Color256
	}
}
