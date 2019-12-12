/*
Copyright 2019

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"flag"
	"log"
	"os"
	"strconv"

	awssqs "github.com/whynowy/knative-source-awssqs/pkg/adapter"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"sigs.k8s.io/controller-runtime/pkg/runtime/signals"

	"golang.org/x/net/context"
)

const (
	// envCredsFile is the path of the AWS credentials file
	envCredsFile = "AWS_APPLICATION_CREDENTIALS"

	// Environment variable containing IAM role ARN to access SQS
	envSqsIamRoleArn = "SQS_IAM_ROLE_ARN"

	// Environment variable containing SQS queue name
	envQueueName = "SQS_QUEUE_NAME"

	// Environment variable containing SQS queue region
	envRegion = "SQS_REGION"

	// Environment variable for bulk delivery flag
	envBulkDelivery = "BULK_DELIVERY"

	// Environment variable for workers
	envWorkers = "WORKERS"

	// Sink for messages.
	envSinkURI = "SINK_URI"
)

func getRequiredEnv(envKey string) string {
	val, defined := os.LookupEnv(envKey)
	if !defined {
		log.Fatalf("required environment variable not defined '%s'", envKey)
	}
	return val
}

func getOptionalEnv(envKey string) string {
	val, defined := os.LookupEnv(envKey)
	if !defined {
		return ""
	}
	return val
}

func main() {
	flag.Parse()

	ctx := context.Background()
	logCfg := zap.NewProductionConfig()
	logCfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	logger, err := logCfg.Build()
	if err != nil {
		log.Fatalf("Unable to create logger: %v", err)
	}

	workers, err := strconv.Atoi(getRequiredEnv(envWorkers))
	if err != nil {
		log.Fatalf("Environment variable value of %v is invalid: %v", envWorkers, err)
	}

	bulkDelivery := false
	bulkDeliveryStr := getOptionalEnv(envBulkDelivery)
	if len(bulkDeliveryStr) > 0 {
		bulkDelivery, err = strconv.ParseBool(bulkDeliveryStr)
		if err != nil {
			log.Fatalf("Environment variable value of %v is invalid: %v", envBulkDelivery, err)
		}
	}

	adapter := &awssqs.Adapter{
		SqsIamRoleArn: getOptionalEnv(envSqsIamRoleArn),
		CredsFile:     getOptionalEnv(envCredsFile),
		QueueName:     getRequiredEnv(envQueueName),
		Region:        getRequiredEnv(envRegion),
		SinkURI:       getRequiredEnv(envSinkURI),
		Workers:       workers,
		BulkDelivery:  bulkDelivery,
	}

	logger.Info("Starting AWS SQS Receive Adapter.", zap.Any("adapter", adapter))
	stopCh := signals.SetupSignalHandler()
	if err := adapter.Start(ctx, stopCh); err != nil {
		logger.Fatal("failed to start adapter: ", zap.Error(err))
	}
}
