package storage

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type S3Store struct {
	client     *minio.Client
	bucketName string
}

func NewS3Store() (*S3Store, error) {
	if err := godotenv.Load(); err != nil {
		log.Println("no .env file found, relying on real env vars")
	}

	s3URL := os.Getenv("S3_ENDPOINT_URL")
	bucketName := os.Getenv("S3_BUCKET_NAME")
	accessKey := os.Getenv("S3_ACCESS_KEY_ID")
	secretKey := os.Getenv("S3_SECRET_ACCESS_KEY")
	useSSL, err := strconv.ParseBool(os.Getenv("S3_USE_SSL"))
	if err != nil {
		useSSL = false
	}

	client, err := minio.New(s3URL, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	exists, err := client.BucketExists(ctx, bucketName)
	if err != nil {
		return nil, err
	}
	if !exists {
		err = client.MakeBucket(ctx, bucketName, minio.MakeBucketOptions{Region: "us-east-1"})
		if err != nil {
			return nil, err
		}
	}

	return &S3Store{client: client, bucketName: bucketName}, nil
}

func (s *S3Store) ListTracks(ctx context.Context) ([]string, error) {
	var keys []string
	opts := minio.ListObjectsOptions{Recursive: true}
	for obj := range s.client.ListObjects(ctx, s.bucketName, opts) {
		if obj.Err != nil {
			return nil, obj.Err
		}
		keys = append(keys, obj.Key)
	}

	return keys, nil
}

func (s *S3Store) GetObject(ctx context.Context, key string) (*minio.Object, error) {
	return s.client.GetObject(ctx, s.bucketName, key, minio.GetObjectOptions{})
}

func (s *S3Store) UploadTrack(ctx context.Context, key string, body io.Reader, size int64) error {
	_, err := s.client.PutObject(ctx, s.bucketName, key, body, size, minio.PutObjectOptions{
		ContentType: "audio/mpeg",
	})
	return err
}

func (s *S3Store) DeleteTrack(ctx context.Context, key string) error {
	if _, err := s.client.StatObject(ctx, s.bucketName, key, minio.StatObjectOptions{}); err != nil {
		return fmt.Errorf("object %q not found: %w", key, err)
	}
	return s.client.RemoveObject(ctx, s.bucketName, key, minio.RemoveObjectOptions{})
}
