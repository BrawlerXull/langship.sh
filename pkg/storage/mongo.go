package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Mongo holds the connected client + database handle. Construct via NewMongo
// and Close on shutdown.
type Mongo struct {
	client *mongo.Client
	db     *mongo.Database
}

// NewMongo dials Mongo with a 10s connect+ping timeout and ensures indexes
// exist. Returns an error if the URI is unreachable.
func NewMongo(ctx context.Context, uri, dbName string) (*Mongo, error) {
	if uri == "" {
		return nil, errors.New("MONGO_URI is required")
	}
	if dbName == "" {
		dbName = "flow"
	}

	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		return nil, fmt.Errorf("mongo connect: %w", err)
	}
	if err := client.Ping(dialCtx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, fmt.Errorf("mongo ping %q: %w", uri, err)
	}

	m := &Mongo{client: client, db: client.Database(dbName)}
	if err := m.ensureIndexes(ctx); err != nil {
		return nil, fmt.Errorf("ensure indexes: %w", err)
	}
	return m, nil
}

// Close disconnects the client. Safe to call multiple times.
func (m *Mongo) Close(ctx context.Context) error {
	if m == nil || m.client == nil {
		return nil
	}
	return m.client.Disconnect(ctx)
}

// Pipelines returns the PipelineStore backed by this Mongo connection.
func (m *Mongo) Pipelines() PipelineStore { return &mongoPipelines{coll: m.db.Collection("pipelines")} }

// Runs returns the RunStore backed by this Mongo connection.
func (m *Mongo) Runs() RunStore { return &mongoRuns{coll: m.db.Collection("runs")} }

func (m *Mongo) ensureIndexes(ctx context.Context) error {
	if _, err := m.db.Collection("pipelines").Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "updated_at", Value: -1}}},
	}); err != nil {
		return fmt.Errorf("pipelines indexes: %w", err)
	}
	if _, err := m.db.Collection("runs").Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "pipeline_id", Value: 1}, {Key: "started_at", Value: -1}}},
		{Keys: bson.D{{Key: "started_at", Value: -1}}},
	}); err != nil {
		return fmt.Errorf("runs indexes: %w", err)
	}
	return nil
}

// --- pipelines ------------------------------------------------------------

type mongoPipelines struct{ coll *mongo.Collection }

// pipelineDoc swaps json.RawMessage for bson.Raw so the n8n definition stays
// queryable and round-trips cleanly through Mongo.
type pipelineDoc struct {
	ID         string    `bson:"_id"`
	Name       string    `bson:"name"`
	Definition bson.Raw  `bson:"definition"`
	NodeCount  int       `bson:"node_count"`
	Status     string    `bson:"status,omitempty"`
	CreatedAt  time.Time `bson:"created_at"`
	UpdatedAt  time.Time `bson:"updated_at"`
}

func (s *mongoPipelines) Create(ctx context.Context, p *Pipeline) error {
	def, err := jsonToBSON(p.Definition)
	if err != nil {
		return err
	}
	_, err = s.coll.InsertOne(ctx, pipelineDoc{
		ID:         p.ID,
		Name:       p.Name,
		Definition: def,
		NodeCount:  p.NodeCount,
		Status:     p.Status,
		CreatedAt:  p.CreatedAt,
		UpdatedAt:  p.UpdatedAt,
	})
	return err
}

func (s *mongoPipelines) Get(ctx context.Context, id string) (*Pipeline, error) {
	var doc pipelineDoc
	if err := s.coll.FindOne(ctx, bson.M{"_id": id}).Decode(&doc); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return docToPipeline(&doc)
}

func (s *mongoPipelines) Update(ctx context.Context, p *Pipeline) error {
	set := bson.M{
		"name":       p.Name,
		"node_count": p.NodeCount,
		"updated_at": p.UpdatedAt,
	}
	if p.Status != "" {
		set["status"] = p.Status
	}
	if len(p.Definition) > 0 {
		def, err := jsonToBSON(p.Definition)
		if err != nil {
			return err
		}
		set["definition"] = def
	}
	res, err := s.coll.UpdateByID(ctx, p.ID, bson.M{"$set": set})
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *mongoPipelines) Delete(ctx context.Context, id string) error {
	res, err := s.coll.DeleteOne(ctx, bson.M{"_id": id})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *mongoPipelines) List(ctx context.Context) ([]*Pipeline, error) {
	cur, err := s.coll.Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "updated_at", Value: -1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var docs []pipelineDoc
	if err := cur.All(ctx, &docs); err != nil {
		return nil, err
	}
	out := make([]*Pipeline, 0, len(docs))
	for i := range docs {
		p, err := docToPipeline(&docs[i])
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

func docToPipeline(d *pipelineDoc) (*Pipeline, error) {
	def, err := bsonToJSON(d.Definition)
	if err != nil {
		return nil, err
	}
	return &Pipeline{
		ID:         d.ID,
		Name:       d.Name,
		Definition: def,
		NodeCount:  d.NodeCount,
		Status:     d.Status,
		CreatedAt:  d.CreatedAt,
		UpdatedAt:  d.UpdatedAt,
	}, nil
}

// --- runs ----------------------------------------------------------------

type mongoRuns struct{ coll *mongo.Collection }

type runDoc struct {
	ID           string     `bson:"_id"`
	PipelineID   string     `bson:"pipeline_id"`
	PipelineName string     `bson:"pipeline_name,omitempty"`
	Status       string     `bson:"status"`
	StartedAt    time.Time  `bson:"started_at"`
	FinishedAt   *time.Time `bson:"finished_at,omitempty"`
	TriggerData  bson.Raw   `bson:"trigger_data,omitempty"`
	Outputs      bson.Raw   `bson:"outputs,omitempty"`
	NodeOutputs  bson.Raw   `bson:"node_outputs,omitempty"`
	Errors       []string   `bson:"errors,omitempty"`
}

func (s *mongoRuns) Insert(ctx context.Context, r *Run) error {
	td, err := jsonToBSON(r.TriggerData)
	if err != nil {
		return err
	}
	_, err = s.coll.InsertOne(ctx, runDoc{
		ID:           r.ID,
		PipelineID:   r.PipelineID,
		PipelineName: r.PipelineName,
		Status:       r.Status,
		StartedAt:    r.StartedAt,
		TriggerData:  td,
	})
	return err
}

func (s *mongoRuns) UpdateStatus(ctx context.Context, id, status string) error {
	_, err := s.coll.UpdateByID(ctx, id, bson.M{"$set": bson.M{"status": status}})
	return err
}

func (s *mongoRuns) Complete(ctx context.Context, id, status string, outputs, nodeOutputs json.RawMessage, errMsg string) error {
	out, err := jsonToBSON(outputs)
	if err != nil {
		return err
	}
	nodeOut, err := jsonToBSON(nodeOutputs)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	set := bson.M{
		"status":      status,
		"finished_at": now,
	}
	if out != nil {
		set["outputs"] = out
	}
	if nodeOut != nil {
		set["node_outputs"] = nodeOut
	}
	if errMsg != "" {
		set["errors"] = []string{errMsg}
	}
	_, err = s.coll.UpdateByID(ctx, id, bson.M{"$set": set})
	return err
}

func (s *mongoRuns) Get(ctx context.Context, id string) (*Run, error) {
	var d runDoc
	if err := s.coll.FindOne(ctx, bson.M{"_id": id}).Decode(&d); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return docToRun(&d)
}

func (s *mongoRuns) ListByPipeline(ctx context.Context, pipelineID string, limit int) ([]*Run, error) {
	if limit <= 0 {
		limit = 50
	}
	cur, err := s.coll.Find(ctx, bson.M{"pipeline_id": pipelineID},
		options.Find().SetSort(bson.D{{Key: "started_at", Value: -1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	return collectRuns(ctx, cur)
}

func (s *mongoRuns) List(ctx context.Context, limit int) ([]*Run, error) {
	if limit <= 0 {
		limit = 50
	}
	cur, err := s.coll.Find(ctx, bson.M{},
		options.Find().SetSort(bson.D{{Key: "started_at", Value: -1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	return collectRuns(ctx, cur)
}

func collectRuns(ctx context.Context, cur *mongo.Cursor) ([]*Run, error) {
	var docs []runDoc
	if err := cur.All(ctx, &docs); err != nil {
		return nil, err
	}
	out := make([]*Run, 0, len(docs))
	for i := range docs {
		r, err := docToRun(&docs[i])
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}

func docToRun(d *runDoc) (*Run, error) {
	td, err := bsonToJSON(d.TriggerData)
	if err != nil {
		return nil, err
	}
	out, err := bsonToJSON(d.Outputs)
	if err != nil {
		return nil, err
	}
	nodeOut, err := bsonToJSON(d.NodeOutputs)
	if err != nil {
		return nil, err
	}
	return &Run{
		ID:           d.ID,
		PipelineID:   d.PipelineID,
		PipelineName: d.PipelineName,
		Status:       d.Status,
		StartedAt:    d.StartedAt,
		FinishedAt:   d.FinishedAt,
		TriggerData:  td,
		Outputs:      out,
		NodeOutputs:  nodeOut,
		Errors:       d.Errors,
	}, nil
}

// --- json/bson conversions -----------------------------------------------

// jsonToBSON converts arbitrary JSON to a bson.Raw document so Mongo stores
// it as a real subdocument (queryable, indexable) rather than an opaque string.
// Returns nil for empty/null input.
func jsonToBSON(raw json.RawMessage) (bson.Raw, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("jsonToBSON: %w", err)
	}
	// Mongo top-level docs must be objects; wrap scalars/arrays under a key.
	if _, isObj := v.(map[string]any); !isObj {
		v = map[string]any{"_value": v}
	}
	b, err := bson.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("bson marshal: %w", err)
	}
	return b, nil
}

func bsonToJSON(r bson.Raw) (json.RawMessage, error) {
	if len(r) == 0 {
		return nil, nil
	}
	var v any
	if err := bson.Unmarshal(r, &v); err != nil {
		return nil, fmt.Errorf("bson unmarshal: %w", err)
	}
	if m, ok := v.(map[string]any); ok {
		if inner, hasWrap := m["_value"]; hasWrap && len(m) == 1 {
			v = inner
		}
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("json marshal: %w", err)
	}
	return b, nil
}
