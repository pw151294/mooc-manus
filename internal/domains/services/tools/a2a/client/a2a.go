package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"net/http"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2aclient"
	"github.com/a2aproject/a2a-go/a2aclient/agentcard"
)

var cardURL = flag.String("card-url", "http://127.0.0.1:9001", "Base URL of AgentCard server.")

func main() {
	flag.Parse()
	ctx := context.Background()

	// Resolve an AgentCard
	card, err := agentcard.DefaultResolver.Resolve(ctx, *cardURL)
	if err != nil {
		log.Fatalf("Failed to resolve an AgentCard: %v", err)
	}

	// Insecure connection is used for example purposes
	//withInsecureGRPC := a2aclient.WithGRPCTransport(grpc.WithTransportCredentials(insecure.NewCredentials()))
	withInsecureJSONRPC := a2aclient.WithJSONRPCTransport(&http.Client{})

	// Create a client connected to one of the interfaces specified in the AgentCard.
	client, err := a2aclient.NewFromCard(ctx, card, withInsecureJSONRPC)
	if err != nil {
		log.Fatalf("Failed to create a client: %v", err)
	}
	client.Destroy()

	// Send a message and log the response.
	msg := a2a.NewMessage(a2a.MessageRoleUser, a2a.TextPart{Text: "Hello, world"})
	resp, err := client.SendMessage(ctx, &a2a.MessageSendParams{Message: msg})
	if err != nil {
		log.Fatalf("Failed to send a message: %v", err)
	}

	bytes, _ := json.Marshal(resp)
	log.Printf("Response: %s", string(bytes))
}
