package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
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
	serverCtx     serverContext
	serverAddress string
)

const (
	contentTypeJson = "application/json"
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
	clearDb()

	serverCtx = serverContext{mongoClient: mongoClient, dbName: dbName, collectionIndex: make(map[string]bool)}
	mainServer := serverCtx.MainServer()

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

func clearDb() {
	ctx, cancel := context.WithTimeout(context.TODO(), 2*time.Second)
	defer cancel()

	collection := testDB.Collection(DocumentCollection)
	err := collection.Drop(ctx)
	if err != nil {
		log.Printf("Error trying to drop collection (%s). Error: %s\n", DocumentCollection, err.Error())
	} else {
		log.Printf("Collection (%s) was cleared\n", DocumentCollection)
		// Needs to reset this map cause we cleared the db so the indexes should be created again
		serverCtx.collectionIndex = map[string]bool{}
	}
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

func TestHttpServerPostObject_OK(t *testing.T) {
	url := fmt.Sprintf("%s/save", serverAddress)
	doc := MyDocument{Name: "test1", Key: "key1"}

	jsonData, err := json.Marshal(doc)
	if err != nil {
		log.Fatalf("Error marshaling JSON: %v", err)
	}

	resp, err := http.Post(url, contentTypeJson, bytes.NewBuffer(jsonData))
	if err != nil {
		t.Fatalf("POST request (url: %s | Object: %s) failed: %v", url, jsonData, err)
	}
	defer resp.Body.Close()

	var body MyDocument
	err = json.NewDecoder(resp.Body).Decode(&body)
	if err != nil {
		t.Fatalf("POST request (url: %s). Could not deserialized body.", url)
	}

	t.Logf("Status: %d", resp.StatusCode)
	t.Logf("Body: %s", body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected: status 200, got: %d", resp.StatusCode)
	}

	if body.Key != doc.Key || body.Name != doc.Name || body.State != STATE_INIT {
		t.Fatalf("expected: {key: %s | name: %s | state: %s}, got: %v", doc.Key, doc.Name, STATE_INIT, body)
	}
}

func TestHttpServerPostObject_DuplicateKey(t *testing.T) {
	clearDb()
	url := fmt.Sprintf("%s/save", serverAddress)
	doc := MyDocument{Name: "test1", Key: "key1"}

	jsonData, err := json.Marshal(doc)
	if err != nil {
		log.Fatalf("Error marshaling JSON: %v", err)
	}

	resp_ok, err := http.Post(url, contentTypeJson, bytes.NewBuffer(jsonData))
	if err != nil {
		t.Fatalf("POST request (url: %s | Object: %s) failed: %v", url, jsonData, err)
	}
	defer resp_ok.Body.Close()

	count, _ := testDB.Collection(DocumentCollection).CountDocuments(context.TODO(), nil)
	log.Printf("Nb documents: %d", count)

	if resp_ok.StatusCode != http.StatusOK {
		t.Fatalf("expected: status 200, got: %d", resp_ok.StatusCode)
	}

	resp, err := http.Post(url, contentTypeJson, bytes.NewBuffer(jsonData))
	if err != nil {
		t.Fatalf("POST request (url: %s | Object: %s) failed: %v", url, jsonData, err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	t.Logf("Status: %d", resp.StatusCode)
	t.Logf("Body: %s", body)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected: status 400, got: %d", resp.StatusCode)
	}

	expected := "keyIndex dup key: { key: \"" + doc.Key + "\" }"
	if !strings.Contains(string(body), expected) {
		t.Fatalf("expected: (%s) in (%s) but it was not in the body", expected, body)
	}
}
