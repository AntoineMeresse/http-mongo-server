package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type serverContext struct {
	portNumber  int
	mongoClient *mongo.Client
	dbName      string
}

func (s *serverContext) rootHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "This is the main page. Port used: %d", s.portNumber)
}

func (s *serverContext) healthHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := s.mongoClient.Ping(ctx, nil); err != nil {
		http.Error(w, "MongoDB Unhealthy: "+err.Error(), http.StatusServiceUnavailable)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("UP"))
}

func main() {
	mongoUri := "mongodb://localhost:27017"
	mongoDbName := "test"

	// Init mongo
	clientOptions := options.Client().ApplyURI(mongoUri)
	mongoCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	mongoClient, err := mongo.Connect(mongoCtx, clientOptions)
	if err != nil {
		log.Fatal(err)
	}

	// Init context
	ctx := serverContext{portNumber: 8080, mongoClient: mongoClient, dbName: mongoDbName}

	port := fmt.Sprintf(":%d", ctx.portNumber)

	http.HandleFunc("/", ctx.rootHandler)
	http.HandleFunc("/health", ctx.healthHandler)

	fmt.Println("Server is listening on port http://localhost" + port)
	if err := http.ListenAndServe(port, nil); err != nil {
		fmt.Println("Error while starting server: ", err)
	}
}
