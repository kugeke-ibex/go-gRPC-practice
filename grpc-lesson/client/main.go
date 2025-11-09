package main

import (
	"google.golang.org/grpc"
	"grpc-lesson/pb"
	"log"
	"context"
	"fmt"
)

func main() {
	conn, err := grpc.Dial("localhost:50051", grpc.WithInsecure())
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	client := pb.NewFileServiceClient(conn)
	callListFiles(client)
}

func callListFiles(client pb.FileServiceClient) {
	res, err := client.ListFiles(context.Background(), &pb.ListFilesRequest{})
	if err != nil {
		log.Fatalf("Failed to call ListFiles: %v", err)
	}
	fmt.Println(res.GetFilenames())
}