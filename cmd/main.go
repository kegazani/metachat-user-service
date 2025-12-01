package main

import (
	"context"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gocql/gocql"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"github.com/metachat/common/event-sourcing/aggregates"
	"github.com/metachat/common/event-sourcing/serializer"
	"github.com/metachat/common/event-sourcing/store"
	"github.com/metachat/config/logging"
	pb "github.com/metachat/proto/generated/user"
	grpcServer "metachat/user-service/internal/grpc"
	"metachat/user-service/internal/kafka"
	"metachat/user-service/internal/repository"
	"metachat/user-service/internal/service"
)

func main() {
	// Initialize logger
	loggerConfig := logging.LoggerConfig{
		ServiceName: "user-service",
		Environment: viper.GetString("environment"),
		LogLevel:    viper.GetString("log.level"),
		LogFormat:   viper.GetString("log.format"),
		LogOutput:   viper.GetString("log.output"),
	}
	logger := logging.NewLogger(loggerConfig)

	// Load configuration
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("./config")
	viper.AddConfigPath("/app/config")

	if err := viper.ReadInConfig(); err != nil {
		logger.Fatalf("Failed to read config file: %v", err)
	}

	// Initialize event store
	var eventStore store.EventStore
	eventStoreType := viper.GetString("event_store.type")

	switch eventStoreType {
	case "memory":
		eventStore = store.NewMemoryEventStore()
	case "eventstoredb":
		// TODO: Implement EventStoreDB client
		logger.Fatal("EventStoreDB client not implemented yet")
	default:
		eventStore = store.NewMemoryEventStore()
		logger.Warn("Using in-memory event store (not suitable for production)")
	}

	// Initialize serializer
	serializer := serializer.NewJSONSerializer()

	// Initialize Cassandra cluster
	cluster := gocql.NewCluster(viper.GetString("cassandra.hosts"))
	cluster.Keyspace = viper.GetString("cassandra.keyspace")
	cluster.Consistency = gocql.Quorum
	cluster.Timeout = 10 * time.Second

	// Create Cassandra session
	session, err := cluster.CreateSession()
	if err != nil {
		logger.Fatalf("Failed to connect to Cassandra: %v", err)
	}
	defer session.Close()

	// Initialize repositories
	userRepository := repository.NewUserRepository(eventStore, serializer)
	userReadRepository := repository.NewUserReadRepository(session)

	// Initialize Cassandra tables
	if err := userReadRepository.InitializeTables(); err != nil {
		logger.Fatalf("Failed to initialize Cassandra tables: %v", err)
	}

	// Initialize Kafka producer
	kafkaBootstrapServers := viper.GetString("kafka.bootstrap_servers")
	kafkaTopic := viper.GetString("kafka.user_events_topic")
	userEventProducer, err := kafka.NewUserEventProducer(kafkaBootstrapServers, kafkaTopic)
	if err != nil {
		logger.Fatalf("Failed to create user event producer: %v", err)
	}
	defer userEventProducer.Close()

	// Initialize services
	userService := service.NewUserService(userRepository, userReadRepository, userEventProducer)

	// Initialize aggregates (not used for now)
	_ = func(id string) aggregates.Aggregate {
		return aggregates.NewUserAggregate(id)
	}

	// Initialize gRPC server
	grpcSrv := grpcServer.NewUserServer(userService, logger)

	// Create gRPC server
	port := viper.GetString("server.port")
	if port == "" {
		port = "50051" // Default gRPC port
	}

	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		logger.Fatalf("Failed to listen on port %s: %v", port, err)
	}

	s := grpc.NewServer()
	pb.RegisterUserServiceServer(s, grpcSrv)

	// Enable reflection for development
	reflection.Register(s)

	// Start server in a goroutine
	go func() {
		logger.Infof("Starting gRPC server on port %s", port)
		if err := s.Serve(lis); err != nil {
			logger.Fatalf("Failed to start gRPC server: %v", err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down gRPC server...")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		s.GracefulStop()
		close(done)
	}()

	select {
	case <-done:
		logger.Info("gRPC server exited gracefully")
	case <-ctx.Done():
		logger.Info("gRPC server shutdown timeout")
	}

	logger.Info("Server exited")
}
