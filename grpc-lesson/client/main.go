package main

import (
	"context"
	"fmt"
	"grpc-lesson/pb"
	"io"
	"log"
	"os"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func main() {
	// conn, err := grpc.Dial("localhost:50051", grpc.WithInsecure())
	certFile := "/Users/kugeke/Library/Application Support/mkcert/rootCA.pem"
	creds, err := credentials.NewClientTLSFromFile(certFile, "")
	if err != nil {
		log.Fatalf("Failed to load TLS credentials from %s: %v", certFile, err)
	}
	conn, err := grpc.NewClient("localhost:50051", grpc.WithTransportCredentials(creds))
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	client := pb.NewFileServiceClient(conn)
	// callListFiles(client)
	callDownload(client)
	// callUpload(client)
	// callUploadAndNotifyProgress(client)
}

func callListFiles(client pb.FileServiceClient) {
	md := metadata.New(map[string]string{"authorization": "Bearer bad-token"})
	ctx := metadata.NewOutgoingContext(context.Background(), md)

	res, err := client.ListFiles(ctx, &pb.ListFilesRequest{})
	if err != nil {
		log.Fatalf("Failed to call ListFiles: %v", err)
	}
	fmt.Println(res.GetFilenames())
}

func callDownload(client pb.FileServiceClient) {
	ctx, cancel := context.WithTimeout(context.Background(), 10 * time.Second)
	defer cancel()

	req := &pb.DownloadRequest{Filename: "name.txt"}
	stream, err := client.Download(ctx, req)
	if err != nil {
		log.Fatalf("Failed to call Download: %v", err)
	}
	for {
		res, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			resErr, ok := status.FromError(err)
			if ok {
				if resErr.Code() == codes.NotFound {
					log.Fatalf("Error code: %v, Error Message: %v", resErr.Code(), resErr.Message())
				} else if resErr.Code() == codes.DeadlineExceeded {
					log.Fatalln("deadline exceeded")
				} else {
					log.Fatalf("Failed to receive data: %v", err)
				}
			} else {
				log.Fatalf("Failed to receive data: %v", err)
			}
		}

		log.Printf("Response from Download(bytes): %v", res.GetData())
		log.Printf("Response from Download(string): %v", string(res.GetData()))
	}
}

func callUpload(client pb.FileServiceClient) {
	fileName := "sports.txt"
	path := "/Users/kugeke/Development/Go/go-gRPC-practice/grpc-lesson/storage/" + fileName

	file, err := os.Open(path)
	if err != nil {
		log.Fatalf("Failed to open file: %v", err)
	}
	defer file.Close()

	stream, err := client.Upload(context.Background())
	if err != nil {
		log.Fatalf("Failed to call Upload: %v", err)
	}
	for {
		buf := make([]byte, 5)
		n, err := file.Read(buf)
		if n == 0 || err == io.EOF {
			break
		}
		if err != nil {
			log.Fatalf("Failed to read file: %v", err)
		}

		req := &pb.UploadRequest{Data: buf[:n]}
		if sendErr := stream.Send(req); sendErr != nil {
			log.Fatalf("Failed to send data: %v", sendErr)
		}

		time.Sleep(1 * time.Second)
	}

	res, err := stream.CloseAndRecv()
	if err != nil {
		log.Fatalf("Failed to close stream: %v", err)
	}

	log.Printf("Received data size: %v", res.GetSize())
}

func callUploadAndNotifyProgress(client pb.FileServiceClient) {
	fileName := "sports.txt"
	path := "/Users/kugeke/Development/Go/go-gRPC-practice/grpc-lesson/storage/" + fileName

	file, err := os.Open(path)
	if err != nil {
		log.Fatalf("Failed to open file: %v", err)
	}
	defer file.Close()

	stream, err := client.UploadAndNotifyProgress(context.Background())
	if err != nil {
		log.Fatalf("Failed to call UploadAndNotifyProgress: %v", err)
	}

	// request
	buf := make([]byte, 5)
	go func() {
		for {
			n, err := file.Read(buf)
			if n == 0 || err == io.EOF {
				break
			}
			if err != nil {
				log.Fatalf("Failed to read file: %v", err)
			}

			req := &pb.UploadAndNotifyProgressRequest{Data: buf[:n]}
			if sendErr := stream.Send(req); sendErr != nil {
				log.Fatalf("Failed to send data: %v", sendErr)
			}
			time.Sleep(1 * time.Second)
		}

		err := stream.CloseSend()
		if err != nil {
			log.Fatalf("Failed to close stream: %v", err)
		}
	}()

	// backend
	ch := make(chan struct{})
	go func() {
		for {
			res, err := stream.Recv()
			if err == io.EOF {
				break
			}
			if err != nil {
				log.Fatalf("Failed to receive data: %v", err)
			}

			log.Printf("Response fraom UploadAndNotifyProgress: %v", res.GetMsg())
		}
		close(ch)
	}()
	log.Println("Waiting for the response...")
	<-ch
	log.Println("Done")
}
