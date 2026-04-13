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
	certFile := "ssl/rootCA.pem"
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
	callListFiles(client)
	// callDownload(client)
	// callUpload(client)
	// callUploadAndNotifyProgress(client)
}

func callListFiles(client pb.FileServiceClient) {
	fmt.Println("========== [Unary] ListFiles ==========")

	md := metadata.New(map[string]string{"authorization": "Bearer test-token"})
	ctx := metadata.NewOutgoingContext(context.Background(), md)

	res, err := client.ListFiles(ctx, &pb.ListFilesRequest{})
	if err != nil {
		log.Fatalf("Failed to call ListFiles: %v", err)
	}

	filenames := res.GetFilenames()
	fmt.Printf("Found %d file(s):\n", len(filenames))
	for i, name := range filenames {
		fmt.Printf("  [%d] %s\n", i+1, name)
	}
}

func callDownload(client pb.FileServiceClient) {
	fmt.Println("========== [Server Streaming] Download ==========")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	filename := "name.txt"
	fmt.Printf("Requesting file: %s\n", filename)

	req := &pb.DownloadRequest{Filename: filename}
	stream, err := client.Download(ctx, req)
	if err != nil {
		log.Fatalf("Failed to call Download: %v", err)
	}

	chunkIdx := 0
	totalBytes := 0
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

		chunkIdx++
		data := res.GetData()
		totalBytes += len(data)
		fmt.Printf("  chunk #%d (%d bytes)\n", chunkIdx, len(data))
		fmt.Printf("    bytes : %v\n", data)
		fmt.Printf("    string: %q\n", string(data))
	}
	fmt.Printf("Download finished: %d chunk(s), %d bytes total\n", chunkIdx, totalBytes)
}

func callUpload(client pb.FileServiceClient) {
	fmt.Println("========== [Client Streaming] Upload ==========")

	fileName := "sports.txt"
	path := "../storage/" + fileName
	fmt.Printf("Uploading file: %s\n", fileName)

	file, err := os.Open(path)
	if err != nil {
		log.Fatalf("Failed to open file: %v", err)
	}
	defer file.Close()

	stream, err := client.Upload(context.Background())
	if err != nil {
		log.Fatalf("Failed to call Upload: %v", err)
	}

	chunkIdx := 0
	sentBytes := 0
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
		chunkIdx++
		sentBytes += n
		fmt.Printf("  → sent chunk #%d (%d bytes): %q\n", chunkIdx, n, string(buf[:n]))

		time.Sleep(1 * time.Second)
	}

	res, err := stream.CloseAndRecv()
	if err != nil {
		log.Fatalf("Failed to close stream: %v", err)
	}

	fmt.Printf("Upload finished: sent %d chunk(s) / %d bytes\n", chunkIdx, sentBytes)
	fmt.Printf("Server reported size: %d bytes\n", res.GetSize())
}

func callUploadAndNotifyProgress(client pb.FileServiceClient) {
	fmt.Println("========== [Bidirectional Streaming] UploadAndNotifyProgress ==========")

	fileName := "sports.txt"
	path := "../storage/" + fileName
	fmt.Printf("Uploading file: %s\n", fileName)

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
		chunkIdx := 0
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
			chunkIdx++
			fmt.Printf("  → [send]     chunk #%d (%d bytes): %q\n", chunkIdx, n, string(buf[:n]))
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

			fmt.Printf("  ← [progress] %s\n", res.GetMsg())
		}
		close(ch)
	}()

	fmt.Println("Waiting for progress notifications...")
	<-ch
	fmt.Println("UploadAndNotifyProgress finished")
}
