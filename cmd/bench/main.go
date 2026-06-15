package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"time"

	skylexv1 "github.com/zhinea/skylex/gen/skylex/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	var (
		serverAddr  = flag.String("addr", "localhost:9090", "gRPC server address")
		concurrency = flag.Int("c", 50, "concurrent workers")
		clusters    = flag.Int("n", 100, "total clusters to create")
		authToken   = flag.String("token", "", "JWT auth token")
	)
	flag.Parse()

	if *authToken == "" {
		*authToken = os.Getenv("SKYLEX_AUTH_TOKEN")
	}

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

	conn, err := grpc.NewClient(*serverAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to connect: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if !conn.WaitForStateChange(ctx, connectivity.Ready) {
		if conn.GetState() != connectivity.Ready {
			fmt.Fprintln(os.Stderr, "connection not ready")
			os.Exit(1)
		}
	}

	clusterClient := skylexv1.NewClusterServiceClient(conn)

	total := *clusters
	workers := *concurrency
	if workers > total {
		workers = total
	}

	start := time.Now()

	var wg sync.WaitGroup
	var successCount, failCount atomic.Int64
	jobs := make(chan int, total)

	perRPCCreds := grpc.WithPerRPCCredentials(tokenAuth(*authToken))
	_ = perRPCCreds

	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				name := fmt.Sprintf("bench-cluster-%04d", i)
				req := &skylexv1.CreateClusterRequest{
					Name: name,
					Config: &skylexv1.ClusterConfig{
						Engine:          skylexv1.Engine_ENGINE_POSTGRESQL,
						Version:         "16",
						ReplicationMode: skylexv1.ReplicationMode_REPLICATION_MODE_ASYNC,
						ReplicaCount:    2,
						PitrEnabled:     false,
						Labels:          map[string]string{"bench": "true"},
					},
				}

				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				_, err := clusterClient.CreateCluster(ctx, req)
				cancel()

				if err != nil {
					failCount.Add(1)
					log.Error("create failed", "name", name, "error", err)
				} else {
					successCount.Add(1)
				}
			}
		}()
	}

	for i := range total {
		jobs <- i
	}
	close(jobs)

	wg.Wait()

	elapsed := time.Since(start)
	succeeded := successCount.Load()
	failed := failCount.Load()
	rate := float64(succeeded) / elapsed.Seconds()

	fmt.Printf("=== Benchmark Results ===\n")
	fmt.Printf("Total clusters:  %d\n", total)
	fmt.Printf("Concurrency:     %d\n", workers)
	fmt.Printf("Duration:        %v\n", elapsed.Round(time.Millisecond))
	fmt.Printf("Succeeded:       %d\n", succeeded)
	fmt.Printf("Failed:          %d\n", failed)
	fmt.Printf("Throughput:      %.2f clusters/sec\n", rate)
	if succeeded > 0 {
		fmt.Printf("Avg latency:     %v\n", (elapsed / time.Duration(succeeded)).Round(time.Microsecond))
	}
}

type tokenAuth string

func (t tokenAuth) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	return map[string]string{"authorization": "Bearer " + string(t)}, nil
}

func (t tokenAuth) RequireTransportSecurity() bool {
	return false
}