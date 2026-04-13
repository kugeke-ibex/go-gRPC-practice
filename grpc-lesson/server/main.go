package main

import (
	"bytes"
	"context"
	"fmt"
	"grpc-lesson/pb"
	"io"
	"log"
	"net"
	"os"
	"time"

	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_auth "github.com/grpc-ecosystem/go-grpc-middleware/auth"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
)

type server struct {
	pb.UnimplementedFileServiceServer
}

func (*server) ListFiles(ctx context.Context, req *pb.ListFilesRequest) (*pb.ListFilesResponse, error) {
	fmt.Println("========== [Unary] ListFiles invoked ==========")

	dir := "../storage"

	paths, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var filenames []string
	for _, path := range paths {
		if !path.IsDir() {
			filenames = append(filenames, path.Name())
		}
	}

	fmt.Printf("  → returning %d file(s): %v\n", len(filenames), filenames)

	res := &pb.ListFilesResponse{
		Filenames: filenames,
	}

	return res, nil
}

func (*server) Download(req *pb.DownloadRequest, stream pb.FileService_DownloadServer) error {
	fmt.Println("========== [Server Streaming] Download invoked ==========")

	filename := req.GetFilename()
	path := "../storage/" + filename
	fmt.Printf("Requested file: %s\n", filename)

	if _, err := os.Stat(path); os.IsNotExist(err) {
		fmt.Printf("  file not found: %s\n", filename)
		return status.Errorf(codes.NotFound, "file not found: %s", filename)
	}

	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	buf := make([]byte, 5)
	chunkIdx := 0
	totalBytes := 0
	for {
		n, err := file.Read(buf)
		if n == 0 || err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		res := &pb.DownloadResponse{
			Data: buf[:n],
		}
		if sendErr := stream.Send(res); sendErr != nil {
			return sendErr
		}
		chunkIdx++
		totalBytes += n
		fmt.Printf("  → sent chunk #%d (%d bytes): %q\n", chunkIdx, n, string(buf[:n]))
		time.Sleep(1 * time.Second)
	}
	fmt.Printf("Download finished: %d chunk(s) / %d bytes\n", chunkIdx, totalBytes)
	return nil
}

func (*server) Upload(stream pb.FileService_UploadServer) error {
	fmt.Println("========== [Client Streaming] Upload invoked ==========")

	var buf bytes.Buffer
	chunkIdx := 0

	for {
		req, err := stream.Recv()
		if err == io.EOF {
			fmt.Printf("Upload finished: received %d chunk(s) / %d bytes total\n", chunkIdx, buf.Len())
			fmt.Printf("  → returning size: %d\n", buf.Len())
			res := &pb.UploadResponse{
				Size: int32(buf.Len()),
			}
			return stream.SendAndClose(res)
		}
		if err != nil {
			return err
		}

		data := req.GetData()
		chunkIdx++
		fmt.Printf("  ← received chunk #%d (%d bytes): %q\n", chunkIdx, len(data), string(data))
		buf.Write(data)
	}
}

func (*server) UploadAndNotifyProgress(stream pb.FileService_UploadAndNotifyProgressServer) error {
	fmt.Println("========== [Bidirectional Streaming] UploadAndNotifyProgress invoked ==========")

	size := 0
	chunkIdx := 0
	for {
		req, err := stream.Recv()
		if err == io.EOF {
			fmt.Printf("UploadAndNotifyProgress finished: received %d chunk(s) / %d bytes total\n", chunkIdx, size)
			return nil
		}
		if err != nil {
			return err
		}

		data := req.GetData()
		chunkIdx++
		size += len(data)
		fmt.Printf("  ← received chunk #%d (%d bytes): %q\n", chunkIdx, len(data), string(data))

		msg := fmt.Sprintf("Received %d bytes", size)
		res := &pb.UploadAndNotifyProgressResponse{Msg: msg}
		if sendErr := stream.Send(res); sendErr != nil {
			return sendErr
		}
		fmt.Printf("  → [progress] %s\n", msg)
	}
}

func myLogging() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		fmt.Printf("[interceptor] %s | request : %+v\n", info.FullMethod, req)

		resp, err := handler(ctx, req)
		if err != nil {
			fmt.Printf("[interceptor] %s | error   : %v\n", info.FullMethod, err)
			return nil, err
		}
		fmt.Printf("[interceptor] %s | response: %+v\n", info.FullMethod, resp)
		return resp, nil
	}
}

func authorize(ctx context.Context) (context.Context, error) {
	token, err := grpc_auth.AuthFromMD(ctx, "Bearer")
	if err != nil {
		return nil, err
	}

	if token != "test-token" {
		return nil, status.Error(codes.Unauthenticated, "token is invalid")
	}

	return ctx, nil
}

func main() {
	lis, err := net.Listen("tcp", "localhost:50051")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	creds, err := credentials.NewServerTLSFromFile(
		"ssl/localhost.pem", 
		"ssl/localhost-key.pem",
	)
	if err != nil {
		log.Fatalf("Failed to load TLS keys: %v", err)
	}

	s := grpc.NewServer(
		grpc.Creds(creds),
		grpc.UnaryInterceptor(
		grpc_middleware.ChainUnaryServer(
			myLogging(),
			grpc_auth.UnaryServerInterceptor(authorize),
			),
		),
	)
	pb.RegisterFileServiceServer(s, &server{})

	fmt.Println("Server is running on port 50051")
	if err := s.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}