package config

import (
	"flag"
	"os"
)

type Config struct {
	ServRunAddr    string
	PostgresURL    string
	MinIOEndpoint  string
	MinIOAccessKey string
	MinIOSecretKey string
	MinIOBucket    string
	RabbitMQURL    string
}

func NewConfig() *Config {
	return &Config{}
}

func (cfg *Config) GetEnvData() {

	flag.StringVar(&cfg.ServRunAddr, "a", "localhost:8080", "address for start server")
	flag.StringVar(&cfg.PostgresURL, "b", "", "address fot connect to DB")
	flag.StringVar(&cfg.MinIOEndpoint, "c", "", "")
	flag.StringVar(&cfg.MinIOAccessKey, "d", "", "")
	flag.StringVar(&cfg.MinIOSecretKey, "e", "", "")
	flag.StringVar(&cfg.MinIOBucket, "f", "", "'")
	flag.StringVar(&cfg.RabbitMQURL, "g", "", "")

	flag.Parse()

	if servRunAddrEnv, ok := os.LookupEnv("RUN_ADDRESS"); ok {
		cfg.ServRunAddr = servRunAddrEnv
	}

	if postgresURLEnv, ok := os.LookupEnv("POSTGRES_URL"); ok {
		cfg.PostgresURL = postgresURLEnv
	}

	if minIOEndpointEnv, ok := os.LookupEnv("MINIO_ENDPOINT"); ok {
		cfg.MinIOEndpoint = minIOEndpointEnv
	}

	if minIOAccessKeyEnv, ok := os.LookupEnv("MINIO_ACCESS_KEY"); ok {
		cfg.MinIOAccessKey = minIOAccessKeyEnv
	}

	if minIOSecretKeyEnv, ok := os.LookupEnv("MINIO_SECRET_KEY"); ok {
		cfg.MinIOSecretKey = minIOSecretKeyEnv
	}

	if minIOBucketEnv, ok := os.LookupEnv("MINIO_BUCKET"); ok {
		cfg.MinIOBucket = minIOBucketEnv
	}

	if rabbitMQURLEnv, ok := os.LookupEnv("RABBITMQ_URL"); ok {
		cfg.RabbitMQURL = rabbitMQURLEnv
	}

}
