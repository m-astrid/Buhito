// Запуск gRPC-сервера.
package server

import (
	"fmt"
	"log/slog"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	trainerv1 "github.com/evgshumakova/mini-owl/gen/trainer/v1"
)

// Run поднимает gRPC-сервер на addr и блокируется до ошибки.
// grpc (коннекшены) закрывается при выходе из Run.
func Run(addr string, srv *Server, log *slog.Logger) error {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", addr, err)
	}

	g := grpc.NewServer()
	trainerv1.RegisterTrainerServiceServer(g, srv)
	reflection.Register(g) // для grpcurl и будущих клиентов

	log.Info("gRPC-сервер запущен", "addr", addr)
	if err := g.Serve(lis); err != nil {
		return fmt.Errorf("grpc serve: %w", err)
	}
	return nil
}
