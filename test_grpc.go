package main

import (
	"context"
	"fmt"
	"log"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection/grpc_reflection_v1alpha"
)

func main() {
	conn, err := grpc.Dial("auth-service:50052", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()

	c := grpc_reflection_v1alpha.NewServerReflectionClient(conn)
	stream, err := c.ServerReflectionInfo(context.Background())
	if err != nil {
		log.Fatalf("failed to create stream: %v", err)
	}

	req := &grpc_reflection_v1alpha.ServerReflectionRequest{
		MessageRequest: &grpc_reflection_v1alpha.ServerReflectionRequest_ListServices{
			ListServices: "services",
		},
	}

	if err := stream.Send(req); err != nil {
		log.Fatalf("failed to send req: %v", err)
	}

	resp, err := stream.Recv()
	if err != nil {
		log.Fatalf("failed to recv resp: %v", err)
	}

	fmt.Println("Services:")
	for _, svc := range resp.GetListServicesResponse().GetService() {
		fmt.Println(svc.Name)
	}
}
