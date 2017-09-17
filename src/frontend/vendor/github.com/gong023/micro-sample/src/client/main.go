// this file is sample for client
package main

import (
	"context"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"log"

	pb "github.com/gong023/micro-sample/proto/gen"
)

func main() {
	logger, err := zap.NewDevelopment()
	if err != nil {
		log.Fatalf("failed to create logger :%v", err)
	}
	conn, err := grpc.Dial("localhost:8000", grpc.WithInsecure(), grpc.WithUnaryInterceptor(
		grpc_zap.UnaryClientInterceptor(logger),
	))
	grpc_zap.ReplaceGrpcLogger(logger)
	if err != nil {
		log.Fatalf("failed to connect :%v", err)
	}
	defer conn.Close()

	client := pb.NewCalcClient(conn)
	ctx := context.Background()
	res, err := client.Increment(ctx, &pb.NumRequest{Val: 0})
	if err != nil {
		logger.Fatal("got error from server", zap.Error(err))
	}
	logger.Info("got response", zap.Int64("value", res.Val))
}
