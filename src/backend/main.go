package main

import (
	"fmt"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"log"
	"net"

	pb "github.com/gong023/micro-sample/proto/gen"
	context "golang.org/x/net/context"
)

func main() {
	port := 8000
	logger, err := zap.NewDevelopment()
	if err != nil {
		log.Fatalf("failed to create logger :%v", err)
	}
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		logger.Fatal("failed to listen", zap.Error(err))
	}
	server := grpc.NewServer(grpc.UnaryInterceptor(
		grpc_zap.UnaryServerInterceptor(logger),
	))
	grpc_zap.ReplaceGrpcLogger(logger)
	pb.RegisterCalcServer(server, &CalcService{})
	server.Serve(lis)
}

type CalcService struct{}

func (s *CalcService) Increment(ctx context.Context, req *pb.NumRequest) (*pb.NumResponse, error) {
	req.Val++
	return &pb.NumResponse{Val: req.Val}, nil
}
