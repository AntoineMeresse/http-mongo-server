package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	STATE_INIT      = "INIT"
	STATE_VERIFIED  = "VERIFIED"
	STATE_REJECTED  = "REJECTED"
	STATE_PROCESSED = "PROCESSED"
)

type serverContext struct {
	port            string
	mongoClient     *mongo.Client
	dbName          string
	collectionIndex map[string]bool
}

type MyDocument struct {
	ID    *primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
	Name  string              `json:"name"`
	Key   string              `json:"key"`
	State string              `json:"state,omitempty"`
}

type MyDocumentList struct {
	ID        *primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
	ToProcess []MyDocument        `json:"documentList"`
}

type MyDocumentId struct {
	ID *primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
}

func (s *serverContext) rootHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "This is the main page. Port used: %s", s.port)
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

func (s *serverContext) ensureIndex(collection *mongo.Collection, ctx context.Context) {
	fmt.Printf("Ensure index start\n")
	name := collection.Name()
	if _, ok := s.collectionIndex[name]; ok {
		// fmt.Printf("Ensure index already ok\n")
		return
	}

	indexes := []mongo.IndexModel{
		{Keys: bson.D{{Key: "key", Value: 1}}, Options: options.Index().SetUnique(true).SetName("keyIndex")},
	}

	_, err := collection.Indexes().CreateMany(ctx, indexes)
	if err != nil {
		fmt.Printf("Could not ensure index already exist. Error: %s", err.Error())
		return
	}

	// fmt.Printf("Ensure index was ok. Adding %s to map\n", name)
	s.collectionIndex[name] = true
}

func (s *serverContext) saveHandler(w http.ResponseWriter, r *http.Request) {
	collection := s.mongoClient.Database(s.dbName).Collection("documentCollection")

	var doc MyDocument
	if err := json.NewDecoder(r.Body).Decode(&doc); err != nil {
		fmt.Println("Could not deserialized body :/")
		http.Error(w, "Error: "+err.Error(), http.StatusBadRequest)
		return
	}
	if doc.ID == nil {
		id := primitive.NewObjectID()
		doc.ID = &id
	}
	doc.State = STATE_INIT

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	s.ensureIndex(collection, ctx)

	res, err := collection.InsertOne(ctx, doc)
	if err != nil {
		fmt.Println("Could not insert document :/")
		http.Error(w, "Error: "+err.Error(), http.StatusBadRequest)
		return
	}

	if res != nil {
		println("Document was inserted")
	}

	json.NewEncoder(w).Encode(doc)
}

func (s *serverContext) updateToState(w http.ResponseWriter, r *http.Request, updateState string) {
	key := r.PathValue("key")
	collection := s.mongoClient.Database(s.dbName).Collection("documentCollection")

	filter := bson.M{"key": key, "state": STATE_INIT}
	update := bson.M{
		"$set": bson.M{
			"state": updateState,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	res, err := collection.UpdateOne(ctx, filter, update)
	if err != nil {
		http.Error(w, "Error: "+err.Error(), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write(fmt.Appendf(nil, "Match: %d| Updated: %d | Update to state: %s", res.MatchedCount, res.ModifiedCount, updateState))
}

func (s *serverContext) updateToVerified(w http.ResponseWriter, r *http.Request) {
	s.updateToState(w, r, STATE_VERIFIED)
}

func (s *serverContext) updateToRejected(w http.ResponseWriter, r *http.Request) {
	s.updateToState(w, r, STATE_REJECTED)
}

func (s *serverContext) saveBatchHandler(w http.ResponseWriter, r *http.Request) {
	collection := s.mongoClient.Database(s.dbName).Collection("documentCollectionBatch")

	var doc MyDocumentList
	if err := json.NewDecoder(r.Body).Decode(&doc); err != nil {
		fmt.Println("Could not deserialized body to MyDocumentList:/")
		http.Error(w, "Error: "+err.Error(), http.StatusBadRequest)
		return
	}
	if doc.ID == nil {
		id := primitive.NewObjectID()
		doc.ID = &id
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	res, err := collection.InsertOne(ctx, doc)
	if err != nil {
		fmt.Println("Could not insert document :/")
		http.Error(w, "Error: "+err.Error(), http.StatusBadRequest)
		return
	}

	if res != nil {
		fmt.Printf("Document batch was inserted. Res: %v\n", res)
	}

	json.NewEncoder(w).Encode(MyDocumentId{ID: doc.ID})
}

func (s *serverContext) processBatchHandler(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("documentId")
	collection := s.mongoClient.Database(s.dbName).Collection("documentCollectionBatch")

	documentId, err := primitive.ObjectIDFromHex(key)
	if err != nil {
		http.Error(w, "Error: "+err.Error(), http.StatusBadRequest)
		return
	}

	ctxRead, cancelRead := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancelRead()

	var batchDocument MyDocumentList
	err = collection.FindOne(ctxRead, bson.M{"_id": documentId}).Decode(&batchDocument)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			http.Error(w, "Error: "+err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, "Error: "+err.Error(), http.StatusBadRequest)
		return
	}

	ctxProcess, cancelProcess := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancelProcess()

	size := len(batchDocument.ToProcess)
	// fmt.Printf("Size: %d\n", size)
	updates := make([]mongo.WriteModel, 0, size)
	// fmt.Printf("UpdateSize: %d\n", len(updates))

	// fmt.Println("-------To process --------")
	for _, doc := range batchDocument.ToProcess {
		// fmt.Printf("Update n°%d -> key: %s\n", i, doc.Key)
		updates = append(updates,
			mongo.NewUpdateOneModel().
				SetFilter(bson.M{"key": doc.Key, "state": bson.M{"$ne": STATE_PROCESSED}}).
				SetUpdate(bson.M{"$set": bson.M{"state": STATE_PROCESSED}}),
		)
	}

	// fmt.Printf("Updates[%d]: %v\n", len(updates), updates)

	res, err := s.mongoClient.Database(s.dbName).Collection("documentCollection").BulkWrite(ctxProcess, updates)
	if err != nil {
		http.Error(w, "Error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(res)
}

func main() {
	cfg := getEnvVariables("./properties.json")

	// Init mongo
	clientOptions := options.Client().ApplyURI(cfg.mongoUri)
	mongoCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	mongoClient, err := mongo.Connect(mongoCtx, clientOptions)
	if err != nil {
		log.Fatal(err)
	}

	// Init context
	ctx := serverContext{port: cfg.port, mongoClient: mongoClient, dbName: cfg.mongoDb, collectionIndex: make(map[string]bool)}

	port := fmt.Sprintf(":%s", ctx.port)

	http.HandleFunc("GET /", ctx.rootHandler)
	http.HandleFunc("GET /health", ctx.healthHandler)
	http.HandleFunc("POST /save", ctx.saveHandler)
	http.HandleFunc("POST /batch/save", ctx.saveBatchHandler)
	http.HandleFunc("PUT /update/{key}/verified", ctx.updateToVerified)
	http.HandleFunc("PUT /update/{key}/rejected", ctx.updateToRejected)
	http.HandleFunc("PUT /process/{documentId}", ctx.processBatchHandler)

	fmt.Println("Server is listening on port http://localhost" + port)
	if err := http.ListenAndServe(port, nil); err != nil {
		fmt.Println("Error while starting server: ", err)
	}
}
