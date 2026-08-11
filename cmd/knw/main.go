package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/continuumx1/knw/internal/explain"
	knwkube "github.com/continuumx1/knw/internal/kubernetes"
)

func main() {
	client, err := knwkube.NewClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "KNW error: %v\n", err)
		os.Exit(1)
	}

	args := os.Args[1:]

	if len(args) == 0 {
		printClusterInfo(client)
		return
	}

	if args[0] == "why" {
		if len(args) != 2 {
			fmt.Println("Usage: knw why <kind>/<name>")
			os.Exit(1)
		}

		kind, name, ok := parseResource(args[1])
		if !ok {
			fmt.Println("Invalid resource format.")
			fmt.Println("Example: knw why pod/payment-api")
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

	fmt.Printf("Unknown command: %s\n", args[0])
	fmt.Println()
	fmt.Println("Available commands:")
	fmt.Println("  knw why <kind>/<name>")
	fmt.Println("  knw map [namespace]")
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
