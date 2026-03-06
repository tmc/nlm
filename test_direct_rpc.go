package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/tmc/nlm/internal/batchexecute"
	"github.com/tmc/nlm/internal/rpc"
)

func main() {
	// Load environment
	authToken := os.Getenv("NLM_AUTH_TOKEN")
	cookies := os.Getenv("NLM_COOKIES")

	if authToken == "" || cookies == "" {
		fmt.Println("Please source ~/.nlm/env first")
		os.Exit(1)
	}

	// Create RPC client directly (like the original implementation)
	rpcClient := rpc.New(authToken, cookies, batchexecute.WithDebug(true))

	projectID := "e072d9da-dbec-401b-a0c6-9e2bea47a00f"
	instructions := "Create a test audio overview"

	fmt.Printf("Testing direct RPC call for audio creation...\n")
	fmt.Printf("Project ID: %s\n", projectID)
	fmt.Printf("Instructions: %s\n\n", instructions)

	// Make the direct RPC call (original approach)
	resp, err := rpcClient.Do(rpc.Call{
		ID: "AHyHrd", // RPCCreateAudioOverview
		Args: []interface{}{
			projectID,
			0, // audio_type
			[]string{instructions},
		},
		NotebookID: projectID,
	})

	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	// Parse response
	var data interface{}
	if err := json.Unmarshal(resp, &data); err != nil {
		fmt.Printf("Failed to parse response: %v\n", err)
		fmt.Printf("Raw response: %s\n", string(resp))
	} else {
		fmt.Printf("Success! Response: %+v\n", data)
	}
}
