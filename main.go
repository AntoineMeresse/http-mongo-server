package main

import (
	"context"
	"encoding/json"
	"fmt"
	"mongo-http-audit-service/src/myLogger"
	"net/http"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/rs/zerolog/log"
)

const (
	STATE_INIT      = "INIT"
	STATE_VERIFIED  = "VERIFIED"
	STATE_REJECTED  = "REJECTED"
	STATE_PROCESSED = "PROCESSED"
)

type serverContext struct {
	mongoClient     *mongo.Client
	dbName          string
	collectionIndex map[string]bool
}

type MyDocument struct {
	ID    *primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
	Name  string              `json:"name"`
	Key   string              `json:"key"`
	State string              `bson:"state,omitempty" json:"state,omitempty"`
}

type MyDocumentList struct {
	ID        *primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
	ToProcess []MyDocument        `json:"documentList"`
}

type MyDocumentId struct {
	ID *primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
}

func (s *serverContext) rootHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		myLogger.Log.Debug().Msgf("Unknow page: %s", r.URL.Path)
		http.NotFound(w, r)
		return
	}
	fmt.Fprintf(w, "This is the main page.")
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
	myLogger.Log.Debug().Msg("Ensure index start")
	name := collection.Name()
	if _, ok := s.collectionIndex[name]; ok {
		myLogger.Log.Trace().Msg("Ensure index already ok")
		return
	}

	indexes := []mongo.IndexModel{
		{Keys: bson.D{{Key: "key", Value: 1}}, Options: options.Index().SetUnique(true).SetName("keyIndex")},
	}

	_, err := collection.Indexes().CreateMany(ctx, indexes)
	if err != nil {
		myLogger.Log.Error().Msgf("Could not ensure index already exist. Error: %s", err.Error())
		return
	}

	myLogger.Log.Debug().Msgf("Ensure index was ok. Adding %s to map", name)
	s.collectionIndex[name] = true
}

func (s *serverContext) saveHandler(w http.ResponseWriter, r *http.Request) {
	collection := s.mongoClient.Database(s.dbName).Collection("documentCollection")

	var doc MyDocument
	if err := json.NewDecoder(r.Body).Decode(&doc); err != nil {
		myLogger.Log.Error().Msgf("Could not deserialized body :/")
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
		myLogger.Log.Error().Msgf("Could not insert document :/")
		http.Error(w, "Error: "+err.Error(), http.StatusBadRequest)
		return
	}

	if res != nil {
		myLogger.Log.Debug().Msg("Document was inserted")
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
		myLogger.Log.Error().Msg("Could not deserialized body to MyDocumentList")
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
		myLogger.Log.Error().Msg("Could not insert document :/")
		http.Error(w, "Error: "+err.Error(), http.StatusBadRequest)
		return
	}

	if res != nil {
		myLogger.Log.Debug().Msgf("Document batch was inserted. Res: %v", res)
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

	ctxProcess, cancelProcess := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancelProcess()

	updates := make([]mongo.WriteModel, 0, len(batchDocument.ToProcess))

	// myLogger.Log.Error().Msg("-------To process --------")
	for i, doc := range batchDocument.ToProcess {
		myLogger.Log.Debug().Msgf("Update n°%d -> key: %s", i, doc.Key)
		updates = append(updates,
			mongo.NewUpdateOneModel().
				SetFilter(bson.M{"key": doc.Key, "state": bson.M{"$ne": STATE_PROCESSED}}).
				SetUpdate(bson.M{"$set": bson.M{"state": STATE_PROCESSED}}),
		)
	}

	myLogger.Log.Debug().Msgf("Documents to update: %d", len(updates))

	res, err := s.mongoClient.Database(s.dbName).Collection("documentCollection").BulkWrite(ctxProcess, updates)
	if err != nil {
		bulkErr, ok := err.(mongo.BulkWriteException)
		if ok {
			for _, writeErr := range bulkErr.WriteErrors {
				myLogger.Log.Error().Msgf("[Bulk error] Index: %d | Error: %s", writeErr.Index, writeErr.Message)
			}
			http.Error(w, "Error bulk write: "+bulkErr.Error(), http.StatusInternalServerError)
		} else {
			http.Error(w, "Error: "+err.Error(), http.StatusInternalServerError)
		}
		return
	}

	json.NewEncoder(w).Encode(res)
}

func (ctx *serverContext) MainServer() *http.ServeMux {
	mainHttp := http.NewServeMux()
	mainHttp.HandleFunc("GET /", ctx.rootHandler)
	mainHttp.HandleFunc("POST /save", ctx.saveHandler)
	mainHttp.HandleFunc("POST /batch/save", ctx.saveBatchHandler)
	mainHttp.HandleFunc("PUT /update/{key}/verified", ctx.updateToVerified)
	mainHttp.HandleFunc("PUT /update/{key}/rejected", ctx.updateToRejected)
	mainHttp.HandleFunc("PUT /process/{documentId}", ctx.processBatchHandler)
	return mainHttp
}

func main() {
	cfg := getEnvVariables("./properties.json")
	myLogger.InitLogging(cfg.dev, cfg.levelLog)
	if cfg.dev {
		myLogger.Log.Debug().Msgf("Config: %v", cfg)
	}

	// Init mongo
	clientOptions := options.Client().ApplyURI(cfg.mongoUri)
	mongoCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	mongoClient, err := mongo.Connect(mongoCtx, clientOptions)
	if err != nil {
		log.Fatal().Msg(err.Error())
	}

	// Init context
	ctx := serverContext{mongoClient: mongoClient, dbName: cfg.mongoDb, collectionIndex: make(map[string]bool)}

	port := fmt.Sprintf(":%s", cfg.port)
	managementPort := fmt.Sprintf(":%s", cfg.managementPort)

	mainHttp := ctx.MainServer()

	if port == managementPort {
		mainHttp.HandleFunc("GET /health", ctx.healthHandler)
	} else {
		go func() {
			managementHttp := http.NewServeMux()
			managementHttp.HandleFunc("GET /health", ctx.healthHandler)
			myLogger.Log.Info().Msg("[Health] Server is listening on: http://localhost" + managementPort + "/health")
			myLogger.Log.Fatal().Err(http.ListenAndServe(managementPort, managementHttp))
		}()
	}

	myLogger.Log.Info().Msg("[ Main ] Server is listening on: http://localhost" + port)
	myLogger.Log.Fatal().Err(http.ListenAndServe(port, mainHttp))
}
