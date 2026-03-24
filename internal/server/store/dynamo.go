package store

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/depscope/depscope/internal/core"
)

const ttlDays = 7

// dynamoItem is the DynamoDB representation of a ScanJob.
type dynamoItem struct {
	ID        string `dynamodbav:"id"`
	URL       string `dynamodbav:"url"`
	Profile   string `dynamodbav:"profile"`
	Status    string `dynamodbav:"status"`
	Error     string `dynamodbav:"error,omitempty"`
	Result    []byte `dynamodbav:"result,omitempty"` // gzip-compressed JSON
	CreatedAt int64  `dynamodbav:"created_at"`       // Unix timestamp
	TTL       int64  `dynamodbav:"ttl"`              // Unix timestamp for DynamoDB TTL
}

// DynamoStore is a DynamoDB-backed implementation of ScanStore.
type DynamoStore struct {
	client    *dynamodb.Client
	tableName string
}

// NewDynamoStore creates a new DynamoStore using the default AWS configuration.
func NewDynamoStore(ctx context.Context, tableName string) (*DynamoStore, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}
	client := dynamodb.NewFromConfig(cfg)
	return &DynamoStore{client: client, tableName: tableName}, nil
}

// Create stores a new ScanJob with status "queued", created_at, and ttl set.
func (s *DynamoStore) Create(id string, req ScanRequest) error {
	now := time.Now()
	item := dynamoItem{
		ID:        id,
		URL:       req.URL,
		Profile:   req.Profile,
		Status:    "queued",
		CreatedAt: now.Unix(),
		TTL:       now.Add(ttlDays * 24 * time.Hour).Unix(),
	}
	av, err := attributevalue.MarshalMap(item)
	if err != nil {
		return fmt.Errorf("marshal item: %w", err)
	}
	_, err = s.client.PutItem(context.Background(), &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item:      av,
	})
	if err != nil {
		return fmt.Errorf("put item: %w", err)
	}
	return nil
}

// UpdateStatus changes the status of an existing job.
func (s *DynamoStore) UpdateStatus(id string, status string) error {
	_, err := s.client.UpdateItem(context.Background(), &dynamodb.UpdateItemInput{
		TableName: aws.String(s.tableName),
		Key:       keyOf(id),
		UpdateExpression: aws.String("SET #s = :s"),
		ExpressionAttributeNames: map[string]string{
			"#s": "status",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":s": &types.AttributeValueMemberS{Value: status},
		},
	})
	if err != nil {
		return fmt.Errorf("update status: %w", err)
	}
	return nil
}

// SaveResult compresses the result as gzip JSON and sets status to "complete".
func (s *DynamoStore) SaveResult(id string, result *core.ScanResult) error {
	compressed, err := compressResult(result)
	if err != nil {
		return fmt.Errorf("compress result: %w", err)
	}
	_, err = s.client.UpdateItem(context.Background(), &dynamodb.UpdateItemInput{
		TableName: aws.String(s.tableName),
		Key:       keyOf(id),
		UpdateExpression: aws.String("SET #s = :s, #r = :r"),
		ExpressionAttributeNames: map[string]string{
			"#s": "status",
			"#r": "result",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":s": &types.AttributeValueMemberS{Value: "complete"},
			":r": &types.AttributeValueMemberB{Value: compressed},
		},
	})
	if err != nil {
		return fmt.Errorf("save result: %w", err)
	}
	return nil
}

// SaveError stores the error message and sets status to "failed".
func (s *DynamoStore) SaveError(id string, errMsg string) error {
	_, err := s.client.UpdateItem(context.Background(), &dynamodb.UpdateItemInput{
		TableName: aws.String(s.tableName),
		Key:       keyOf(id),
		UpdateExpression: aws.String("SET #s = :s, #e = :e"),
		ExpressionAttributeNames: map[string]string{
			"#s": "status",
			"#e": "error",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":s": &types.AttributeValueMemberS{Value: "failed"},
			":e": &types.AttributeValueMemberS{Value: errMsg},
		},
	})
	if err != nil {
		return fmt.Errorf("save error: %w", err)
	}
	return nil
}

// Get retrieves a job by ID, decompressing the result if present.
func (s *DynamoStore) Get(id string) (*ScanJob, error) {
	out, err := s.client.GetItem(context.Background(), &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key:       keyOf(id),
	})
	if err != nil {
		return nil, fmt.Errorf("get item: %w", err)
	}
	if out.Item == nil {
		return nil, fmt.Errorf("job not found: %s", id)
	}
	return unmarshalItem(out.Item)
}

// List does a DynamoDB Scan and returns all scan jobs.
func (s *DynamoStore) List() []*ScanJob {
	var jobs []*ScanJob
	paginator := dynamodb.NewScanPaginator(s.client, &dynamodb.ScanInput{
		TableName: aws.String(s.tableName),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(context.Background())
		if err != nil {
			// Return whatever we have so far on error.
			return jobs
		}
		for _, av := range page.Items {
			job, err := unmarshalItem(av)
			if err != nil {
				continue
			}
			jobs = append(jobs, job)
		}
	}
	return jobs
}

// keyOf returns the DynamoDB primary key attribute map for a given ID.
func keyOf(id string) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{
		"id": &types.AttributeValueMemberS{Value: id},
	}
}

// unmarshalItem converts a DynamoDB attribute map into a ScanJob.
func unmarshalItem(av map[string]types.AttributeValue) (*ScanJob, error) {
	var item dynamoItem
	if err := attributevalue.UnmarshalMap(av, &item); err != nil {
		return nil, fmt.Errorf("unmarshal item: %w", err)
	}
	job := &ScanJob{
		ID:        item.ID,
		URL:       item.URL,
		Profile:   item.Profile,
		Status:    item.Status,
		Error:     item.Error,
		CreatedAt: time.Unix(item.CreatedAt, 0),
	}
	if len(item.Result) > 0 {
		result, err := decompressResult(item.Result)
		if err != nil {
			return nil, fmt.Errorf("decompress result: %w", err)
		}
		job.Result = result
	}
	return job, nil
}

// compressResult JSON-encodes and gzip-compresses a ScanResult.
func compressResult(result *core.ScanResult) ([]byte, error) {
	raw, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("json marshal: %w", err)
	}
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(raw); err != nil {
		return nil, fmt.Errorf("gzip write: %w", err)
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("gzip close: %w", err)
	}
	return buf.Bytes(), nil
}

// decompressResult gzip-decompresses and JSON-decodes a ScanResult.
func decompressResult(data []byte) (*core.ScanResult, error) {
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("gzip reader: %w", err)
	}
	defer r.Close() //nolint:errcheck
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("gzip read: %w", err)
	}
	var result core.ScanResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("json unmarshal: %w", err)
	}
	return &result, nil
}
