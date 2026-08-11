package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/continuumx1/knw/internal/explain"
	knwkube "github.com/continuumx1/knw/internal/kubernetes"
	"github.com/continuumx1/knw/internal/render"
)

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
		fmt.Fprintf(os.Stderr, "KNW error: %v\n", err)
		os.Exit(1)
	}

	if len(args) == 0 {
		printClusterInfo(client)
		return
	}

	if args[0] == "inspect" {
		if len(args) != 2 {
			fmt.Println("Usage: knw inspect <kind>/<name>")
			os.Exit(1)
		}

		kind, name, ok := parseResource(args[1])
		if !ok {
			fmt.Println("Invalid resource format.")
			fmt.Println("Example: knw inspect pod/payment-api")
			os.Exit(1)
		}

		var result string

		switch strings.ToLower(kind) {
		case "pod":
			result, err = explain.PodWhy(
				context.Background(),
				client.Clientset,
				client.Namespace,
				name,
			)
		case "service":
			result, err = explain.ServiceWhy(
				context.Background(),
				client.Clientset,
				client.Namespace,
				name,
			)
		case "ingress":
			result, err = explain.IngressWhy(
				context.Background(),
				client.Clientset,
				client.Namespace,
				name,
			)
		default:
			fmt.Printf("KNW v0.1 currently supports: pod, service, ingress\n")
			os.Exit(1)
		}

		if err != nil {
			fmt.Fprintf(os.Stderr, "KNW error: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("KNW — Know what's happening.")
		fmt.Println()
		fmt.Print(result)

		return
	}

	if args[0] == "history" {
		if len(args) != 2 {
			fmt.Println("Usage: knw history <kind>/<name>")
			os.Exit(1)
		}

		kind, name, ok := parseResource(args[1])
		if !ok {
			fmt.Println("Invalid resource format.")
			fmt.Println("Example: knw history deployment/payment-api")
			os.Exit(1)
		}

		if strings.ToLower(kind) != "deployment" {
			fmt.Printf("KNW history currently supports: deployment\n")
			os.Exit(1)
		}

		result, err := explain.DeploymentHistory(
			context.Background(),
			client.Clientset,
			client.Namespace,
			name,
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "KNW error: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("KNW — Know what's happening.")
		fmt.Println()
		fmt.Print(result)

		return
	}

	if args[0] == "map" {
		if len(args) > 2 {
			fmt.Println("Usage: knw map [namespace]")
			os.Exit(1)
		}

		namespace := client.Namespace
		if len(args) == 2 {
			namespace = args[1]
		}

		result, err := explain.MapNamespace(
			context.Background(),
			client.Clientset,
			namespace,
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "KNW error: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("KNW — Know what's happening.")
		fmt.Println()
		fmt.Print(result)

		return
	}

	fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", args[0])
	printUsage()
	os.Exit(1)
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
	fmt.Println("  knw map [namespace]          Map a namespace's resource graph")
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
