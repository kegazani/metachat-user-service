package main

import (
	"context"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gocql/gocql"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"metachat/user-service/internal/auth"
	grpcServer "metachat/user-service/internal/grpc"
	"metachat/user-service/internal/kafka"
	"metachat/user-service/internal/repository"
	"metachat/user-service/internal/service"

	"github.com/kegazani/metachat-event-sourcing/aggregates"
	"github.com/kegazani/metachat-event-sourcing/serializer"
	"github.com/kegazani/metachat-event-sourcing/store"
	pb "github.com/kegazani/metachat-proto/user"
	"github.com/sirupsen/logrus"
)

func main() {
	viper.AutomaticEnv()
	viper.SetEnvPrefix("")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("./config")
	viper.AddConfigPath("/app/config")

	if err := viper.ReadInConfig(); err != nil {
		logrus.Fatalf("Failed to read config file: %v", err)
	}

	// Initialize logger from config
	logger := logrus.New()
	logLevel := viper.GetString("logging.level")
	logFormat := viper.GetString("logging.format")

	switch logLevel {
	case "debug":
		logger.SetLevel(logrus.DebugLevel)
	case "info":
		logger.SetLevel(logrus.InfoLevel)
	case "warn":
		logger.SetLevel(logrus.WarnLevel)
	case "error":
		logger.SetLevel(logrus.ErrorLevel)
	default:
		logger.SetLevel(logrus.InfoLevel)
	}

	if logFormat == "json" {
		logger.SetFormatter(&logrus.JSONFormatter{})
	} else {
		logger.SetFormatter(&logrus.TextFormatter{})
	}

	// Initialize event store from config
	var eventStore store.EventStore
	eventStoreType := viper.GetString("event_store.type")

	switch eventStoreType {
	case "memory":
		eventStore = store.NewMemoryEventStore()
		logger.Info("Using in-memory event store")
	case "eventstoredb":
		eventStoreURL := viper.GetString("event_store.url")
		if eventStoreURL == "" {
			eventStoreURL = "http://localhost:2113"
		}
		eventStoreUsername := viper.GetString("event_store.username")
		if eventStoreUsername == "" {
			eventStoreUsername = "admin"
		}
		eventStorePassword := viper.GetString("event_store.password")
		if eventStorePassword == "" {
			eventStorePassword = "changeit"
		}
		streamPrefix := viper.GetString("event_store.stream_prefix")
		if streamPrefix == "" {
			streamPrefix = "metachat-user"
		}

		esdbStore, err := store.NewEventStoreDBEventStoreFromConfig(eventStoreURL, eventStoreUsername, eventStorePassword, streamPrefix)
		if err != nil {
			logger.WithError(err).Fatal("Failed to create EventStoreDB client")
		}
		eventStore = esdbStore
		logger.Info("Using EventStoreDB event store")
	default:
		eventStore = store.NewMemoryEventStore()
		logger.Warn("Using in-memory event store (not suitable for production)")
	}

	// Initialize serializer
	serializer := serializer.NewJSONSerializer()

	// Initialize Cassandra from config
	hosts := viper.GetStringSlice("cassandra.hosts")
	if len(hosts) == 0 {
		if h := viper.GetString("cassandra.hosts"); h != "" {
			hosts = []string{h}
		}
	}

	cassandraTimeout := viper.GetDuration("cassandra.timeout")
	if cassandraTimeout == 0 {
		cassandraTimeout = 10 * time.Second
	}

	reconnectAttempts := viper.GetInt("cassandra.reconnect_attempts")
	if reconnectAttempts == 0 {
		reconnectAttempts = 10
	}

	reconnectDelay := viper.GetDuration("cassandra.reconnect_delay")
	if reconnectDelay == 0 {
		reconnectDelay = 5 * time.Second
	}

	var session *gocql.Session
	var err error
	for i := 0; i < reconnectAttempts; i++ {
		cluster := gocql.NewCluster(hosts...)
		cluster.Keyspace = viper.GetString("cassandra.keyspace")

		consistency := viper.GetString("cassandra.consistency")
		switch consistency {
		case "ONE":
			cluster.Consistency = gocql.One
		case "QUORUM":
			cluster.Consistency = gocql.Quorum
		case "ALL":
			cluster.Consistency = gocql.All
		default:
			cluster.Consistency = gocql.Quorum
		}

		cluster.Timeout = cassandraTimeout

		// Add authentication if provided
		if username := viper.GetString("cassandra.username"); username != "" {
			cluster.Authenticator = gocql.PasswordAuthenticator{
				Username: username,
				Password: viper.GetString("cassandra.password"),
			}
		}

		session, err = cluster.CreateSession()
		if err == nil {
			break
		}
		logger.WithError(err).Warn("Failed to connect to Cassandra, retrying")
		time.Sleep(reconnectDelay)
	}
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

	// Initialize Kafka producer from config
	kafkaBrokers := viper.GetStringSlice("kafka.brokers")
	kafkaBootstrapServers := ""
	for i, broker := range kafkaBrokers {
		if i > 0 {
			kafkaBootstrapServers += ","
		}
		kafkaBootstrapServers += broker
	}

	kafkaTopic := viper.GetString("kafka.topics.user_events")
	if kafkaTopic == "" {
		kafkaTopic = viper.GetString("kafka.user_events_topic")
	}
	if kafkaTopic == "" {
		kafkaTopic = viper.GetString("kafka.topic_prefix") + "-user-events"
	}
	if kafkaTopic == "" {
		kafkaTopic = "metachat-user-events"
	}

	userEventProducer, err := kafka.NewUserEventProducer(kafkaBootstrapServers, kafkaTopic)
	if err != nil {
		logger.Fatalf("Failed to create user event producer: %v", err)
	}
	defer userEventProducer.Close()

	// Initialize JWT from config
	jwtSecret := viper.GetString("auth.jwt_secret")
	if jwtSecret == "" {
		jwtSecret = "dev-secret"
		logger.Warn("Using default JWT secret (not suitable for production)")
	}

	jwtExpiration := viper.GetDuration("auth.jwt_expiration")
	if jwtExpiration == 0 {
		jwtExpiration = 24 * time.Hour
	}

	jwtManager := auth.NewJWTManager(jwtSecret, jwtExpiration)

	googleClientID := viper.GetString("auth.google_client_id")
	googleOAuth := auth.NewGoogleOAuthProvider(googleClientID)

	appleClientID := viper.GetString("auth.apple_client_id")
	appleOAuth := auth.NewAppleOAuthProvider(appleClientID)

	userService := service.NewUserService(userRepository, userReadRepository, userEventProducer, jwtManager, googleOAuth, appleOAuth, nil, nil)

	// Initialize aggregates (not used for now)
	_ = func(id string) aggregates.Aggregate {
		return aggregates.NewUserAggregate(id)
	}

	// Initialize gRPC server
	grpcSrv := grpcServer.NewUserServer(userService, logger)

	// Create gRPC server from config
	port := viper.GetString("server.port")
	if port == "" {
		port = "50051" // Default gRPC port
	}

	host := viper.GetString("server.host")
	if host == "" {
		host = "0.0.0.0"
	}

	address := net.JoinHostPort(host, port)
	lis, err := net.Listen("tcp", address)
	if err != nil {
		logger.Fatalf("Failed to listen on %s: %v", address, err)
	}

	s := grpc.NewServer(
		grpc.UnaryInterceptor(grpcServer.CorrelationIDInterceptor(logger)),
	)
	pb.RegisterUserServiceServer(s, grpcSrv)

	// Enable reflection from config
	if viper.GetBool("grpc.reflection_enabled") {
		reflection.Register(s)
		logger.Info("gRPC reflection enabled")
	}

	// Start server in a goroutine
	go func() {
		logger.Infof("Starting gRPC server on %s", address)
		if err := s.Serve(lis); err != nil {
			logger.Fatalf("Failed to start gRPC server: %v", err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down gRPC server...")

	// Graceful shutdown from config
	shutdownTimeout := viper.GetDuration("grpc.shutdown_timeout")
	if shutdownTimeout == 0 {
		shutdownTimeout = 10 * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
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
