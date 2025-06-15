package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	mongoClient   *mongo.Client
	testDB        *mongo.Database
	serverAddress string
)

func TestMain(m *testing.M) {
	ctx := context.Background()
	image := "mongo:latest"
	containerRequest := testcontainers.ContainerRequest{Image: image, ExposedPorts: []string{"27017/tcp"}, WaitingFor: wait.ForListeningPort("27017/tcp")}
	mongoContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{ContainerRequest: containerRequest, Started: true})
	if err != nil {
		log.Fatalf("Could not start test container (image: %s)", image)
	}

	// Mongo info
	host, _ := mongoContainer.Host(ctx)
	port, _ := mongoContainer.MappedPort(ctx, "27017")
	uri := fmt.Sprintf("mongodb://%s:%s", host, port.Port())

	dbName := "myServerTestDb"
	clientOpts := options.Client().ApplyURI(uri).SetConnectTimeout(10 * time.Second)
	client, err := mongo.Connect(ctx, clientOpts)
	if err != nil {
		log.Fatalf("Could not connect to MongoDB: %v", err)
	}
	mongoClient = client
	testDB = client.Database(dbName)

	serverContext := serverContext{mongoClient: mongoClient, dbName: dbName, collectionIndex: make(map[string]bool)}
	mainServer := serverContext.MainServer()

	// Start main serv on random port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatalf("Could not listen: %v", err)
	}

	server := &http.Server{Handler: mainServer}
	go server.Serve(listener)

	serverAddress = "http://" + listener.Addr().String()

	log.Printf("Mongo server uri: %s | Server uri: %s", uri, serverAddress)

	code := m.Run()

	log.Println("All tests were done")
	server.Close()
	mongoContainer.Terminate(ctx)
	os.Exit(code)
}

func TestHttpServerRoot(t *testing.T) {
	url := fmt.Sprintf("%s/", serverAddress)

	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET request (url: %s) failed: %v", url, err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	t.Logf("Status: %d", resp.StatusCode)
	t.Logf("Body: %s", body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected: status 200, got: %d", resp.StatusCode)
	}

	expected := "This is the main page."
	if string(body) != expected {
		t.Fatalf("expected: %s, got: %d", expected, body)
	}
}
