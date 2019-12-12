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

package awssqs

import (
	"fmt"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"

	"github.com/cloudevents/sdk-go/pkg/cloudevents"
	"github.com/cloudevents/sdk-go/pkg/cloudevents/client"
	"github.com/cloudevents/sdk-go/pkg/cloudevents/types"

	"github.com/knative/eventing-sources/pkg/kncloudevents"
	"github.com/knative/pkg/logging"

	"go.uber.org/zap"
	"golang.org/x/net/context"
)

const (
	eventType = "aws.sqs.message"
)

// Adapter implements the AWS SQS adapter to deliver SQS messages from
// an SQS queue to a Sink.
type Adapter struct {

	// QueueName is the AWS SQS name
	QueueName string

	// Region is the AWS SQS region
	Region string

	// Goroutine workers to listen to SQS queue
	Workers int

	// SinkURI is the URI messages will be forwarded on to.
	SinkURI string

	// SqsIamRoleArn is the pre-existing AWS IAM role to access SQS queue, it is required when using KIAM approach.
	SqsIamRoleArn string

	// CredsFile is the full path of the AWS credentials file, it is required when using k8s secret approach.
	CredsFile string

	// Deliver multiple messages or single message
	BulkDelivery bool

	// Client sends cloudevents to the target.
	client client.Client

	queueURL string
}

// Initialize cloudevent client
func (a *Adapter) initClient() error {
	if a.client == nil {
		var err error
		if a.client, err = kncloudevents.NewDefaultClient(a.SinkURI); err != nil {
			return err
		}
	}
	return nil
}

// Start function
func (a *Adapter) Start(ctx context.Context, stopCh <-chan struct{}) error {

	logger := logging.FromContext(ctx)

	logger.Info("Starting with config: ", zap.Any("adapter", a))

	err := a.initClient()
	if err != nil {
		logger.Error("Failed to create cloudevent client", zap.Error(err))
		return err
	}

	var svc *sqs.SQS
	if len(a.CredsFile) > 0 {
		sess := session.Must(session.NewSessionWithOptions(session.Options{
			SharedConfigState: session.SharedConfigDisable,
			Config:            aws.Config{Region: &a.Region},
			SharedConfigFiles: []string{a.CredsFile},
		}))
		svc = sqs.New(sess)
	} else if len(a.SqsIamRoleArn) > 0 {
		sess := session.Must(session.NewSession())
		creds := stscreds.NewCredentials(sess, a.SqsIamRoleArn)
		svc = sqs.New(sess, &aws.Config{Credentials: creds, Region: aws.String(a.Region)})
	} else {
		return fmt.Errorf("Neither AWS_APPLICATION_CREDENTIALS nor SQS_IAM_ROLE_ARN is found in ENV")
	}

	urlResult, err := svc.GetQueueUrl(&sqs.GetQueueUrlInput{
		QueueName: aws.String(a.QueueName),
	})
	if err != nil {
		logger.Error("Failed to get SQS queue url: %v", err)
		return err
	}
	a.queueURL = *urlResult.QueueUrl
	logger.Debugf("Queue url: %v", a.queueURL)

	// Sleep 20 seconds to wait for VirtualService networking is ready.
	time.Sleep(20 * time.Second)

	return a.pollLoop(ctx, svc, stopCh)
}

// pollLoop continuously polls from the given SQS queue until stopCh
// emits an element.
func (a *Adapter) pollLoop(ctx context.Context, svc *sqs.SQS, stopCh <-chan struct{}) error {
	logger := logging.FromContext(ctx)

	cctx, cancel := context.WithCancel(ctx)
	for i := 0; i < a.Workers; i++ {
		go func(i int) {
			for {
				result, err := svc.ReceiveMessageWithContext(cctx, &sqs.ReceiveMessageInput{
					AttributeNames: []*string{
						aws.String(sqs.MessageSystemAttributeNameSentTimestamp),
					},
					MessageAttributeNames: []*string{
						aws.String(sqs.QueueAttributeNameAll),
					},
					QueueUrl:            &a.queueURL,
					MaxNumberOfMessages: aws.Int64(10),
					VisibilityTimeout:   aws.Int64(120), // 120 seconds
					WaitTimeSeconds:     aws.Int64(3),
				})
				if err != nil {
					logger.Errorf("[Goroutine %v]: Failed to retrieve message: %v", i, err)
					time.Sleep(2 * time.Second)
				}
				if result != nil && len(result.Messages) > 0 {
					logger.Debugf("[Goroutine %v]: [Received message] %v", i, result)
					if a.BulkDelivery {
						err = a.postMessages(logger, result.Messages)
						if err != nil {
							logger.Errorf("[Goroutine %v]: Failed to post bulk messages: %v", i, err)
							// TODO:
						} else {
							for _, message := range result.Messages {
								err = a.deleteMessage(svc, message)
								if err != nil {
									logger.Errorf("[Goroutine %v]: Failed to delete message: %v", i, err)
								}
							}
						}
					} else {
						for _, message := range result.Messages {
							err = a.postMessage(logger, message)
							if err != nil {
								logger.Errorf("[Goroutine %v]: Failed to post message: %v", i, err)
								// TODO:
							} else {
								err = a.deleteMessage(svc, message)
								if err != nil {
									logger.Errorf("[Goroutine %v]: Failed to delete message: %v", i, err)
								}
							}
						}
					}
				} else {
					time.Sleep(2 * time.Second)
				}
			}
		}(i)
	}
	<-stopCh
	cancel()
	logger.Info("Shutting down.")
	return nil
}

// postMessages sends bulk SQS events to the SinkURI
func (a *Adapter) postMessages(logger *zap.SugaredLogger, messages []*sqs.Message) error {
	timestamp := time.Now().UnixNano()
	event := cloudevents.Event{
		Context: cloudevents.EventContextV02{
			Type:   eventType,
			Source: *types.ParseURLRef(a.queueURL),
			Time:   &types.Timestamp{Time: time.Unix(timestamp, 0)},
		}.AsV02(),
		Data: messages,
	}
	_, err := a.client.Send(context.TODO(), event)
	return err
}

// postMessage sends an SQS event to the SinkURI
func (a *Adapter) postMessage(logger *zap.SugaredLogger, m *sqs.Message) error {
	// TODO verify the timestamp conversion
	timestamp, err := strconv.ParseInt(*m.Attributes["SentTimestamp"], 10, 64)
	if err != nil {
		logger.Errorw("Failed to unmarshal the message.", zap.Error(err), zap.Any("message", m.Body))
		timestamp = time.Now().UnixNano()
	}

	event := cloudevents.Event{
		Context: cloudevents.EventContextV02{
			ID:     *m.MessageId,
			Type:   eventType,
			Source: *types.ParseURLRef(a.queueURL),
			Time:   &types.Timestamp{Time: time.Unix(timestamp, 0)},
		}.AsV02(),
		Data: m,
	}
	_, err = a.client.Send(context.TODO(), event)
	return err
}

func (a *Adapter) deleteMessage(svc *sqs.SQS, m *sqs.Message) error {
	deleteParams := &sqs.DeleteMessageInput{
		QueueUrl:      aws.String(a.queueURL),
		ReceiptHandle: m.ReceiptHandle,
	}
	_, err := svc.DeleteMessage(deleteParams) // No response returned when successed.
	return err
}
