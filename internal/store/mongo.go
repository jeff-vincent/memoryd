package store

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// MongoStore implements Store backed by MongoDB Atlas with $vectorSearch.
type MongoStore struct {
	client     *mongo.Client
	collection *mongo.Collection
	retrievals *mongo.Collection
	sources    *mongo.Collection
	pages      *mongo.Collection
}

// NewMongoStore connects to Atlas and returns a ready store.
func NewMongoStore(ctx context.Context, uri, database string) (*MongoStore, error) {
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return nil, fmt.Errorf("connecting to Atlas: %w", err)
	}

	if err := client.Ping(ctx, nil); err != nil {
		return nil, fmt.Errorf("pinging Atlas: %w", err)
	}

	db := client.Database(database)
	return &MongoStore{
		client:     client,
		collection: db.Collection("memories"),
		retrievals: db.Collection("retrieval_events"),
		sources:    db.Collection("sources"),
		pages:      db.Collection("source_pages"),
	}, nil
}

func (s *MongoStore) VectorSearch(ctx context.Context, embedding []float32, topK int) ([]Memory, error) {
	pipeline := mongo.Pipeline{
		{{Key: "$vectorSearch", Value: bson.D{
			{Key: "index", Value: "vector_index"},
			{Key: "path", Value: "embedding"},
			{Key: "queryVector", Value: embedding},
			{Key: "numCandidates", Value: topK * 20},
			{Key: "limit", Value: topK},
		}}},
		{{Key: "$project", Value: bson.D{
			{Key: "content", Value: 1},
			{Key: "source", Value: 1},
			{Key: "metadata", Value: 1},
			{Key: "created_at", Value: 1},
			{Key: "score", Value: bson.D{{Key: "$meta", Value: "vectorSearchScore"}}},
		}}},
	}

	cursor, err := s.collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("vector search: %w", err)
	}
	defer cursor.Close(ctx)

	var results []Memory
	if err := cursor.All(ctx, &results); err != nil {
		return nil, fmt.Errorf("decoding search results: %w", err)
	}
	return results, nil
}

func (s *MongoStore) Insert(ctx context.Context, mem Memory) error {
	if mem.CreatedAt.IsZero() {
		mem.CreatedAt = time.Now()
	}
	_, err := s.collection.InsertOne(ctx, mem)
	return err
}

func (s *MongoStore) Delete(ctx context.Context, id string) error {
	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return fmt.Errorf("invalid memory ID %q: %w", id, err)
	}
	_, err = s.collection.DeleteOne(ctx, bson.M{"_id": oid})
	return err
}

func (s *MongoStore) List(ctx context.Context, query string, limit int) ([]Memory, error) {
	filter := bson.M{}
	if query != "" {
		filter["content"] = bson.M{"$regex": query, "$options": "i"}
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "created_at", Value: -1}})
	if limit > 0 {
		opts.SetLimit(int64(limit))
	}

	cursor, err := s.collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var results []Memory
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}

func (s *MongoStore) DeleteAll(ctx context.Context) error {
	_, err := s.collection.DeleteMany(ctx, bson.M{})
	return err
}

func (s *MongoStore) CountBySource(ctx context.Context, source string) (int64, error) {
	return s.collection.CountDocuments(ctx, bson.M{"source": source})
}

func (s *MongoStore) UpdateContent(ctx context.Context, id string, content string, emb []float32) error {
	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return fmt.Errorf("invalid memory ID %q: %w", id, err)
	}
	_, err = s.collection.UpdateByID(ctx, oid, bson.M{
		"$set": bson.M{"content": content, "embedding": emb},
	})
	return err
}

func (s *MongoStore) ListBySource(ctx context.Context, sourcePrefix string, limit int) ([]Memory, error) {
	if limit <= 0 {
		limit = 100
	}
	filter := bson.M{"source": bson.M{"$regex": "^" + regexp.QuoteMeta(sourcePrefix)}}
	opts := options.Find().
		SetLimit(int64(limit)).
		SetSort(bson.D{{Key: "created_at", Value: -1}}).
		SetProjection(bson.M{"embedding": 0})
	cursor, err := s.collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var results []Memory
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}

func (s *MongoStore) Close() error {
	return s.client.Disconnect(context.Background())
}

// --- Steward support ---

// ListOldest returns memories sorted by created_at ascending, including embeddings.
func (s *MongoStore) ListOldest(ctx context.Context, limit int) ([]Memory, error) {
	if limit <= 0 {
		limit = 500
	}
	opts := options.Find().
		SetSort(bson.D{{Key: "created_at", Value: 1}}).
		SetLimit(int64(limit))
	cursor, err := s.collection.Find(ctx, bson.M{}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var results []Memory
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}

// UpdateQualityScore sets quality_score on a memory.
func (s *MongoStore) UpdateQualityScore(ctx context.Context, id primitive.ObjectID, score float64) error {
	_, err := s.collection.UpdateByID(ctx, id, bson.M{
		"$set": bson.M{"quality_score": score},
	})
	return err
}

// --- QualityStore implementation ---

func (s *MongoStore) RecordRetrievalBatch(ctx context.Context, events []RetrievalEvent) error {
	if len(events) == 0 {
		return nil
	}
	docs := make([]interface{}, len(events))
	for i, e := range events {
		if e.CreatedAt.IsZero() {
			e.CreatedAt = time.Now()
		}
		docs[i] = e
	}
	_, err := s.retrievals.InsertMany(ctx, docs)
	return err
}

func (s *MongoStore) GetRetrievalCount(ctx context.Context) (int64, error) {
	return s.retrievals.CountDocuments(ctx, bson.M{})
}

func (s *MongoStore) IncrementHitCount(ctx context.Context, id primitive.ObjectID) error {
	_, err := s.collection.UpdateByID(ctx, id, bson.M{
		"$inc": bson.M{"hit_count": 1},
		"$set": bson.M{"last_retrieved": time.Now()},
	})
	return err
}

func (s *MongoStore) RecentRetrievals(ctx context.Context, limit int) ([]RetrievalLog, error) {
	if limit <= 0 {
		limit = 50
	}
	opts := options.Find().SetSort(bson.M{"created_at": -1}).SetLimit(int64(limit))
	cursor, err := s.retrievals.Find(ctx, bson.M{}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var events []RetrievalEvent
	if err := cursor.All(ctx, &events); err != nil {
		return nil, err
	}

	// Collect unique memory IDs and batch-fetch content.
	idSet := make(map[primitive.ObjectID]struct{})
	var ids []primitive.ObjectID
	for _, e := range events {
		if _, ok := idSet[e.MemoryID]; !ok {
			idSet[e.MemoryID] = struct{}{}
			ids = append(ids, e.MemoryID)
		}
	}

	memMap := make(map[primitive.ObjectID]Memory)
	if len(ids) > 0 {
		memCursor, err := s.collection.Find(ctx, bson.M{"_id": bson.M{"$in": ids}})
		if err == nil {
			defer memCursor.Close(ctx)
			var mems []Memory
			if err := memCursor.All(ctx, &mems); err == nil {
				for _, m := range mems {
					memMap[m.ID] = m
				}
			}
		}
	}

	var logs []RetrievalLog
	for _, e := range events {
		m := memMap[e.MemoryID]
		content := m.Content
		if len(content) > 200 {
			content = content[:200] + "..."
		}
		logs = append(logs, RetrievalLog{
			MemoryID:  e.MemoryID,
			Content:   content,
			Source:    m.Source,
			Score:     e.Score,
			CreatedAt: e.CreatedAt,
		})
	}
	return logs, nil
}

func (s *MongoStore) TopMemories(ctx context.Context, limit int) ([]Memory, error) {
	if limit <= 0 {
		limit = 10
	}
	opts := options.Find().SetSort(bson.M{"hit_count": -1}).SetLimit(int64(limit))
	cursor, err := s.collection.Find(ctx, bson.M{"hit_count": bson.M{"$gt": 0}}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var mems []Memory
	if err := cursor.All(ctx, &mems); err != nil {
		return nil, err
	}
	return mems, nil
}

// --- SourceStore implementation ---

func (s *MongoStore) InsertSource(ctx context.Context, src Source) (string, error) {
	if src.CreatedAt.IsZero() {
		src.CreatedAt = time.Now()
	}
	result, err := s.sources.InsertOne(ctx, src)
	if err != nil {
		return "", err
	}
	return result.InsertedID.(primitive.ObjectID).Hex(), nil
}

func (s *MongoStore) ListSources(ctx context.Context) ([]Source, error) {
	cursor, err := s.sources.Find(ctx, bson.M{},
		options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var results []Source
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}

func (s *MongoStore) DeleteSource(ctx context.Context, id string) error {
	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return fmt.Errorf("invalid source ID: %w", err)
	}
	_, err = s.sources.DeleteOne(ctx, bson.M{"_id": oid})
	return err
}

func (s *MongoStore) UpdateSourceStatus(ctx context.Context, id string, status string, errMsg string, pageCount int, memoryCount int) error {
	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return fmt.Errorf("invalid source ID: %w", err)
	}
	update := bson.M{
		"status":       status,
		"error":        errMsg,
		"page_count":   pageCount,
		"memory_count": memoryCount,
		"last_crawled": time.Now(),
	}
	_, err = s.sources.UpdateByID(ctx, oid, bson.M{"$set": update})
	return err
}

func (s *MongoStore) GetSourcePage(ctx context.Context, sourceID primitive.ObjectID, pageURL string) (*SourcePage, error) {
	var page SourcePage
	err := s.pages.FindOne(ctx, bson.M{"source_id": sourceID, "url": pageURL}).Decode(&page)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &page, nil
}

func (s *MongoStore) UpsertSourcePage(ctx context.Context, page SourcePage) error {
	if page.LastFetched.IsZero() {
		page.LastFetched = time.Now()
	}
	_, err := s.pages.UpdateOne(ctx,
		bson.M{"source_id": page.SourceID, "url": page.URL},
		bson.M{"$set": page},
		options.Update().SetUpsert(true),
	)
	return err
}

func (s *MongoStore) DeleteSourcePages(ctx context.Context, sourceID primitive.ObjectID) error {
	_, err := s.pages.DeleteMany(ctx, bson.M{"source_id": sourceID})
	return err
}

func (s *MongoStore) DeleteMemoriesBySource(ctx context.Context, source string) error {
	// Use regex prefix match so "source:name" matches "source:name|url".
	filter := bson.M{"source": bson.M{"$regex": "^" + regexp.QuoteMeta(source)}}
	_, err := s.collection.DeleteMany(ctx, filter)
	return err
}
